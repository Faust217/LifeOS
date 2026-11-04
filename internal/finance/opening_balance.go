package finance

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/jackc/pgx/v5"
)

// OpeningPreview is an explicit complete-data application projection, not a row.
type OpeningPreview struct {
	AccountID              string    `json:"account_id"`
	Currency               string    `json:"currency"`
	PreviousBasis          string    `json:"previous_balance_basis"`
	ResultingBasis         string    `json:"resulting_balance_basis"`
	OpeningBalanceAt       time.Time `json:"opening_balance_at"`
	PreviousOpeningMinor   string    `json:"previous_opening_balance_minor"`
	BalanceBeforeMinor     string    `json:"balance_before_minor"`
	SuggestedOpeningMinor  string    `json:"suggested_opening_balance_minor"`
	SuggestedRepresentable bool      `json:"suggested_representable"`
	ChosenOpeningMinor     *string   `json:"chosen_opening_balance_minor"`
	BalanceAfterMinor      *string   `json:"balance_after_minor"`
	BalanceChangeMinor     *string   `json:"balance_change_minor"`
	BeforeCount            int64     `json:"before_count"`
	BeforeNetMinor         string    `json:"before_net_minor"`
	OnOrAfterCount         int64     `json:"on_or_after_count"`
	OnOrAfterNetMinor      string    `json:"on_or_after_net_minor"`
	BalanceRevision        string    `json:"balance_revision"`
}

func validOpeningShape(at *time.Time) bool {
	return at != nil && !at.IsZero() && at.Year() >= 1 && at.Year() <= 9999
}
func validOpeningInstant(at *time.Time) bool { return validOpeningShape(at) && !at.After(time.Now()) }

func (s *Service) PreviewOpeningBalance(ctx context.Context, owner identity.OwnerID, id string, at time.Time, chosen *int64) (OpeningPreview, error) {
	if !owner.Valid() {
		return OpeningPreview{}, ErrUnauthorized
	}
	if !validID(id) || !validOpeningInstant(&at) {
		return OpeningPreview{}, ErrInvalidInput
	}
	tx, err := s.repo.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return OpeningPreview{}, err
	}
	defer tx.Rollback(ctx)
	if err = requireOpeningZone(ctx, tx, owner); err != nil {
		return OpeningPreview{}, err
	}
	account, err := readOpeningAccount(ctx, tx, owner, id, false)
	if err != nil {
		return OpeningPreview{}, err
	}
	result, err := previewOpening(ctx, tx, owner, account, at.UTC().Truncate(time.Microsecond), chosen)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func (s *Service) ConfirmOpeningBalance(ctx context.Context, owner identity.OwnerID, id string, at time.Time, amount, revision int64, acknowledge bool, requestKey string) (Account, error) {
	if !owner.Valid() {
		return Account{}, ErrUnauthorized
	}
	if !validID(id) || !validOpeningShape(&at) || revision < 0 {
		return Account{}, ErrInvalidInput
	}
	at = at.UTC().Truncate(time.Microsecond)
	id = strings.ToLower(id)
	cmd := financialCommand{operation: "opening_balance.confirm", target: id, parameters: []any{canonicalInstant(at), strconv.FormatInt(amount, 10), strconv.FormatInt(revision, 10), acknowledge}}
	return executeFinancialWrite(ctx, s.repo, owner, requestKey, cmd, func(ctx context.Context, tx pgx.Tx) (Account, error) {
		return confirmOpeningBalanceInTx(ctx, tx, owner, id, at, amount, revision, acknowledge)
	})
}

func confirmOpeningBalanceInTx(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, id string, at time.Time, amount, revision int64, acknowledge bool) (Account, error) {
	if !validOpeningInstant(&at) {
		return Account{}, ErrInvalidInput
	}

	if err := requireOpeningZone(ctx, tx, owner); err != nil {
		return Account{}, err
	}
	account, err := readOpeningAccount(ctx, tx, owner, id, true)
	if err != nil {
		return Account{}, err
	}
	if account.OpeningBalanceAt != nil || account.BalanceRevision != revision {
		return Account{}, ErrConflict
	}
	preview, err := previewOpening(ctx, tx, owner, account, at, &amount)
	if err != nil {
		return Account{}, err
	}
	if *preview.BalanceChangeMinor != "0" && !acknowledge {
		return Account{}, ErrInvalidInput
	}
	evidenceID, err := newUUID()
	if err != nil {
		return Account{}, err
	}
	var confirmed time.Time
	err = tx.QueryRow(ctx, `UPDATE finance_accounts SET opening_balance_minor=$3,opening_balance_at=$4,opening_balance_confirmed_at=clock_timestamp(),balance_revision=balance_revision+1,updated_at=clock_timestamp() WHERE owner_id=$1 AND id=$2 RETURNING opening_balance_confirmed_at`, owner.String(), id, amount, at).Scan(&confirmed)
	if err != nil {
		return Account{}, err
	}
	choice := "known_balance"
	if strconv.FormatInt(amount, 10) == preview.SuggestedOpeningMinor {
		choice = "preserve"
	}
	previousBalance, _ := strconv.ParseInt(preview.BalanceBeforeMinor, 10, 64)
	resultingBalance, _ := strconv.ParseInt(*preview.BalanceAfterMinor, 10, 64)
	_, err = tx.Exec(ctx, `INSERT INTO finance_opening_confirmations(id,owner_id,account_id,previous_opening_balance_minor,previous_balance_minor,opening_balance_at,opening_balance_minor,suggested_opening_balance_minor,resulting_balance_minor,assertion_choice,previous_revision,confirmed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8::text::numeric,$9,$10,$11,$12)`, evidenceID, owner.String(), id, account.OpeningBalanceMinor, previousBalance, at, amount, preview.SuggestedOpeningMinor, resultingBalance, choice, revision, confirmed)
	if err != nil {
		return Account{}, err
	}
	account.OpeningBalanceMinor = amount
	account.OpeningBalanceAt = &at
	account.OpeningConfirmedAt = &confirmed
	account.BalanceRevision++
	return account, bumpHistoryRevision(ctx, tx, owner)
}

