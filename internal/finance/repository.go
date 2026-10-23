package finance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type repository struct{ pool *pgxpool.Pool }

type movementSpec struct {
	accountID string
	amount    int64
}

type postSpec struct {
	kind        string
	amount      int64
	currency    string
	occurredAt  time.Time
	note        string
	movements   []movementSpec
	allocations []AllocationInput
}

type accountState struct {
	currency  string
	archived  sql.NullTime
	openingAt *time.Time
}

func (r *repository) createAccount(ctx context.Context, owner identity.OwnerID, input AccountInput) (Account, error) {
	id, err := newUUID()
	if err != nil {
		return Account{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback(ctx)
	if err = requireOpeningZone(ctx, tx, owner); err != nil {
		return Account{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO finance_accounts(id,owner_id,name,account_type,currency,opening_balance_minor,opening_balance_at,opening_balance_confirmed_at,balance_revision) VALUES($1,$2,$3,$4,$5,$6,$7,clock_timestamp(),1)`, id, owner.String(), input.Name, input.Type, input.Currency, input.OpeningBalanceMinor, input.OpeningBalanceAt)
	if err != nil {
		return Account{}, err
	}
	account, err := readOpeningAccount(ctx, tx, owner, id, false)
	if err != nil {
		return Account{}, err
	}
	return account, tx.Commit(ctx)
}

func (r *repository) createCategory(ctx context.Context, owner identity.OwnerID, input CategoryInput) (Category, error) {
	id, err := newUUID()
	if err != nil {
		return Category{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Category{}, fmt.Errorf("begin category creation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockCategoryNames(ctx, tx, owner); err != nil {
		return Category{}, err
	}
	conflict, err := categoryNameExists(ctx, tx, owner, input.Kind, input.Name, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		return Category{}, err
	}
	if conflict {
		return Category{}, ErrConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO finance_categories (id, owner_id, name, kind)
		VALUES ($1, $2, $3, $4)`, id, owner.String(), input.Name, input.Kind)
	if err != nil {
		return Category{}, fmt.Errorf("create category: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Category{}, fmt.Errorf("commit category creation: %w", err)
	}
	return Category{ID: id, Name: input.Name, Kind: input.Kind}, nil
}

// A row lock on the owner serializes category name writes without changing the v4 schema.
func lockCategoryNames(ctx context.Context, tx pgx.Tx, owner identity.OwnerID) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM lifeos_users WHERE id = $1 FOR UPDATE`, owner.String()).Scan(&id)
	return notFoundOr("lock category owner", err)
}

func categoryNameExists(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, kind, name, exceptID string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM finance_categories
		WHERE owner_id = $1 AND kind = $2 AND lower(name) = lower($3) AND id <> $4)`,
		owner.String(), kind, name, exceptID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check category name: %w", err)
	}
	return exists, nil
}

func (r *repository) renameCategory(ctx context.Context, owner identity.OwnerID, categoryID, name string) (Category, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Category{}, fmt.Errorf("begin category rename: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockCategoryNames(ctx, tx, owner); err != nil {
		return Category{}, err
	}
	var category Category
	var archived sql.NullTime
	err = tx.QueryRow(ctx, `SELECT id, kind, archived_at FROM finance_categories
		WHERE owner_id = $1 AND id = $2 FOR UPDATE`, owner.String(), categoryID).
		Scan(&category.ID, &category.Kind, &archived)
	if err != nil {
		return Category{}, notFoundOr("find category", err)
	}
	conflict, err := categoryNameExists(ctx, tx, owner, category.Kind, name, categoryID)
	if err != nil {
		return Category{}, err
	}
	if conflict {
		return Category{}, ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE finance_categories SET name = $3, updated_at = now()
		WHERE owner_id = $1 AND id = $2`, owner.String(), categoryID, name)
	if err != nil {
		return Category{}, fmt.Errorf("rename category: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Category{}, fmt.Errorf("commit category rename: %w", err)
	}
	category.Name = name
	if archived.Valid {
		category.ArchivedAt = &archived.Time
	}
	return category, nil
}

func (r *repository) archiveCategory(ctx context.Context, owner identity.OwnerID, categoryID string) error {
	var id string
	err := r.pool.QueryRow(ctx, `UPDATE finance_categories SET archived_at = now(), updated_at = now()
		WHERE owner_id = $1 AND id = $2 AND archived_at IS NULL RETURNING id`, owner.String(), categoryID).Scan(&id)
	return notFoundOr("archive category", err)
}

func (r *repository) restoreCategory(ctx context.Context, owner identity.OwnerID, categoryID string) error {
	var id string
	err := r.pool.QueryRow(ctx, `UPDATE finance_categories SET archived_at = NULL, updated_at = now()
		WHERE owner_id = $1 AND id = $2 AND archived_at IS NOT NULL RETURNING id`, owner.String(), categoryID).Scan(&id)
	return notFoundOr("restore category", err)
}

func (r *repository) listAccounts(ctx context.Context, owner identity.OwnerID) ([]Account, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, account_type, currency, opening_balance_minor, archived_at, opening_balance_at, opening_balance_confirmed_at, balance_revision
		FROM finance_accounts WHERE owner_id = $1 ORDER BY created_at, id`, owner.String())
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()
	accounts := make([]Account, 0)
	for rows.Next() {
		var account Account
		var archived sql.NullTime
		if err := rows.Scan(&account.ID, &account.Name, &account.Type, &account.Currency, &account.OpeningBalanceMinor, &archived, &account.OpeningBalanceAt, &account.OpeningConfirmedAt, &account.BalanceRevision); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		if archived.Valid {
			account.ArchivedAt = &archived.Time
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (r *repository) listCategories(ctx context.Context, owner identity.OwnerID) ([]Category, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, kind, archived_at FROM finance_categories
		WHERE owner_id = $1 ORDER BY created_at, id`, owner.String())
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()
	categories := make([]Category, 0)
	for rows.Next() {
		var category Category
		var archived sql.NullTime
		if err := rows.Scan(&category.ID, &category.Name, &category.Kind, &archived); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		if archived.Valid {
			category.ArchivedAt = &archived.Time
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}

func (r *repository) archiveAccount(ctx context.Context, owner identity.OwnerID, accountID string) error {
	var id string
	err := r.pool.QueryRow(ctx, `UPDATE finance_accounts SET archived_at = now(), updated_at = now(),balance_revision=balance_revision+1
		WHERE owner_id = $1 AND id = $2 AND archived_at IS NULL RETURNING id`, owner.String(), accountID).Scan(&id)
	return notFoundOr("archive account", err)
}

func (r *repository) restoreAccount(ctx context.Context, owner identity.OwnerID, accountID string) error {
	var id string
	err := r.pool.QueryRow(ctx, `UPDATE finance_accounts SET archived_at = NULL, updated_at = now(),balance_revision=balance_revision+1
		WHERE owner_id = $1 AND id = $2 AND archived_at IS NOT NULL RETURNING id`, owner.String(), accountID).Scan(&id)
	return notFoundOr("restore account", err)
}

func (r *repository) balance(ctx context.Context, owner identity.OwnerID, accountID string) (Balance, error) {
	var result Balance
	var amountText string
	err := r.pool.QueryRow(ctx, balanceSQL, owner.String(), accountID).Scan(&result.AccountID, &result.Currency, &amountText)
	if err != nil {
		return Balance{}, notFoundOr("read balance", err)
	}
	result.AmountMinor, err = parseMoney(amountText)
	return result, err
}

const balanceSQL = `SELECT a.id, a.currency,
    (a.opening_balance_minor::numeric + COALESCE((
        SELECT SUM(m.amount_minor) FROM account_movements m
        JOIN financial_events e ON e.owner_id = m.owner_id AND e.id = m.event_id
        WHERE m.owner_id = a.owner_id AND m.account_id = a.id AND e.status = 'posted' AND e.deleted_at IS NULL
        AND (a.opening_balance_at IS NULL OR e.occurred_at >= a.opening_balance_at)
    ), 0))::text
    FROM finance_accounts a WHERE a.owner_id = $1 AND a.id = $2`

func (r *repository) categorySpending(ctx context.Context, owner identity.OwnerID, categoryID, currencyCode string) (int64, error) {
	var amountText string
	err := r.pool.QueryRow(ctx, `SELECT COALESCE((
		SELECT SUM(ca.amount_minor) FROM category_allocations ca
		JOIN financial_events e ON e.owner_id = ca.owner_id AND e.id = ca.event_id
		WHERE ca.owner_id = c.owner_id AND ca.category_id = c.id
		AND e.kind = 'expense' AND e.status = 'posted' AND e.deleted_at IS NULL AND e.currency = $3
	), 0)::text FROM finance_categories c WHERE c.owner_id = $1 AND c.id = $2 AND c.kind = 'expense'`, owner.String(), categoryID, currencyCode).Scan(&amountText)
	if err != nil {
		return 0, notFoundOr("category spending", err)
	}
	return parseMoney(amountText)
}

func (r *repository) history(ctx context.Context, owner identity.OwnerID, limit, offset int) ([]Event, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, kind, amount_minor, currency, occurred_at, note, status, voided_at, void_reason
		FROM financial_events WHERE owner_id = $1 AND deleted_at IS NULL
		ORDER BY occurred_at DESC, id DESC LIMIT $2 OFFSET $3`, owner.String(), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}
	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		var voided sql.NullTime
		var reason sql.NullString
		if err := rows.Scan(&event.ID, &event.Kind, &event.AmountMinor, &event.Currency, &event.OccurredAt,
			&event.Note, &event.Status, &voided, &reason); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if voided.Valid {
			event.VoidedAt = &voided.Time
		}
		if reason.Valid {
			event.VoidReason = reason.String
		}
		events = append(events, event)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("read history rows: %w", err)
	}
	for i := range events {
		events[i].Movements, err = r.eventMovements(ctx, owner, events[i].ID)
		if err != nil {
			return nil, err
		}
		events[i].Allocations, err = r.eventAllocations(ctx, owner, events[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return events, nil
}

func (r *repository) eventMovements(ctx context.Context, owner identity.OwnerID, eventID string) ([]Movement, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, account_id, amount_minor, currency FROM account_movements
		WHERE owner_id = $1 AND event_id = $2 ORDER BY created_at, id`, owner.String(), eventID)
	if err != nil {
		return nil, fmt.Errorf("read movements: %w", err)
	}
	defer rows.Close()
	movements := make([]Movement, 0)
	for rows.Next() {
		var movement Movement
		if err := rows.Scan(&movement.ID, &movement.AccountID, &movement.AmountMinor, &movement.Currency); err != nil {
			return nil, fmt.Errorf("scan movement: %w", err)
		}
		movements = append(movements, movement)
	}
	return movements, rows.Err()
}

func (r *repository) eventAllocations(ctx context.Context, owner identity.OwnerID, eventID string) ([]Allocation, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, category_id, amount_minor, currency FROM category_allocations
		WHERE owner_id = $1 AND event_id = $2 ORDER BY created_at, id`, owner.String(), eventID)
	if err != nil {
		return nil, fmt.Errorf("read allocations: %w", err)
	}
	defer rows.Close()
	allocations := make([]Allocation, 0)
	for rows.Next() {
		var allocation Allocation
		if err := rows.Scan(&allocation.ID, &allocation.CategoryID, &allocation.AmountMinor, &allocation.Currency); err != nil {
			return nil, fmt.Errorf("scan allocation: %w", err)
		}
		allocations = append(allocations, allocation)
	}
	return allocations, rows.Err()
}

func (r *repository) post(ctx context.Context, owner identity.OwnerID, spec postSpec) (Event, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Event{}, fmt.Errorf("begin financial event: %w", err)
	}
	defer tx.Rollback(ctx)
	states, err := lockAccounts(ctx, tx, owner, spec.movements, false)
	if err != nil {
		return Event{}, err
	}
	for _, movement := range spec.movements {
		state := states[movement.accountID]
		if state.currency != spec.currency {
			return Event{}, ErrCurrencyMismatch
		}
		if state.openingAt != nil && spec.occurredAt.Before(*state.openingAt) {
			continue
		}
		current, err := balanceInTx(ctx, tx, owner, movement.accountID)
		if err != nil {
			return Event{}, err
		}
		if _, err := addMoney(current, movement.amount); err != nil {
			return Event{}, err
		}
	}
	for _, allocation := range spec.allocations {
		var kind string
		var archived sql.NullTime
		err := tx.QueryRow(ctx, `SELECT kind, archived_at FROM finance_categories
			WHERE owner_id = $1 AND id = $2 FOR SHARE`, owner.String(), allocation.CategoryID).Scan(&kind, &archived)
		if err != nil {
			return Event{}, notFoundOr("find category", err)
		}
		if archived.Valid {
			return Event{}, ErrArchived
		}
		if kind != spec.kind {
			return Event{}, ErrInvalidInput
		}
	}
	id, err := newUUID()
	if err != nil {
		return Event{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO financial_events
		(id, owner_id, kind, amount_minor, currency, occurred_at, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, owner.String(), spec.kind,
		spec.amount, spec.currency, spec.occurredAt, spec.note)
	if err != nil {
		return Event{}, fmt.Errorf("insert financial event: %w", err)
	}
	event := Event{ID: id, Kind: spec.kind, AmountMinor: spec.amount, Currency: spec.currency,
		OccurredAt: spec.occurredAt, Note: spec.note, Status: "posted"}
	for _, planned := range spec.movements {
		movementID, err := newUUID()
		if err != nil {
			return Event{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO account_movements
			(id, owner_id, event_id, account_id, amount_minor, currency)
			VALUES ($1, $2, $3, $4, $5, $6)`, movementID, owner.String(), id,
			planned.accountID, planned.amount, spec.currency)
		if err != nil {
			return Event{}, fmt.Errorf("insert account movement: %w", err)
		}
		event.Movements = append(event.Movements, Movement{ID: movementID, AccountID: planned.accountID,
			AmountMinor: planned.amount, Currency: spec.currency})
	}
	for _, planned := range spec.allocations {
		allocationID, err := newUUID()
		if err != nil {
			return Event{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO category_allocations
			(id, owner_id, event_id, category_id, kind, amount_minor, currency)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, allocationID, owner.String(), id,
			planned.CategoryID, spec.kind, planned.AmountMinor, spec.currency)
		if err != nil {
			return Event{}, fmt.Errorf("insert category allocation: %w", err)
		}
		event.Allocations = append(event.Allocations, Allocation{ID: allocationID, CategoryID: planned.CategoryID,
			AmountMinor: planned.AmountMinor, Currency: spec.currency})
	}
	if err := bumpBalanceRevisions(ctx, tx, owner, spec.movements); err != nil {
		return Event{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Event{}, fmt.Errorf("commit financial event: %w", err)
	}
	return event, nil
}

func (r *repository) voidEvent(ctx context.Context, owner identity.OwnerID, eventID, reason string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin void: %w", err)
	}
	defer tx.Rollback(ctx)
	var status string
	var deleted bool
	var occurredAt time.Time
	err = tx.QueryRow(ctx, `SELECT status, deleted_at IS NOT NULL, occurred_at FROM financial_events WHERE owner_id = $1 AND id = $2 FOR UPDATE`, owner.String(), eventID).Scan(&status, &deleted, &occurredAt)
	if err != nil {
		return notFoundOr("find event", err)
	}
	if deleted || status != "posted" {
		return ErrConflict
	}
	movements, err := eventMovementSpecs(ctx, tx, owner, eventID)
	if err != nil {
		return err
	}
	if err := checkRemovalBalances(ctx, tx, owner, movements, occurredAt); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE financial_events SET status = 'voided', voided_at = now(), void_reason = $3
		WHERE owner_id = $1 AND id = $2 AND status = 'posted' AND deleted_at IS NULL`, owner.String(), eventID, reason)
	if err != nil {
		return fmt.Errorf("void event: %w", err)
	}
	if err := bumpBalanceRevisions(ctx, tx, owner, movements); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *repository) deleteEvent(ctx context.Context, owner identity.OwnerID, eventID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete: %w", err)
	}
	defer tx.Rollback(ctx)
	var status string
	var deleted bool
	var occurredAt time.Time
	err = tx.QueryRow(ctx, `SELECT status, deleted_at IS NOT NULL, occurred_at FROM financial_events WHERE owner_id = $1 AND id = $2 FOR UPDATE`, owner.String(), eventID).Scan(&status, &deleted, &occurredAt)
	if err != nil {
		return notFoundOr("find event", err)
	}
	if deleted {
		return tx.Commit(ctx)
	}
	movements, err := eventMovementSpecs(ctx, tx, owner, eventID)
	if err != nil {
		return err
	}
	if status == "posted" {
		if err = checkRemovalBalances(ctx, tx, owner, movements, occurredAt); err != nil {
			return err
		}
	} else if status == "voided" {
		if _, err = lockAccounts(ctx, tx, owner, movements, true); err != nil {
			return err
		}
	} else {
		return ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE financial_events SET deleted_at = now()
		WHERE owner_id = $1 AND id = $2 AND deleted_at IS NULL`, owner.String(), eventID)
	if err != nil {
		return fmt.Errorf("delete event: %w", err)
	}
	if err := bumpBalanceRevisions(ctx, tx, owner, movements); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func eventMovementSpecs(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, eventID string) ([]movementSpec, error) {
	rows, err := tx.Query(ctx, `SELECT account_id, amount_minor FROM account_movements
		WHERE owner_id = $1 AND event_id = $2`, owner.String(), eventID)
	if err != nil {
		return nil, fmt.Errorf("read event movements: %w", err)
	}
	var movements []movementSpec
	for rows.Next() {
		var movement movementSpec
		if err := rows.Scan(&movement.accountID, &movement.amount); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan event movement: %w", err)
		}
		movements = append(movements, movement)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("read event movements: %w", err)
	}
	if len(movements) == 0 {
		return nil, ErrConflict
	}
	return movements, nil
}

func checkRemovalBalances(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, movements []movementSpec, occurredAt time.Time) error {
	states, err := lockAccounts(ctx, tx, owner, movements, true)
	if err != nil {
		return err
	}
	for _, movement := range movements {
		if at := states[movement.accountID].openingAt; at != nil && occurredAt.Before(*at) {
			continue
		}
		current, err := balanceInTx(ctx, tx, owner, movement.accountID)
		if err != nil {
			return err
		}
		if _, err := subtractMoney(current, movement.amount); err != nil {
			return err
		}
	}
	return nil
}

func lockAccounts(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, movements []movementSpec, allowArchived bool) (map[string]accountState, error) {
	ids := make([]string, 0, len(movements))
	for _, movement := range movements {
		ids = append(ids, movement.accountID)
	}
	sort.Strings(ids)
	states := make(map[string]accountState, len(ids))
	for _, id := range ids {
		if _, seen := states[id]; seen {
			return nil, ErrInvalidInput
		}
		var state accountState
		err := tx.QueryRow(ctx, `SELECT currency, archived_at, opening_balance_at FROM finance_accounts
			WHERE owner_id = $1 AND id = $2 FOR UPDATE`, owner.String(), id).Scan(&state.currency, &state.archived, &state.openingAt)
		if err != nil {
			return nil, notFoundOr("find account", err)
		}
		if state.archived.Valid && !allowArchived {
			return nil, ErrArchived
		}
		states[id] = state
	}
	return states, nil
}

func balanceInTx(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, accountID string) (int64, error) {
	var id, currency, amountText string
	err := tx.QueryRow(ctx, balanceSQL, owner.String(), accountID).Scan(&id, &currency, &amountText)
	if err != nil {
		return 0, notFoundOr("read locked balance", err)
	}
	return parseMoney(amountText)
}

func addMoney(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, ErrOverflow
	}
	return a + b, nil
}

func parseMoney(value string) (int64, error) {
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok || !parsed.IsInt64() {
		return 0, ErrOverflow
	}
	return parsed.Int64(), nil
}

func notFoundOr(action string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	return nil
}
