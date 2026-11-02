package finance

import (
	"context"
	"database/sql"
	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/jackc/pgx/v5"
)

func (r *repository) readEvent(ctx context.Context, owner identity.OwnerID, id string) (Event, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback(ctx)
	event, deleted, err := readEventInTx(ctx, tx, owner, id, false)
	if err != nil {
		return Event{}, err
	}
	if deleted {
		return Event{}, ErrNotFound
	}
	if err = tx.Commit(ctx); err != nil {
		return Event{}, err
	}
	return event, nil
}

func readEventInTx(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, id string, lock bool) (Event, bool, error) {
	query := `SELECT event_revision,id,kind,amount_minor,currency,occurred_at,note,status,voided_at,void_reason,deleted_at IS NOT NULL FROM financial_events WHERE owner_id=$1 AND id=$2`
	if lock {
		query += " FOR UPDATE"
	}
	var e Event
	var at sql.NullTime
	var reason sql.NullString
	var deleted bool
	err := tx.QueryRow(ctx, query, owner.String(), id).Scan(&e.Revision, &e.ID, &e.Kind, &e.AmountMinor, &e.Currency, &e.OccurredAt, &e.Note, &e.Status, &at, &reason, &deleted)
	if err != nil {
		return e, false, notFoundOr("read event", err)
	}
	if at.Valid {
		e.VoidedAt = &at.Time
	}
	if reason.Valid {
		e.VoidReason = reason.String
	}
	e.Movements, err = eventMovementsInTx(ctx, tx, owner, id)
	if err != nil {
		return e, deleted, err
	}
	e.Allocations, err = eventAllocationsInTx(ctx, tx, owner, id)
	return e, deleted, err
}