func requireOpeningZone(ctx context.Context, tx pgx.Tx, owner identity.OwnerID) error {
	profile, err := identity.ExportProfile(ctx, tx, owner)
	if err != nil {
		return err
	}
	if profile.TimeZone == nil {
		return ErrInvalidInput
	}
	return nil
}

func readOpeningAccount(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, id string, lock bool) (Account, error) {
	q := `SELECT id,name,account_type,currency,opening_balance_minor,archived_at,opening_balance_at,opening_balance_confirmed_at,balance_revision FROM finance_accounts WHERE owner_id=$1 AND id=$2`
	if lock {
		q += " FOR UPDATE"
	}
	var a Account
	err := tx.QueryRow(ctx, q, owner.String(), id).Scan(&a.ID, &a.Name, &a.Type, &a.Currency, &a.OpeningBalanceMinor, &a.ArchivedAt, &a.OpeningBalanceAt, &a.OpeningConfirmedAt, &a.BalanceRevision)
	return a, notFoundOr("read opening account", err)
}

func previewOpening(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, a Account, at time.Time, chosen *int64) (OpeningPreview, error) {
	p := OpeningPreview{AccountID: a.ID, Currency: a.Currency, OpeningBalanceAt: at, PreviousOpeningMinor: strconv.FormatInt(a.OpeningBalanceMinor, 10), BalanceRevision: strconv.FormatInt(a.BalanceRevision, 10)}
	p.PreviousBasis, p.ResultingBasis = "legacy_all_movements", "known_balance_before_instant"
	if a.OpeningBalanceAt != nil {
		return p, ErrConflict
	}
	err := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE e.occurred_at<$3),COALESCE(sum(m.amount_minor::numeric) FILTER(WHERE e.occurred_at<$3),0)::text,count(*) FILTER(WHERE e.occurred_at>=$3),COALESCE(sum(m.amount_minor::numeric) FILTER(WHERE e.occurred_at>=$3),0)::text FROM account_movements m JOIN financial_events e ON e.owner_id=m.owner_id AND e.id=m.event_id WHERE m.owner_id=$1 AND m.account_id=$2 AND e.status='posted' AND e.deleted_at IS NULL`, owner.String(), a.ID, at).Scan(&p.BeforeCount, &p.BeforeNetMinor, &p.OnOrAfterCount, &p.OnOrAfterNetMinor)
	if err != nil {
		return p, err
	}
	before, _ := new(big.Int).SetString(p.BeforeNetMinor, 10)
	after, _ := new(big.Int).SetString(p.OnOrAfterNetMinor, 10)
	suggested := new(big.Int).Add(big.NewInt(a.OpeningBalanceMinor), before)
	old := new(big.Int).Add(new(big.Int).Set(suggested), after)
	if !old.IsInt64() {
		return p, ErrOverflow
	}
	p.BalanceBeforeMinor = old.String()
	p.SuggestedOpeningMinor = suggested.String()
	p.SuggestedRepresentable = suggested.IsInt64()
	if chosen == nil && !suggested.IsInt64() {
		return p, nil
	}
	amount := suggested.Int64()
	if chosen != nil {
		amount = *chosen
	}
	result := new(big.Int).Add(big.NewInt(amount), after)
	if !result.IsInt64() {
		return p, ErrOverflow
	}
	delta := new(big.Int).Sub(result, old)
	value, resultText, deltaText := strconv.FormatInt(amount, 10), result.String(), delta.String()
	p.ChosenOpeningMinor = &value
	p.BalanceAfterMinor = &resultText
	p.BalanceChangeMinor = &deltaText
	return p, nil
}

func bumpBalanceRevisions(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, movements []movementSpec) error {
	ids := make([]string, 0, len(movements))
	for _, m := range movements {
		ids = append(ids, m.accountID)
	}
	_, err := tx.Exec(ctx, `UPDATE finance_accounts SET balance_revision=balance_revision+1 WHERE owner_id=$1 AND id=ANY($2::uuid[])`, owner.String(), ids)
	if err != nil {
		return fmt.Errorf("advance account balance revision: %w", err)
	}
	return nil
}

func subtractMoney(a, b int64) (int64, error) {
	result := new(big.Int).Sub(big.NewInt(a), big.NewInt(b))
	if !result.IsInt64() {
		return 0, ErrOverflow
	}
	return result.Int64(), nil
}

func (a Account) OpeningBasis() string {
	if a.OpeningBalanceAt == nil {
		return "legacy_all_movements"
	}
	return "known_balance_before_instant"
}
