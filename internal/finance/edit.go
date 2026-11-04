package finance

import (
	"context"
	"errors"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/jackc/pgx/v5"
)

var (
	ErrRevisionRequired = errors.New("event revision required")
	ErrRevisionConflict = errors.New("event revision conflict")
	ErrNotEditable      = errors.New("event is not editable")
)

func requestedRevision(values []int64) int64 {
	if len(values) == 1 && values[0] > 0 {
		return values[0]
	}
	return 0
}

type EditInput struct {
	Kind string
	EventInput
	FromAccountID string
	ToAccountID   string
}

func (s *Service) GetEvent(ctx context.Context, owner identity.OwnerID, id string) (Event, error) {
	if !owner.Valid() {
		return Event{}, ErrUnauthorized
	}
	if !validID(id) {
		return Event{}, ErrInvalidInput
	}
	return s.repo.readEvent(ctx, owner, strings.ToLower(id))
}

func (s *Service) EditEvent(ctx context.Context, owner identity.OwnerID, id string, expected int64, input EditInput, key string) (Event, error) {
	if !owner.Valid() {
		return Event{}, ErrUnauthorized
	}
	if !validID(id) || expected <= 0 || !validEventInput(input.AmountMinor, input.Currency, input.OccurredAt, input.Note) {
		return Event{}, ErrInvalidInput
	}
	id = strings.ToLower(id)
	spec := postSpec{kind: input.Kind, amount: input.AmountMinor, currency: input.Currency, occurredAt: input.OccurredAt.UTC().Truncate(time.Microsecond), note: input.Note, requestKey: key}
	params := []any{strconv.FormatInt(expected, 10), input.Kind, input.Currency}
	if input.Kind == "transfer" {
		from, to := strings.ToLower(input.FromAccountID), strings.ToLower(input.ToAccountID)
		if !validID(from) || !validID(to) || from == to || input.AccountID != "" || len(input.Allocations) != 0 {
			return Event{}, ErrInvalidInput
		}
		spec.movements = []movementSpec{{from, -input.AmountMinor}, {to, input.AmountMinor}}
		params = append(params, from, to, strconv.FormatInt(input.AmountMinor, 10), canonicalInstant(input.OccurredAt), input.Note)
	} else {
		account := strings.ToLower(input.AccountID)
		if (input.Kind != "income" && input.Kind != "expense") || !validID(account) || input.FromAccountID != "" || input.ToAccountID != "" {
			return Event{}, ErrInvalidInput
		}
		parts := append([]AllocationInput{}, input.Allocations...)
		for i := range parts {
			parts[i].CategoryID = strings.ToLower(parts[i].CategoryID)
		}
		sort.Slice(parts, func(i, j int) bool { return parts[i].CategoryID < parts[j].CategoryID })
		sum := new(big.Int)
		pairs := []any{}
		for i, a := range parts {
			if !validID(a.CategoryID) || a.AmountMinor <= 0 || (i > 0 && parts[i-1].CategoryID == a.CategoryID) {
				return Event{}, ErrInvalidInput
			}
			sum.Add(sum, big.NewInt(a.AmountMinor))
			pairs = append(pairs, []any{a.CategoryID, strconv.FormatInt(a.AmountMinor, 10)})
		}
		if len(parts) > 0 && sum.Cmp(big.NewInt(input.AmountMinor)) != 0 {
			return Event{}, ErrInvalidInput
		}
		amount := input.AmountMinor
		if input.Kind == "expense" {
			amount = -amount
		}
		spec.movements = []movementSpec{{account, amount}}
		spec.allocations = parts
		params = append(params, account, strconv.FormatInt(input.AmountMinor, 10), canonicalInstant(input.OccurredAt), input.Note, pairs)
	}
	cmd := financialCommand{version: 2, operation: "event.edit", target: id, parameters: params}
	return executeFinancialWrite(ctx, s.repo, owner, key, cmd, func(ctx context.Context, tx pgx.Tx) (Event, error) {
		return s.repo.editEventInTx(ctx, tx, owner, id, expected, spec)
	})
}

func captureRevision(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, id, action, key string) error {
	_, err := tx.Exec(ctx, `SELECT capture_financial_event_revision($1,$2,$3,$4)`, owner.String(), id, action, key)
	return err
}

