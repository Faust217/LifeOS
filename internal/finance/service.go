package finance

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/currency"
)

var (
	ErrExportVersionIncompatible = errors.New("account data requires export v2")
	ErrUnauthorized              = errors.New("verified owner required")
	ErrInvalidInput              = errors.New("invalid finance input")
	ErrNotFound                  = errors.New("finance record not found")
	ErrArchived                  = errors.New("account or category is archived")
	ErrCurrencyMismatch          = errors.New("account currency mismatch")
	ErrOverflow                  = errors.New("money arithmetic overflow")
	ErrConflict                  = errors.New("finance event cannot be changed")
)

type Account struct {
	ID                  string
	Name                string
	Type                string
	Currency            string
	OpeningBalanceMinor int64
	ArchivedAt          *time.Time
	OpeningBalanceAt    *time.Time
	OpeningConfirmedAt  *time.Time
	BalanceRevision     int64
}

type Category struct {
	ID         string
	Name       string
	Kind       string
	ArchivedAt *time.Time
}

type Balance struct {
	AccountID   string
	Currency    string
	AmountMinor int64
}

type Movement struct {
	ID          string
	AccountID   string
	AmountMinor int64
	Currency    string
}

type Allocation struct {
	ID          string
	CategoryID  string
	AmountMinor int64
	Currency    string
}

type Event struct {
	Revision    int64
	ID          string
	Kind        string
	AmountMinor int64
	Currency    string
	OccurredAt  time.Time
	Note        string
	Status      string
	VoidedAt    *time.Time
	VoidReason  string
	Movements   []Movement
	Allocations []Allocation
}

type AccountInput struct {
	Name                string
	Type                string
	Currency            string
	OpeningBalanceMinor int64
	OpeningBalanceAt    *time.Time
}

type CategoryInput struct {
	Name string
	Kind string
}

type AllocationInput struct {
	CategoryID  string
	AmountMinor int64
}

type EventInput struct {
	AccountID   string
	AmountMinor int64
	Currency    string
	OccurredAt  time.Time
	Note        string
	Allocations []AllocationInput
}

type TransferInput struct {
	FromAccountID string
	ToAccountID   string
	AmountMinor   int64
	Currency      string
	OccurredAt    time.Time
	Note          string
}

type Service struct{ repo *repository }

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{repo: &repository{pool: pool, writeSlots: make(chan struct{}, 8), keyLimit: FinancialWriteKeyLimit, capacityObserver: observeCapacity}}
}

