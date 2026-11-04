package finance

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/jackc/pgx/v5"
)

var (
	ErrHistoryChanged     = errors.New("finance history changed")
	ErrHistoryCursor      = errors.New("invalid history cursor")
	ErrHistoryUnavailable = errors.New("finance history unavailable")
	historyTokenPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

const historyInstantLayout = "2006-01-02T15:04:05.000000Z"

type HistoryQuery struct {
	Limit            int
	Revision, Cursor string
}
type historyCursor struct {
	Revision string
	Limit    int
	At       time.Time
	ID       string
}
type HistoryPosition struct {
	MovementID        string  `json:"movement_id"`
	AccountID         string  `json:"account_id"`
	Currency          string  `json:"currency"`
	BalanceAfterMinor *string `json:"balance_after_minor"`
	Basis             string  `json:"basis"`
	UnavailableReason *string `json:"unavailable_reason"`
}
type HistoryAccount struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Currency         string     `json:"currency"`
	ArchivedAt       *time.Time `json:"archived_at"`
	BalanceBasis     string     `json:"balance_basis"`
	OpeningBalanceAt *time.Time `json:"opening_balance_at"`
}
type HistoryCategory struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	ArchivedAt *time.Time `json:"archived_at"`
}
type HistoryItem struct {
	Event     Event
	Positions []HistoryPosition
}
type HistoryPage struct {
	Revision   string
	TimeZone   *string
	Limit      int
	HasMore    bool
	NextCursor *string
	Items      []HistoryItem
	Accounts   []HistoryAccount
	Categories []HistoryCategory
}