func (r *repository) editEventInTx(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, id string, expected int64, spec postSpec) (Event, error) {
	old, deleted, err := readEventInTx(ctx, tx, owner, id, true)
	if err != nil {
		return Event{}, err
	}
	if deleted {
		return Event{}, ErrNotFound
	}
	if old.Revision != expected {
		return Event{}, ErrRevisionConflict
	}
	if old.Status != "posted" {
		return Event{}, ErrNotEditable
	}
	if old.Kind != spec.kind || old.Currency != spec.currency {
		return Event{}, ErrInvalidInput
	}
	if old.Revision == math.MaxInt64 {
		return Event{}, ErrOverflow
	}
	oldAccounts := map[string]Movement{}
	nextAccounts := map[string]movementSpec{}
	for _, m := range old.Movements {
		oldAccounts[m.AccountID] = m
	}
	union := map[string]bool{}
	for id := range oldAccounts {
		union[id] = true
	}
	for _, m := range spec.movements {
		union[m.accountID] = true
		nextAccounts[m.accountID] = m
	}
	ids := make([]string, 0, len(union))
	for id := range union {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	lockSpecs := make([]movementSpec, 0, len(ids))
	for _, id := range ids {
		lockSpecs = append(lockSpecs, movementSpec{accountID: id})
	}
	states, err := lockAccounts(ctx, tx, owner, lockSpecs, true)
	if err != nil {
		return Event{}, err
	}
	for _, id := range ids {
		state := states[id]
		next, used := nextAccounts[id]
		previous, retained := oldAccounts[id]
		if used && state.currency != spec.currency {
			return Event{}, ErrCurrencyMismatch
		}
		if used && state.archived.Valid && !retained {
			return Event{}, ErrArchived
		}
		balance, err := balanceInTx(ctx, tx, owner, id)
		if err != nil {
			return Event{}, err
		}
		final := big.NewInt(balance)
		if retained && (state.openingAt == nil || !old.OccurredAt.Before(*state.openingAt)) {
			final.Sub(final, big.NewInt(previous.AmountMinor))
		}
		if used && (state.openingAt == nil || !spec.occurredAt.Before(*state.openingAt)) {
			final.Add(final, big.NewInt(next.amount))
		}
		if !final.IsInt64() {
			return Event{}, ErrOverflow
		}
	}
	previousCategories := map[string]bool{}
	for _, a := range old.Allocations {
		previousCategories[a.CategoryID] = true
	}
	for _, a := range spec.allocations {
		var kind string
		var archived bool
		err := tx.QueryRow(ctx, `SELECT kind,archived_at IS NOT NULL FROM finance_categories WHERE owner_id=$1 AND id=$2 FOR SHARE`, owner.String(), a.CategoryID).Scan(&kind, &archived)
		if err != nil {
			return Event{}, notFoundOr("find edit category", err)
		}
		if kind != spec.kind {
			return Event{}, ErrInvalidInput
		}
		if archived && !previousCategories[a.CategoryID] {
			return Event{}, ErrArchived
		}
	}
	_, err = tx.Exec(ctx, `UPDATE financial_events SET amount_minor=$3,occurred_at=$4,note=$5,event_revision=event_revision+1 WHERE owner_id=$1 AND id=$2`, owner.String(), id, spec.amount, spec.occurredAt, spec.note)
	if err != nil {
		return Event{}, err
	}
	for account := range oldAccounts {
		if _, ok := nextAccounts[account]; !ok {
			if _, err = tx.Exec(ctx, `DELETE FROM account_movements WHERE owner_id=$1 AND event_id=$2 AND account_id=$3`, owner.String(), id, account); err != nil {
				return Event{}, err
			}
		}
	}
	for _, m := range spec.movements {
		childID, err := newUUID()
		if err != nil {
			return Event{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO account_movements(id,owner_id,event_id,account_id,amount_minor,currency) VALUES($1,$2,$3,$4,$5,$6)
            ON CONFLICT(owner_id,event_id,account_id) DO UPDATE SET amount_minor=EXCLUDED.amount_minor`, childID, owner.String(), id, m.accountID, m.amount, spec.currency)
		if err != nil {
			return Event{}, err
		}
	}
	categoryIDs := make([]string, 0, len(spec.allocations))
	for _, a := range spec.allocations {
		categoryIDs = append(categoryIDs, a.CategoryID)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM category_allocations WHERE owner_id=$1 AND event_id=$2 AND NOT(category_id=ANY($3::uuid[]))`, owner.String(), id, categoryIDs); err != nil {
		return Event{}, err
	}
	for _, a := range spec.allocations {
		childID, err := newUUID()
		if err != nil {
			return Event{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO category_allocations(id,owner_id,event_id,category_id,kind,amount_minor,currency) VALUES($1,$2,$3,$4,$5,$6,$7)
            ON CONFLICT(owner_id,event_id,category_id) DO UPDATE SET amount_minor=EXCLUDED.amount_minor`, childID, owner.String(), id, a.CategoryID, spec.kind, a.AmountMinor, spec.currency)
		if err != nil {
			return Event{}, err
		}
	}
	if err = bumpBalanceRevisions(ctx, tx, owner, lockSpecs); err != nil {
		return Event{}, err
	}
	if err = captureRevision(ctx, tx, owner, id, "edit", spec.requestKey); err != nil {
		return Event{}, err
	}
	if err = bumpHistoryRevision(ctx, tx, owner); err != nil {
		return Event{}, err
	}
	result, _, err := readEventInTx(ctx, tx, owner, id, false)
	return result, err
}
