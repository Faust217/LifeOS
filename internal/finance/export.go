package finance

import (
	"context"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/Faust217/LifeOS/internal/platform/exportdata"
	"github.com/jackc/pgx/v5"
)

// WriteExport owns Finance SQL and deliberately avoids paginated UI services.
func WriteExport(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, sink exportdata.Sink, versions ...int) error {
	if !owner.Valid() {
		return ErrUnauthorized
	}
	version := 1
	if len(versions) > 0 {
		version = versions[0]
	}
	if version == 1 {
		var anchored bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM finance_accounts WHERE owner_id=$1 AND opening_balance_at IS NOT NULL)`, owner.String()).Scan(&anchored); err != nil {
			return err
		}
		if anchored {
			return ErrExportVersionIncompatible
		}
	}
	accountQuery := `SELECT id::text,name,account_type,currency,opening_balance_minor::text,archived_at,created_at,updated_at FROM finance_accounts WHERE owner_id=$1 ORDER BY created_at,id`
	if version == 2 {
		accountQuery = `SELECT id::text,name,account_type,currency,opening_balance_minor::text,archived_at,created_at,updated_at,CASE WHEN opening_balance_at IS NULL THEN 'legacy_all_movements' ELSE 'known_balance_before_instant' END AS opening_balance_basis,opening_balance_at,opening_balance_confirmed_at FROM finance_accounts WHERE owner_id=$1 ORDER BY created_at,id`
	}
	// Validate the retained ledger's event shape with exact numeric accumulators.
	var invalid bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM financial_events e
 WHERE e.owner_id=$1 AND e.deleted_at IS NULL AND (
 (SELECT count(*) FROM account_movements m WHERE m.owner_id=e.owner_id AND m.event_id=e.id) <> CASE WHEN e.kind='transfer' THEN 2 ELSE 1 END
 OR (SELECT COALESCE(sum(m.amount_minor::numeric),0) FROM account_movements m WHERE m.owner_id=e.owner_id AND m.event_id=e.id) <> CASE WHEN e.kind='expense' THEN -e.amount_minor::numeric WHEN e.kind='income' THEN e.amount_minor::numeric ELSE 0 END
 OR EXISTS(SELECT 1 FROM account_movements m WHERE m.owner_id=e.owner_id AND m.event_id=e.id AND abs(m.amount_minor::numeric)<>e.amount_minor::numeric)
 OR (e.kind='transfer' AND EXISTS(SELECT 1 FROM category_allocations a WHERE a.owner_id=e.owner_id AND a.event_id=e.id))
 OR (EXISTS(SELECT 1 FROM category_allocations a WHERE a.owner_id=e.owner_id AND a.event_id=e.id)
 AND (SELECT sum(a.amount_minor::numeric) FROM category_allocations a WHERE a.owner_id=e.owner_id AND a.event_id=e.id)<>e.amount_minor::numeric)
 ))`, owner.String()).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid {
		return ErrInvalidInput
	}
	queries := []struct{ name, sql string }{
		{"accounts", accountQuery},
		{"categories", `SELECT id::text,name,kind,archived_at,created_at,updated_at FROM finance_categories WHERE owner_id=$1 ORDER BY created_at,id`},
		{"events", `SELECT e.id::text,e.kind,e.amount_minor::text,e.currency,e.occurred_at,e.note,e.status,e.voided_at,e.void_reason,e.created_at,
 CASE WHEN e.kind<>'transfer' THEN (SELECT m.account_id::text FROM account_movements m WHERE m.owner_id=e.owner_id AND m.event_id=e.id LIMIT 1) END AS _account_id,
 CASE WHEN e.kind='transfer' THEN (SELECT m.account_id::text FROM account_movements m WHERE m.owner_id=e.owner_id AND m.event_id=e.id AND m.amount_minor<0) END AS _from_account_id,
 CASE WHEN e.kind='transfer' THEN (SELECT m.account_id::text FROM account_movements m WHERE m.owner_id=e.owner_id AND m.event_id=e.id AND m.amount_minor>0) END AS _to_account_id
 FROM financial_events e WHERE e.owner_id=$1 AND e.deleted_at IS NULL ORDER BY e.occurred_at DESC,e.id DESC`},
		{"movements", `SELECT m.id::text,m.event_id::text,m.account_id::text,m.amount_minor::text,m.currency,m.created_at,e.status AS _status FROM account_movements m JOIN financial_events e ON e.owner_id=m.owner_id AND e.id=m.event_id WHERE m.owner_id=$1 AND e.deleted_at IS NULL ORDER BY m.event_id,m.id`},
		{"allocations", `SELECT a.id::text,a.event_id::text,a.category_id::text,a.kind,a.amount_minor::text,a.currency,a.created_at,e.status AS _status FROM category_allocations a JOIN financial_events e ON e.owner_id=a.owner_id AND e.id=a.event_id WHERE a.owner_id=$1 AND e.deleted_at IS NULL ORDER BY a.event_id,a.id`},
	}
	for _, q := range queries {
		if err := exportdata.WriteRows(ctx, tx, owner.String(), q.name, q.sql, sink); err != nil {
			return err
		}
	}
	return nil
}