func (s *Service) CreateAccount(ctx context.Context, owner identity.OwnerID, input AccountInput, requestKey string) (Account, error) {
	if !owner.Valid() {
		return Account{}, ErrUnauthorized
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || !utf8.ValidString(input.Name) || len(input.Name) > 120 || !validCurrency(input.Currency) || !validAccountType(input.Type) || !validOpeningShape(input.OpeningBalanceAt) {
		return Account{}, ErrInvalidInput
	}
	at := input.OpeningBalanceAt.UTC().Truncate(time.Microsecond)
	input.OpeningBalanceAt = &at
	cmd := financialCommand{operation: "account.create", parameters: []any{input.Name, input.Type, input.Currency, strconv.FormatInt(input.OpeningBalanceMinor, 10), canonicalInstant(at)}}
	return executeFinancialWrite(ctx, s.repo, owner, requestKey, cmd, func(ctx context.Context, tx pgx.Tx) (Account, error) {
		if !validOpeningInstant(input.OpeningBalanceAt) {
			return Account{}, ErrInvalidInput
		}
		return s.repo.createAccountInTx(ctx, tx, owner, input)
	})
}

func (s *Service) CreateCategory(ctx context.Context, owner identity.OwnerID, input CategoryInput) (Category, error) {
	if !owner.Valid() {
		return Category{}, ErrUnauthorized
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 120 || strings.EqualFold(input.Name, "Uncategorized") || (input.Kind != "expense" && input.Kind != "income") {
		return Category{}, ErrInvalidInput
	}
	return s.repo.createCategory(ctx, owner, input)
}

func (s *Service) RenameCategory(ctx context.Context, owner identity.OwnerID, categoryID, name string) (Category, error) {
	if !owner.Valid() {
		return Category{}, ErrUnauthorized
	}
	name = strings.TrimSpace(name)
	if !validID(categoryID) || name == "" || len(name) > 120 || strings.EqualFold(name, "Uncategorized") {
		return Category{}, ErrInvalidInput
	}
	return s.repo.renameCategory(ctx, owner, categoryID, name)
}

func (s *Service) ArchiveCategory(ctx context.Context, owner identity.OwnerID, categoryID string) error {
	if !owner.Valid() {
		return ErrUnauthorized
	}
	if !validID(categoryID) {
		return ErrInvalidInput
	}
	return s.repo.archiveCategory(ctx, owner, categoryID)
}

func (s *Service) RestoreCategory(ctx context.Context, owner identity.OwnerID, categoryID string) error {
	if !owner.Valid() {
		return ErrUnauthorized
	}
	if !validID(categoryID) {
		return ErrInvalidInput
	}
	return s.repo.restoreCategory(ctx, owner, categoryID)
}

func (s *Service) PostExpense(ctx context.Context, owner identity.OwnerID, input EventInput, requestKey string) (Event, error) {
	return s.postSingleAccountEvent(ctx, owner, "expense", input, requestKey)
}

func (s *Service) PostIncome(ctx context.Context, owner identity.OwnerID, input EventInput, requestKey string) (Event, error) {
	return s.postSingleAccountEvent(ctx, owner, "income", input, requestKey)
}

func (s *Service) postSingleAccountEvent(ctx context.Context, owner identity.OwnerID, kind string, input EventInput, requestKey string) (Event, error) {
	if !owner.Valid() {
		return Event{}, ErrUnauthorized
	}
	if !validID(input.AccountID) || !validEventInput(input.AmountMinor, input.Currency, input.OccurredAt, input.Note) {
		return Event{}, ErrInvalidInput
	}
	input.AccountID = strings.ToLower(input.AccountID)
	input.Allocations = append([]AllocationInput{}, input.Allocations...)
	for i := range input.Allocations {
		input.Allocations[i].CategoryID = strings.ToLower(input.Allocations[i].CategoryID)
	}
	sort.Slice(input.Allocations, func(i, j int) bool { return input.Allocations[i].CategoryID < input.Allocations[j].CategoryID })
	var total int64
	seen := make(map[string]bool, len(input.Allocations))
	for _, allocation := range input.Allocations {
		if !validID(allocation.CategoryID) || allocation.AmountMinor <= 0 || seen[allocation.CategoryID] {
			return Event{}, ErrInvalidInput
		}
		seen[allocation.CategoryID] = true
		if allocation.AmountMinor > math.MaxInt64-total {
			return Event{}, ErrOverflow
		}
		total += allocation.AmountMinor
	}
	if len(input.Allocations) > 0 && total != input.AmountMinor {
		return Event{}, ErrInvalidInput
	}
	amount := input.AmountMinor
	if kind == "expense" {
		amount = -amount
	}
	pairs := make([]any, 0, len(input.Allocations))
	for _, a := range input.Allocations {
		pairs = append(pairs, []any{a.CategoryID, strconv.FormatInt(a.AmountMinor, 10)})
	}
	cmd := financialCommand{operation: "event.create", parameters: []any{kind, input.AccountID, strconv.FormatInt(input.AmountMinor, 10), input.Currency, canonicalInstant(input.OccurredAt), input.Note, pairs}}
	return executeFinancialWrite(ctx, s.repo, owner, requestKey, cmd, func(ctx context.Context, tx pgx.Tx) (Event, error) {
		return s.repo.postInTx(ctx, tx, owner, postSpec{
			requestKey: requestKey, kind: kind, amount: input.AmountMinor, currency: input.Currency,
			occurredAt: input.OccurredAt.UTC().Truncate(time.Microsecond), note: input.Note,
			movements:   []movementSpec{{accountID: input.AccountID, amount: amount}},
			allocations: input.Allocations,
		})
	})
}

func (s *Service) Transfer(ctx context.Context, owner identity.OwnerID, input TransferInput, requestKey string) (Event, error) {
	if !owner.Valid() {
		return Event{}, ErrUnauthorized
	}
	input.FromAccountID = strings.ToLower(input.FromAccountID)
	input.ToAccountID = strings.ToLower(input.ToAccountID)
	if !validID(input.FromAccountID) || !validID(input.ToAccountID) || input.FromAccountID == input.ToAccountID ||
		!validEventInput(input.AmountMinor, input.Currency, input.OccurredAt, input.Note) {
		return Event{}, ErrInvalidInput
	}
	cmd := financialCommand{operation: "transfer.create", parameters: []any{input.FromAccountID, input.ToAccountID, strconv.FormatInt(input.AmountMinor, 10), input.Currency, canonicalInstant(input.OccurredAt), input.Note}}
	return executeFinancialWrite(ctx, s.repo, owner, requestKey, cmd, func(ctx context.Context, tx pgx.Tx) (Event, error) {
		return s.repo.postInTx(ctx, tx, owner, postSpec{
			requestKey: requestKey, kind: "transfer", amount: input.AmountMinor, currency: input.Currency,
			occurredAt: input.OccurredAt.UTC().Truncate(time.Microsecond), note: input.Note,
			movements: []movementSpec{{accountID: input.FromAccountID, amount: -input.AmountMinor}, {accountID: input.ToAccountID, amount: input.AmountMinor}},
		})
	})
}

func (s *Service) Balance(ctx context.Context, owner identity.OwnerID, accountID string) (Balance, error) {
	if !owner.Valid() {
		return Balance{}, ErrUnauthorized
	}
	if !validID(accountID) {
		return Balance{}, ErrInvalidInput
	}
	return s.repo.balance(ctx, owner, accountID)
}

func (s *Service) ListAccounts(ctx context.Context, owner identity.OwnerID) ([]Account, error) {
	if !owner.Valid() {
		return nil, ErrUnauthorized
	}
	return s.repo.listAccounts(ctx, owner)
}

func (s *Service) ListCategories(ctx context.Context, owner identity.OwnerID) ([]Category, error) {
	if !owner.Valid() {
		return nil, ErrUnauthorized
	}
	return s.repo.listCategories(ctx, owner)
}

func (s *Service) History(ctx context.Context, owner identity.OwnerID, limit, offset int) ([]Event, error) {
	if !owner.Valid() {
		return nil, ErrUnauthorized
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, ErrInvalidInput
	}
	return s.repo.history(ctx, owner, limit, offset)
}

// CategorySpending requires a currency so that unlike units are never summed.
func (s *Service) CategorySpending(ctx context.Context, owner identity.OwnerID, categoryID, currencyCode string) (int64, error) {
	if !owner.Valid() {
		return 0, ErrUnauthorized
	}
	if !validID(categoryID) || !validCurrency(currencyCode) {
		return 0, ErrInvalidInput
	}
	return s.repo.categorySpending(ctx, owner, categoryID, currencyCode)
}

func (s *Service) ArchiveAccount(ctx context.Context, owner identity.OwnerID, accountID string) error {
	if !owner.Valid() {
		return ErrUnauthorized
	}
	if !validID(accountID) {
		return ErrInvalidInput
	}
	return s.repo.archiveAccount(ctx, owner, accountID)
}

// RestoreAccount makes an archived account usable again without changing its history.
func (s *Service) RestoreAccount(ctx context.Context, owner identity.OwnerID, accountID string) error {
	if !owner.Valid() {
		return ErrUnauthorized
	}
	if !validID(accountID) {
		return ErrInvalidInput
	}
	return s.repo.restoreAccount(ctx, owner, accountID)
}

const noVoidReasonProvided = "No reason provided by owner"

// VoidEvent keeps the original event, movements, and allocations for audit.
// Posted corrections use EditEvent; Void remains a separate visible audit action.
// An omitted expected revision is allowed only to replay an already committed v9 key.
func (s *Service) VoidEvent(ctx context.Context, owner identity.OwnerID, eventID, reason, requestKey string, expected ...int64) error {
	if !owner.Valid() {
		return ErrUnauthorized
	}
	eventID = strings.ToLower(eventID)
	reason = strings.TrimSpace(reason)
	if !validID(eventID) || !utf8.ValidString(reason) || utf8.RuneCountInString(reason) > 500 {
		return ErrInvalidInput
	}
	var explicit any = reason
	mode := "explicit"
	if reason == "" {
		mode = "generated"
		explicit = nil
		reason = noVoidReasonProvided
	}
	revision := requestedRevision(expected)
	cmd := financialCommand{operation: "event.void", target: eventID, parameters: []any{mode, explicit}, revisionRequired: revision == 0}
	if revision > 0 {
		cmd.version = 2
		cmd.parameters = []any{strconv.FormatInt(revision, 10), mode, explicit}
	}
	_, err := executeFinancialWrite(ctx, s.repo, owner, requestKey, cmd, func(ctx context.Context, tx pgx.Tx) (struct{}, error) {
		return struct{}{}, s.repo.voidEventInTx(ctx, tx, owner, eventID, reason, revision, requestKey)
	})
	return err
}

// DeleteEvent hides an owned event from ordinary reads while retaining its ledger rows.
// Repeating a successful deletion is safe and does not change balances again.
// An omitted expected revision is allowed only to replay an already committed v9 key.
func (s *Service) DeleteEvent(ctx context.Context, owner identity.OwnerID, eventID, requestKey string, expected ...int64) error {
	if !owner.Valid() {
		return ErrUnauthorized
	}
	if !validID(eventID) {
		return ErrInvalidInput
	}
	eventID = strings.ToLower(eventID)
	revision := requestedRevision(expected)
	cmd := financialCommand{operation: "event.delete", target: eventID, parameters: []any{}, revisionRequired: revision == 0}
	if revision > 0 {
		cmd.version = 2
		cmd.parameters = []any{strconv.FormatInt(revision, 10)}
	}
	_, err := executeFinancialWrite(ctx, s.repo, owner, requestKey, cmd, func(ctx context.Context, tx pgx.Tx) (struct{}, error) {
		return struct{}{}, s.repo.deleteEventInTx(ctx, tx, owner, eventID, revision, requestKey)
	})
	return err
}

func validAccountType(value string) bool {
	switch value {
	case "cash", "bank", "ewallet", "savings", "credit_card", "other":
		return true
	default:
		return false
	}
}

func validCurrency(code string) bool {
	if len(code) != 3 {
		return false
	}
	for _, letter := range code {
		if letter < 'A' || letter > 'Z' {
			return false
		}
	}
	_, err := currency.ParseISO(code)
	return err == nil
}

func validEventInput(amount int64, currencyCode string, occurredAt time.Time, note string) bool {
	return amount > 0 && validCurrency(currencyCode) && !occurredAt.IsZero() && utf8.ValidString(note) && len(note) <= 1000
}

func validID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, ch := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if ch != '-' {
				return false
			}
		} else if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return false
		}
	}
	return true
}

func newUUID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate finance ID: %w", err)
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:]), nil
}