func historyProjectionToken(owner string, epoch *string, revision int64, zone *string) string {
	data, _ := json.Marshal([]any{"lifeos-finance-history-v1", owner, epoch, strconv.FormatInt(revision, 10), zone})
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
func encodeHistoryCursor(c historyCursor) string {
	data, _ := json.Marshal([]any{1, c.Revision, c.Limit, c.At.UTC().Format(historyInstantLayout), c.ID})
	return base64.RawURLEncoding.EncodeToString(data)
}
func decodeHistoryCursor(value string) (historyCursor, error) {
	var c historyCursor
	if len(value) > 384 {
		return c, ErrHistoryCursor
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil {
		return c, ErrHistoryCursor
	}
	var parts []json.RawMessage
	if json.Unmarshal(data, &parts) != nil || len(parts) != 5 {
		return c, ErrHistoryCursor
	}
	var version int
	var instant string
	if json.Unmarshal(parts[0], &version) != nil || version != 1 || json.Unmarshal(parts[1], &c.Revision) != nil || json.Unmarshal(parts[2], &c.Limit) != nil || json.Unmarshal(parts[3], &instant) != nil || json.Unmarshal(parts[4], &c.ID) != nil {
		return c, ErrHistoryCursor
	}
	c.At, err = time.Parse(historyInstantLayout, instant)
	if err != nil || !validID(c.ID) || c.ID != strings.ToLower(c.ID) || c.Limit < 1 || c.Limit > 100 || !historyTokenPattern.MatchString(c.Revision) || encodeHistoryCursor(c) != value {
		return c, ErrHistoryCursor
	}
	return c, nil
}

func (s *Service) HistoryPage(parent context.Context, owner identity.OwnerID, q HistoryQuery) (HistoryPage, error) {
	if !owner.Valid() {
		return HistoryPage{}, ErrUnauthorized
	}
	if q.Limit < 1 || q.Limit > 100 || (q.Revision != "" && !historyTokenPattern.MatchString(q.Revision)) {
		return HistoryPage{}, ErrInvalidInput
	}
	var cursor *historyCursor
	if q.Cursor != "" {
		c, err := decodeHistoryCursor(q.Cursor)
		if err != nil || q.Revision == "" || c.Revision != q.Revision || c.Limit != q.Limit {
			return HistoryPage{}, ErrHistoryCursor
		}
		cursor = &c
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	result, err := s.repo.historyPage(ctx, owner, q, cursor)
	if err != nil && !errors.Is(err, ErrHistoryChanged) && !errors.Is(err, ErrUnauthorized) {
		return HistoryPage{}, ErrHistoryUnavailable
	}
	return result, err
}

func (r *repository) historyPage(ctx context.Context, owner identity.OwnerID, q HistoryQuery, cursor *historyCursor) (HistoryPage, error) {
	result := HistoryPage{Limit: q.Limit, Items: []HistoryItem{}, Accounts: []HistoryAccount{}, Categories: []HistoryCategory{}}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	if _, err = tx.Exec(ctx, `SET LOCAL statement_timeout='4s'; SET LOCAL idle_in_transaction_session_timeout='6s'`); err != nil {
		return result, err
	}
	var epoch *string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT epoch::text,revision FROM finance_history_revisions WHERE owner_id=$1`, owner.String()).Scan(&epoch, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM finance_accounts WHERE owner_id=$1 UNION ALL SELECT 1 FROM finance_categories WHERE owner_id=$1 UNION ALL SELECT 1 FROM financial_events WHERE owner_id=$1)`, owner.String()).Scan(&exists); err != nil {
			return result, err
		}
		if exists {
			return result, ErrHistoryUnavailable
		}
	} else if err != nil {
		return result, err
	}
	profile, err := identity.ExportProfile(ctx, tx, owner)
	if err != nil {
		return result, err
	}
	result.TimeZone = profile.TimeZone
	result.Revision = historyProjectionToken(owner.String(), epoch, revision, profile.TimeZone)
	if q.Revision != "" && q.Revision != result.Revision {
		return HistoryPage{}, ErrHistoryChanged
	}
	query := `SELECT id FROM financial_events WHERE owner_id=$1 AND deleted_at IS NULL`
	args := []any{owner.String(), q.Limit + 1}
	if cursor != nil {
		query += ` AND (occurred_at,id)<($3,$4::uuid)`
		args = append(args, cursor.At, cursor.ID)
	}
	query += ` ORDER BY occurred_at DESC,id DESC LIMIT $2`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return result, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return result, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	result.HasMore = len(ids) > q.Limit
	if result.HasMore {
		ids = ids[:q.Limit]
	}
	accountIDs, categoryIDs := map[string]bool{}, map[string]bool{}
	for _, id := range ids {
		event, deleted, err := readEventInTx(ctx, tx, owner, id, false)
		if err != nil || deleted {
			return result, ErrHistoryUnavailable
		}
		for _, m := range event.Movements {
			accountIDs[m.AccountID] = true
		}
		for _, a := range event.Allocations {
			categoryIDs[a.CategoryID] = true
		}
		result.Items = append(result.Items, HistoryItem{Event: event, Positions: []HistoryPosition{}})
	}
	result.Accounts, result.Categories, err = readHistoryContexts(ctx, tx, owner, sortedHistoryIDs(accountIDs), sortedHistoryIDs(categoryIDs))
	if err != nil {
		return result, err
	}
	accounts := map[string]HistoryAccount{}
	anchored := []string{}
	for _, a := range result.Accounts {
		accounts[a.ID] = a
		if a.OpeningBalanceAt != nil {
			anchored = append(anchored, a.ID)
		}
	}
	balances, err := readRunningBalancesInTx(ctx, tx, owner, anchored, ids)
	if err != nil {
		return result, err
	}
	for i := range result.Items {
		event := result.Items[i].Event
		movements := append([]Movement{}, event.Movements...)
		if event.Kind == "transfer" {
			sort.Slice(movements, func(i, j int) bool { return movements[i].AmountMinor < movements[j].AmountMinor })
		}
		for _, m := range movements {
			account, ok := accounts[m.AccountID]
			if !ok || account.Currency != m.Currency || m.Currency != event.Currency {
				return result, ErrHistoryUnavailable
			}
			p := HistoryPosition{MovementID: m.ID, AccountID: m.AccountID, Currency: m.Currency, Basis: "legacy_unconfirmed"}
			if account.OpeningBalanceAt != nil {
				p.Basis = "anchored"
				if event.OccurredAt.Before(*account.OpeningBalanceAt) {
					p.Basis = "reconstructed_pre_anchor"
				}
			}
			reason := ""
			if event.Status == "voided" {
				reason = "voided"
			} else if account.OpeningBalanceAt == nil {
				reason = "anchor_required"
			} else {
				value, ok := balances[m.ID]
				if !ok {
					return result, ErrHistoryUnavailable
				}
				exact, ok := new(big.Int).SetString(value, 10)
				if !ok {
					return result, ErrHistoryUnavailable
				}
				if !exact.IsInt64() {
					reason = "amount_out_of_range"
				} else {
					canonical := exact.String()
					p.BalanceAfterMinor = &canonical
				}
			}
			if reason != "" {
				p.UnavailableReason = &reason
			}
			result.Items[i].Positions = append(result.Items[i].Positions, p)
		}
	}
	if result.HasMore {
		last := result.Items[len(result.Items)-1].Event
		next := encodeHistoryCursor(historyCursor{result.Revision, q.Limit, last.OccurredAt, last.ID})
		result.NextCursor = &next
	}
	return result, tx.Commit(ctx)
}

func sortedHistoryIDs(values map[string]bool) []string {
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func readHistoryContexts(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, accounts, categories []string) ([]HistoryAccount, []HistoryCategory, error) {
	accountRows := []HistoryAccount{}
	categoryRows := []HistoryCategory{}
	rows, err := tx.Query(ctx, `SELECT id,name,currency,archived_at,opening_balance_at FROM finance_accounts WHERE owner_id=$1 AND id=ANY($2::uuid[]) ORDER BY id`, owner.String(), accounts)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var a HistoryAccount
		if err = rows.Scan(&a.ID, &a.Name, &a.Currency, &a.ArchivedAt, &a.OpeningBalanceAt); err != nil {
			rows.Close()
			return nil, nil, err
		}
		a.BalanceBasis = "legacy_all_movements"
		if a.ArchivedAt != nil {
			utc := a.ArchivedAt.UTC()
			a.ArchivedAt = &utc
		}
		if a.OpeningBalanceAt != nil {
			utc := a.OpeningBalanceAt.UTC()
			a.OpeningBalanceAt = &utc
			a.BalanceBasis = "known_balance_before_instant"
		}
		accountRows = append(accountRows, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	rows, err = tx.Query(ctx, `SELECT id,name,kind,archived_at FROM finance_categories WHERE owner_id=$1 AND id=ANY($2::uuid[]) ORDER BY id`, owner.String(), categories)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var c HistoryCategory
		if err = rows.Scan(&c.ID, &c.Name, &c.Kind, &c.ArchivedAt); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if c.ArchivedAt != nil {
			utc := c.ArchivedAt.UTC()
			c.ArchivedAt = &utc
		}
		categoryRows = append(categoryRows, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if len(accountRows) != len(accounts) || len(categoryRows) != len(categories) {
		return nil, nil, ErrHistoryUnavailable
	}
	return accountRows, categoryRows, nil
}

// Window expressions form the required complete-partition boundary. Let PG
// inline the CTE: an extra forced materialization only spills the derived rows.
const runningBalancesSQL = `WITH ledger AS (
 SELECT m.id AS movement_id,e.id AS event_id,a.opening_balance_minor::numeric AS opening,
 SUM(m.amount_minor::numeric) OVER(PARTITION BY m.account_id ORDER BY e.occurred_at,e.id ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS prefix,
 SUM(m.amount_minor::numeric) FILTER(WHERE e.occurred_at<a.opening_balance_at) OVER(PARTITION BY m.account_id) AS before_anchor
 FROM account_movements m JOIN financial_events e ON e.owner_id=m.owner_id AND e.id=m.event_id
 JOIN finance_accounts a ON a.owner_id=m.owner_id AND a.id=m.account_id
 WHERE m.owner_id=$1 AND m.account_id=ANY($2::uuid[]) AND e.status='posted' AND e.deleted_at IS NULL
) SELECT movement_id,(opening+prefix-COALESCE(before_anchor,0))::text FROM ledger WHERE event_id=ANY($3::uuid[])`

func readRunningBalancesInTx(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, accounts, events []string) (map[string]string, error) {
	result := map[string]string{}
	if len(accounts) == 0 || len(events) == 0 {
		return result, nil
	}
	rows, err := tx.Query(ctx, runningBalancesSQL, owner.String(), accounts, events)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, value string
		if err = rows.Scan(&id, &value); err != nil {
			return nil, err
		}
		result[id] = value
	}
	return result, rows.Err()
}
