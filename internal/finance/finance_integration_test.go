package finance

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/Faust217/LifeOS/internal/platform/config"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

func financeTestDatabase(t *testing.T, environmentKey string) (*pgxpool.Pool, *migrate.Migrate) {
	t.Helper()
	dsn := os.Getenv(environmentKey)
	if dsn == "" {
		t.Skipf("set %s to an empty disposable *_test PostgreSQL database", environmentKey)
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(strings.TrimPrefix(parsed.Path, "/"), "_test") {
		t.Fatal("finance test DSN must name a disposable *_test database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var existing bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&existing); err != nil || existing {
		t.Fatalf("finance test database must be empty: existing=%t err=%v", existing, err)
	}
	path, err := filepath.Abs("../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	runner, err := migrate.New((&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = runner.Close() })
	if err := runner.Steps(3); err != nil {
		t.Fatalf("migrate empty database to populated version 3: %v", err)
	}
	return pool, runner
}

func verifiedTestOwner(t *testing.T, pool *pgxpool.Pool, userID, secret string) identity.OwnerID {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO lifeos_users (id, email) VALUES ($1, $2)`, userID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(secret))
	if _, err := pool.Exec(ctx, `INSERT INTO lifeos_sessions (token_hash, user_id, expires_at)
		VALUES ($1, $2, now() + interval '1 hour')`, hash[:], userID); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": "http://" + r.Host, "authorization_endpoint": "http://" + r.Host + "/authorize",
			"token_endpoint": "http://" + r.Host + "/token", "jwks_uri": "http://" + r.Host + "/keys",
			"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	defer provider.Close()
	auth, err := identity.NewAuth(ctx, pool, config.AuthConfig{
		ClientID: "finance-test", ClientSecret: "test-only", Issuer: provider.URL,
		PublicOrigin: "http://127.0.0.1:4173",
	})
	if err != nil {
		t.Fatal(err)
	}
	var owner identity.OwnerID
	request := httptest.NewRequest(http.MethodGet, "/finance-test", nil)
	request.AddCookie(&http.Cookie{Name: "lifeos_session", Value: secret})
	response := httptest.NewRecorder()
	auth.RequireRead(func(w http.ResponseWriter, _ *http.Request, verified identity.OwnerID) {
		owner = verified
		w.WriteHeader(http.StatusNoContent)
	}).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !owner.Valid() || owner.String() != userID {
		t.Fatalf("verified owner status=%d id=%q", response.Code, owner.String())
	}
	return owner
}

func TestFinanceMigrationAndLedger(t *testing.T) {
	pool, runner := financeTestDatabase(t, "LIFEOS_FINANCE_TEST_DATABASE_URL")
	ctx := context.Background()
	owner := verifiedTestOwner(t, pool, "11111111-1111-4111-8111-111111111111", "owner-session-test")
	other := verifiedTestOwner(t, pool, "22222222-2222-4222-8222-222222222222", "other-session-test")
	upgradeOwner := verifiedTestOwner(t, pool, "33333333-3333-4333-8333-333333333333", "upgrade-session-test")
	if err := runner.Steps(1); err != nil {
		t.Fatalf("upgrade populated version 3 to Finance version 4: %v", err)
	}
	if version, dirty, err := runner.Version(); err != nil || version != 4 || dirty {
		t.Fatalf("migration version=%d dirty=%t err=%v", version, dirty, err)
	}
	var retained int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM lifeos_users WHERE id IN ($1, $2)`, owner.String(), other.String()).Scan(&retained); err != nil || retained != 2 {
		t.Fatalf("upgrade lost identity users: count=%d err=%v", retained, err)
	}
	// A pre-existing Finance event and movement must survive the v5/v6 upgrade.
	const upgradeAccount = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const upgradeEvent = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	if _, err := pool.Exec(ctx, `INSERT INTO finance_accounts (id, owner_id, name, account_type, currency, opening_balance_minor)
		VALUES ($1, $2, 'Upgrade cash', 'cash', 'MYR', 1000)`, upgradeAccount, upgradeOwner.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO financial_events (id, owner_id, kind, amount_minor, currency, occurred_at)
		VALUES ($1, $2, 'expense', 100, 'MYR', now())`, upgradeEvent, upgradeOwner.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO account_movements (id, owner_id, event_id, account_id, amount_minor, currency)
		VALUES ('cccccccc-cccc-4ccc-8ccc-cccccccccccc', $1, $2, $3, -100, 'MYR')`, upgradeOwner.String(), upgradeEvent, upgradeAccount); err != nil {
		t.Fatal(err)
	}
	if err := runner.Steps(2); err != nil {
		t.Fatalf("upgrade populated version 4 through Goals to Soft Delete version 6: %v", err)
	}
	if version, dirty, err := runner.Version(); err != nil || version != 6 || dirty {
		t.Fatalf("soft delete migration version=%d dirty=%t err=%v", version, dirty, err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	service := NewService(pool)
	if got, err := service.Balance(ctx, upgradeOwner, upgradeAccount); err != nil || got.AmountMinor != 900 {
		t.Fatalf("upgrade event balance=%+v err=%v", got, err)
	}
	var upgradeDeleted *time.Time
	if err := pool.QueryRow(ctx, `SELECT deleted_at FROM financial_events WHERE owner_id = $1 AND id = $2`, upgradeOwner.String(), upgradeEvent).Scan(&upgradeDeleted); err != nil || upgradeDeleted != nil {
		t.Fatalf("upgraded event deleted_at=%v err=%v", upgradeDeleted, err)
	}
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	account := func(o identity.OwnerID, name, kind, currencyCode string, opening int64) Account {
		t.Helper()
		result, err := legacyTestAccount(ctx, service, o, AccountInput{Name: name, Type: kind, Currency: currencyCode, OpeningBalanceMinor: opening})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	category := func(o identity.OwnerID, name, kind string) Category {
		t.Helper()
		result, err := service.CreateCategory(ctx, o, CategoryInput{Name: name, Kind: kind})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	assertBalance := func(o identity.OwnerID, id string, want int64) {
		t.Helper()
		got, err := service.Balance(ctx, o, id)
		if err != nil || got.AmountMinor != want {
			t.Fatalf("balance id=%s got=%+v want=%d err=%v", id, got, want, err)
		}
	}
	bank := account(owner, "Bank", "bank", "MYR", 10000)
	savings := account(owner, "Savings", "savings", "MYR", 0)
	card := account(owner, "Card", "credit_card", "MYR", -5000)
	usd := account(owner, "USD", "bank", "USD", 1000)
	food := category(owner, "Food", "expense")
	groceries := category(owner, "Groceries", "expense")
	salary := category(owner, "Salary", "income")
	otherBank := account(other, "Private", "bank", "MYR", 4000)
	otherFood := category(other, "Private food", "expense")
	assertBalance(owner, bank.ID, 10000)
	assertBalance(owner, card.ID, -5000)

	expense, err := service.PostExpense(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 1500, Currency: "MYR",
		OccurredAt: now, Note: "Lunch", Allocations: []AllocationInput{{food.ID, 1000}, {groceries.ID, 500}}}, newFinancialWriteKey())
	if err != nil || len(expense.Movements) != 1 || expense.Movements[0].AmountMinor != -1500 || len(expense.Allocations) != 2 {
		t.Fatalf("expense=%+v err=%v", expense, err)
	}
	assertBalance(owner, bank.ID, 8500)
	income, err := service.PostIncome(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 3000, Currency: "MYR",
		OccurredAt: now.Add(time.Minute), Allocations: []AllocationInput{{salary.ID, 3000}}}, newFinancialWriteKey())
	if err != nil || len(income.Movements) != 1 || income.Movements[0].AmountMinor != 3000 {
		t.Fatalf("income=%+v err=%v", income, err)
	}
	assertBalance(owner, bank.ID, 11500)
	transfer, err := service.Transfer(ctx, owner, TransferInput{FromAccountID: bank.ID, ToAccountID: savings.ID,
		AmountMinor: 5000, Currency: "MYR", OccurredAt: now.Add(2 * time.Minute)}, newFinancialWriteKey())
	if err != nil || len(transfer.Movements) != 2 || len(transfer.Allocations) != 0 ||
		transfer.Movements[0].AmountMinor+transfer.Movements[1].AmountMinor != 0 {
		t.Fatalf("transfer=%+v err=%v", transfer, err)
	}
	assertBalance(owner, bank.ID, 6500)
	assertBalance(owner, savings.ID, 5000)
	cardPurchase, err := service.PostExpense(ctx, owner, EventInput{AccountID: card.ID, AmountMinor: 1000, Currency: "MYR",
		OccurredAt: now.Add(3 * time.Minute), Allocations: []AllocationInput{{food.ID, 1000}}}, newFinancialWriteKey())
	if err != nil || cardPurchase.Movements[0].AmountMinor != -1000 {
		t.Fatalf("card purchase=%+v err=%v", cardPurchase, err)
	}
	assertBalance(owner, card.ID, -6000)
	cardPayment, err := service.Transfer(ctx, owner, TransferInput{FromAccountID: bank.ID, ToAccountID: card.ID,
		AmountMinor: 2000, Currency: "MYR", OccurredAt: now.Add(4 * time.Minute)}, newFinancialWriteKey())
	if err != nil || len(cardPayment.Allocations) != 0 {
		t.Fatalf("card payment=%+v err=%v", cardPayment, err)
	}
	assertBalance(owner, bank.ID, 4500)
	assertBalance(owner, card.ID, -4000)
	_, err = service.PostExpense(ctx, owner, EventInput{AccountID: usd.ID, AmountMinor: 100, Currency: "USD",
		OccurredAt: now.Add(-time.Minute), Allocations: []AllocationInput{{food.ID, 100}}}, newFinancialWriteKey())
	if err != nil {
		t.Fatal(err)
	}
	assertBalance(owner, usd.ID, 900)
	spending, err := service.CategorySpending(ctx, owner, food.ID, "MYR")
	if err != nil || spending != 2000 {
		t.Fatalf("food spending=%d err=%v", spending, err)
	}
	if got, err := service.CategorySpending(ctx, owner, food.ID, "USD"); err != nil || got != 100 {
		t.Fatalf("cross-currency spending=%d err=%v", got, err)
	}
	history, err := service.History(ctx, owner, 20, 0)
	if err != nil || len(history) != 6 || history[0].ID != cardPayment.ID || len(history[0].Movements) != 2 {
		t.Fatalf("history count=%d err=%v", len(history), err)
	}
	if accounts, err := service.ListAccounts(ctx, other); err != nil || len(accounts) != 1 || accounts[0].ID != otherBank.ID {
		t.Fatalf("other accounts=%+v err=%v", accounts, err)
	}
	if categories, err := service.ListCategories(ctx, other); err != nil || len(categories) != 1 || categories[0].ID != otherFood.ID {
		t.Fatalf("other categories=%+v err=%v", categories, err)
	}
	if events, err := service.History(ctx, other, 20, 0); err != nil || len(events) != 0 {
		t.Fatalf("other history=%+v err=%v", events, err)
	}

	if _, err := service.Balance(ctx, owner, otherBank.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner balance=%v", err)
	}
	if err := service.ArchiveAccount(ctx, owner, otherBank.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner archive=%v", err)
	}
	if err := service.RestoreAccount(ctx, owner, otherBank.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner restore=%v", err)
	}
	if _, err := service.PostExpense(ctx, owner, EventInput{AccountID: otherBank.ID, AmountMinor: 100, Currency: "MYR",
		OccurredAt: now, Allocations: []AllocationInput{{food.ID, 100}}}, newFinancialWriteKey()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign account write=%v", err)
	}
	if _, err := service.PostExpense(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 100, Currency: "MYR",
		OccurredAt: now, Allocations: []AllocationInput{{otherFood.ID, 100}}}, newFinancialWriteKey()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign category write=%v", err)
	}
	if _, err := service.Transfer(ctx, owner, TransferInput{FromAccountID: bank.ID, ToAccountID: otherBank.ID,
		AmountMinor: 100, Currency: "MYR", OccurredAt: now}, newFinancialWriteKey()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign transfer target=%v", err)
	}
	if err := service.VoidEvent(ctx, other, expense.ID, "not mine", newFinancialWriteKey(), testEventRevision(t, service, other, expense.ID)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner void=%v", err)
	}
	if _, err := service.CategorySpending(ctx, owner, otherFood.ID, "MYR"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner category report=%v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO account_movements
		(id, owner_id, event_id, account_id, amount_minor, currency)
		VALUES ('33333333-3333-4333-8333-333333333333', $1, $2, $3, -1, 'MYR')`, owner.String(), expense.ID, otherBank.ID); err == nil {
		t.Fatal("database accepted foreign-owned account movement")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO category_allocations
		(id, owner_id, event_id, category_id, kind, amount_minor, currency)
		VALUES ('44444444-4444-4444-8444-444444444444', $1, $2, $3, 'expense', 1, 'MYR')`, owner.String(), expense.ID, otherFood.ID); err == nil {
		t.Fatal("database accepted foreign-owned category allocation")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO account_movements
		(id, owner_id, event_id, account_id, amount_minor, currency)
		VALUES ('55555555-5555-4555-8555-555555555555', $1, $2, $3, -1, 'USD')`, owner.String(), expense.ID, bank.ID); err == nil {
		t.Fatal("database accepted movement with mismatched currency")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO category_allocations
		(id, owner_id, event_id, category_id, kind, amount_minor, currency)
		VALUES ('66666666-6666-4666-8666-666666666666', $1, $2, $3, 'income', 1, 'MYR')`, owner.String(), expense.ID, salary.ID); err == nil {
		t.Fatal("database accepted category kind inconsistent with event")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO category_allocations
		(id, owner_id, event_id, category_id, kind, amount_minor, currency)
		VALUES ('77777777-7777-4777-8777-777777777777', $1, $2, $3, 'expense', 1, 'MYR')`, owner.String(), transfer.ID, food.ID); err == nil {
		t.Fatal("database accepted transfer as spending")
	}

	if _, err := service.PostExpense(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 0, Currency: "MYR", OccurredAt: now,
		Allocations: []AllocationInput{{food.ID, 1}}}, newFinancialWriteKey()); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero expense=%v", err)
	}
	if _, err := service.PostIncome(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 100, Currency: "ZZZ", OccurredAt: now,
		Allocations: []AllocationInput{{salary.ID, 100}}}, newFinancialWriteKey()); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid ISO currency=%v", err)
	}
	if _, err := service.PostExpense(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: now,
		Allocations: []AllocationInput{{food.ID, 99}}}, newFinancialWriteKey()); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("allocation mismatch=%v", err)
	}
	if _, err := service.PostIncome(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: now,
		Allocations: []AllocationInput{{food.ID, 100}}}, newFinancialWriteKey()); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("income with expense category=%v", err)
	}
	if _, err := service.PostExpense(ctx, owner, EventInput{AccountID: usd.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: now,
		Allocations: []AllocationInput{{food.ID, 100}}}, newFinancialWriteKey()); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("account currency mismatch=%v", err)
	}
	if _, err := service.Transfer(ctx, owner, TransferInput{FromAccountID: bank.ID, ToAccountID: usd.ID,
		AmountMinor: 100, Currency: "MYR", OccurredAt: now}, newFinancialWriteKey()); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("transfer currency mismatch=%v", err)
	}
	overdraft := account(owner, "Overdraft", "bank", "MYR", 100)
	overdraftTarget := account(owner, "Overdraft target", "savings", "MYR", 0)
	overdraftCategory := category(owner, "Overdraft expense", "expense")
	overdraftExpense, err := service.PostExpense(ctx, owner, EventInput{AccountID: overdraft.ID, AmountMinor: 150, Currency: "MYR", OccurredAt: now,
		Allocations: []AllocationInput{{overdraftCategory.ID, 150}}}, newFinancialWriteKey())
	if err != nil || len(overdraftExpense.Movements) != 1 || overdraftExpense.Movements[0].AmountMinor != -150 {
		t.Fatalf("ordinary account overdraft expense=%+v err=%v", overdraftExpense, err)
	}
	assertBalance(owner, overdraft.ID, -50)
	overdraftTransfer, err := service.Transfer(ctx, owner, TransferInput{FromAccountID: overdraft.ID,
		ToAccountID: overdraftTarget.ID, AmountMinor: 75, Currency: "MYR", OccurredAt: now}, newFinancialWriteKey())
	if err != nil || len(overdraftTransfer.Allocations) != 0 {
		t.Fatalf("ordinary account overdraft transfer=%+v err=%v", overdraftTransfer, err)
	}
	assertBalance(owner, overdraft.ID, -125)
	assertBalance(owner, overdraftTarget.ID, 75)
	if err := service.VoidEvent(ctx, owner, overdraftTransfer.ID, "Wrong transfer", newFinancialWriteKey(), testEventRevision(t, service, owner, overdraftTransfer.ID)); err != nil {
		t.Fatalf("void overdraft transfer: %v", err)
	}
	assertBalance(owner, overdraft.ID, -50)
	assertBalance(owner, overdraftTarget.ID, 0)
	if spent, err := service.CategorySpending(ctx, owner, overdraftCategory.ID, "MYR"); err != nil || spent != 150 {
		t.Fatalf("overdraft expense spending=%d err=%v", spent, err)
	}
	maxAccount := account(owner, "Maximum", "bank", "MYR", math.MaxInt64)
	if _, err := service.PostIncome(ctx, owner, EventInput{AccountID: maxAccount.ID, AmountMinor: 1, Currency: "MYR", OccurredAt: now,
		Allocations: []AllocationInput{{salary.ID, 1}}}, newFinancialWriteKey()); !errors.Is(err, ErrOverflow) {
		t.Fatalf("income overflow=%v", err)
	}
	minCard := account(owner, "Minimum liability", "credit_card", "MYR", math.MinInt64)
	if _, err := service.PostExpense(ctx, owner, EventInput{AccountID: minCard.ID, AmountMinor: 1, Currency: "MYR", OccurredAt: now,
		Allocations: []AllocationInput{{food.ID, 1}}}, newFinancialWriteKey()); !errors.Is(err, ErrOverflow) {
		t.Fatalf("liability overflow=%v", err)
	}
	before := financeCounts(t, pool, owner)
	rollbackTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.repo.postInTx(ctx, rollbackTx, owner, postSpec{kind: "expense", amount: 100, currency: "MYR", occurredAt: now,
		movements:   []movementSpec{{accountID: bank.ID, amount: -100}},
		allocations: []AllocationInput{{food.ID, 50}, {food.ID, 50}}})
	_ = rollbackTx.Rollback(ctx)
	if err == nil {
		t.Fatal("duplicate allocation did not abort event")
	}
	if after := financeCounts(t, pool, owner); after != before {
		t.Fatalf("partial financial write survived rollback: before=%v after=%v", before, after)
	}
	assertBalance(owner, bank.ID, 4500)

	if err := service.ArchiveAccount(ctx, owner, savings.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Transfer(ctx, owner, TransferInput{FromAccountID: bank.ID, ToAccountID: savings.ID,
		AmountMinor: 100, Currency: "MYR", OccurredAt: now}, newFinancialWriteKey()); !errors.Is(err, ErrArchived) {
		t.Fatalf("transfer to archived account=%v", err)
	}
	assertBalance(owner, savings.ID, 5000)
	if err := service.RestoreAccount(ctx, owner, savings.ID); err != nil {
		t.Fatalf("restore archived account: %v", err)
	}
	if err := service.RestoreAccount(ctx, owner, savings.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("restore active account=%v", err)
	}
	assertBalance(owner, savings.ID, 5000)
	if err := service.VoidEvent(ctx, owner, expense.ID, "", newFinancialWriteKey(), testEventRevision(t, service, owner, expense.ID)); err != nil {
		t.Fatal(err)
	}
	assertBalance(owner, bank.ID, 6000)
	if got, err := service.CategorySpending(ctx, owner, food.ID, "MYR"); err != nil || got != 1000 {
		t.Fatalf("spending after void=%d err=%v", got, err)
	}
	history, err = service.History(ctx, owner, 20, 0)
	if err != nil || len(history) != 8 {
		t.Fatalf("history after void count=%d err=%v", len(history), err)
	}
	var foundVoid bool
	for _, event := range history {
		if event.ID == expense.ID {
			foundVoid = event.Status == "voided" && event.VoidedAt != nil && event.VoidReason == noVoidReasonProvided && len(event.Movements) == 1 && len(event.Allocations) == 2
		}
	}
	if !foundVoid {
		t.Fatal("void lost financial history")
	}
	if err := service.VoidEvent(ctx, owner, expense.ID, "again", newFinancialWriteKey(), testEventRevision(t, service, owner, expense.ID)); !errors.Is(err, ErrNotEditable) {
		t.Fatalf("repeat void=%v", err)
	}

	concurrentAccount := account(owner, "Concurrency", "bank", "MYR", 100)
	spendingBeforeConcurrent, err := service.CategorySpending(ctx, owner, food.ID, "MYR")
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := service.PostExpense(ctx, owner, EventInput{AccountID: concurrentAccount.ID, AmountMinor: 80,
				Currency: "MYR", OccurredAt: now, Allocations: []AllocationInput{{food.ID, 80}}}, newFinancialWriteKey())
			results <- err
		}()
	}
	group.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		default:
			t.Fatalf("concurrent expense error=%v", err)
		}
	}
	if succeeded != 2 {
		t.Fatalf("concurrent expenses success=%d", succeeded)
	}
	assertBalance(owner, concurrentAccount.ID, -60)
	if spent, err := service.CategorySpending(ctx, owner, food.ID, "MYR"); err != nil || spent != spendingBeforeConcurrent+160 {
		t.Fatalf("concurrent expense spending=%d want=%d err=%v", spent, spendingBeforeConcurrent+160, err)
	}
	transferSource := account(owner, "Concurrent source", "bank", "MYR", 100)
	transferTarget := account(owner, "Concurrent target", "bank", "MYR", 0)
	transferResults := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := service.Transfer(ctx, owner, TransferInput{FromAccountID: transferSource.ID,
				ToAccountID: transferTarget.ID, AmountMinor: 80, Currency: "MYR", OccurredAt: now}, newFinancialWriteKey())
			transferResults <- err
		}()
	}
	group.Wait()
	close(transferResults)
	succeeded = 0
	for err := range transferResults {
		switch {
		case err == nil:
			succeeded++
		default:
			t.Fatalf("concurrent transfer error=%v", err)
		}
	}
	if succeeded != 2 {
		t.Fatalf("concurrent transfers success=%d", succeeded)
	}
	assertBalance(owner, transferSource.ID, -60)
	assertBalance(owner, transferTarget.ID, 160)
	if spent, err := service.CategorySpending(ctx, owner, food.ID, "MYR"); err != nil || spent != spendingBeforeConcurrent+160 {
		t.Fatalf("transfer duplicated spending=%d want=%d err=%v", spent, spendingBeforeConcurrent+160, err)
	}
	oppositeA := account(owner, "Opposite A", "bank", "MYR", 100)
	oppositeB := account(owner, "Opposite B", "bank", "MYR", 100)
	oppositeResults := make(chan error, 2)
	for _, pair := range [][2]string{{oppositeA.ID, oppositeB.ID}, {oppositeB.ID, oppositeA.ID}} {
		group.Add(1)
		go func(fromID, toID string) {
			defer group.Done()
			_, err := service.Transfer(ctx, owner, TransferInput{FromAccountID: fromID,
				ToAccountID: toID, AmountMinor: 60, Currency: "MYR", OccurredAt: now}, newFinancialWriteKey())
			oppositeResults <- err
		}(pair[0], pair[1])
	}
	group.Wait()
	close(oppositeResults)
	for err := range oppositeResults {
		if err != nil {
			t.Fatalf("opposite-direction transfer failed: %v", err)
		}
	}
	assertBalance(owner, oppositeA.ID, 100)
	assertBalance(owner, oppositeB.ID, 100)
	voidAccount := account(owner, "Void income", "bank", "MYR", 0)
	voidIncome, err := service.PostIncome(ctx, owner, EventInput{AccountID: voidAccount.ID, AmountMinor: 100,
		Currency: "MYR", OccurredAt: now, Allocations: []AllocationInput{{salary.ID, 100}}}, newFinancialWriteKey())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PostExpense(ctx, owner, EventInput{AccountID: voidAccount.ID, AmountMinor: 100,
		Currency: "MYR", OccurredAt: now, Allocations: []AllocationInput{{food.ID, 100}}}, newFinancialWriteKey()); err != nil {
		t.Fatal(err)
	}
	if err := service.VoidEvent(ctx, owner, voidIncome.ID, "Income did not occur", newFinancialWriteKey(), testEventRevision(t, service, owner, voidIncome.ID)); err != nil {
		t.Fatalf("void income into negative balance=%v", err)
	}
	assertBalance(owner, voidAccount.ID, -100)
	if err := service.VoidEvent(ctx, owner, income.ID, "correct salary", newFinancialWriteKey(), testEventRevision(t, service, owner, income.ID)); err != nil {
		t.Fatal(err)
	}
	assertBalance(owner, bank.ID, 3000)
	if _, err := service.CategorySpending(ctx, owner, salary.ID, "MYR"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("income category counted as spending=%v", err)
	}
	if _, err := service.Transfer(ctx, owner, TransferInput{FromAccountID: bank.ID, ToAccountID: savings.ID,
		AmountMinor: 100, Currency: "MYR", OccurredAt: now}, newFinancialWriteKey()); err != nil {
		t.Fatalf("transfer to restored account: %v", err)
	}
	assertBalance(owner, bank.ID, 2900)
	assertBalance(owner, savings.ID, 5100)

	// Category names are owner-scoped labels; changing them never rewrites allocations.
	renamed, err := service.RenameCategory(ctx, owner, food.ID, "Medical")
	if err != nil || renamed.ID != food.ID || renamed.Kind != "expense" || renamed.Name != "Medical" {
		t.Fatalf("rename category=%+v err=%v", renamed, err)
	}
	if _, err := service.RenameCategory(ctx, owner, food.ID, "Groceries"); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting rename=%v", err)
	}
	if _, err := service.CreateCategory(ctx, owner, CategoryInput{Name: "medical", Kind: "expense"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate category creation=%v", err)
	}
	if _, err := service.RenameCategory(ctx, owner, food.ID, "Uncategorized"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("reserved category name=%v", err)
	}
	if _, err := service.RenameCategory(ctx, other, food.ID, "Private"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner rename=%v", err)
	}
	if err := service.ArchiveCategory(ctx, other, food.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner category archive=%v", err)
	}
	spendingBeforeArchive, err := service.CategorySpending(ctx, owner, food.ID, "MYR")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveCategory(ctx, owner, food.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PostExpense(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: now,
		Allocations: []AllocationInput{{food.ID, 100}}}, newFinancialWriteKey()); !errors.Is(err, ErrArchived) {
		t.Fatalf("archived category accepted=%v", err)
	}
	if got, err := service.CategorySpending(ctx, owner, food.ID, "MYR"); err != nil || got != spendingBeforeArchive {
		t.Fatalf("archived category changed spending=%d want=%d err=%v", got, spendingBeforeArchive, err)
	}
	if err := service.RestoreCategory(ctx, other, food.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner category restore=%v", err)
	}
	if err := service.RestoreCategory(ctx, owner, food.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PostExpense(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: now,
		Allocations: []AllocationInput{{food.ID, 100}}}, newFinancialWriteKey()); err != nil {
		t.Fatalf("restored category rejected=%v", err)
	}
	assertBalance(owner, bank.ID, 2800)
	uncategorizedExpense, err := service.PostExpense(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 200, Currency: "MYR", OccurredAt: now.Add(time.Hour), Allocations: []AllocationInput{}}, newFinancialWriteKey())
	if err != nil || len(uncategorizedExpense.Movements) != 1 || len(uncategorizedExpense.Allocations) != 0 {
		t.Fatalf("uncategorized expense=%+v err=%v", uncategorizedExpense, err)
	}
	uncategorizedIncome, err := service.PostIncome(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 150, Currency: "MYR", OccurredAt: now.Add(2 * time.Hour), Allocations: []AllocationInput{}}, newFinancialWriteKey())
	if err != nil || len(uncategorizedIncome.Movements) != 1 || len(uncategorizedIncome.Allocations) != 0 {
		t.Fatalf("uncategorized income=%+v err=%v", uncategorizedIncome, err)
	}
	assertBalance(owner, bank.ID, 2750)
	var persistedAllocations int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM category_allocations WHERE owner_id = $1 AND event_id IN ($2, $3)`,
		owner.String(), uncategorizedExpense.ID, uncategorizedIncome.ID).Scan(&persistedAllocations); err != nil || persistedAllocations != 0 {
		t.Fatalf("uncategorized allocations=%d err=%v", persistedAllocations, err)
	}
	if err := service.VoidEvent(ctx, owner, uncategorizedExpense.ID, "Correction", newFinancialWriteKey(), testEventRevision(t, service, owner, uncategorizedExpense.ID)); err != nil {
		t.Fatalf("void uncategorized expense=%v", err)
	}
	assertBalance(owner, bank.ID, 2950)
	if _, err := service.PostExpense(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: now,
		Allocations: []AllocationInput{{food.ID, 70}, {groceries.ID, 20}}}, newFinancialWriteKey()); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("partial multi-allocation accepted=%v", err)
	}

	// Retain the published v4 guard check without bypassing current v9 metadata.
	publishedDown, err := os.ReadFile("../../db/migrations/000004_money_ledger.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	downTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = downTx.Exec(ctx, string(publishedDown))
	_ = downTx.Rollback(ctx)
	if err == nil {
		t.Fatal("populated v4 down discarded ledger history")
	}
	// A populated v9 down refuses to discard retry protection or the ledger.
	if err := runner.Steps(-1); err == nil {
		t.Fatal("idempotency down migration removed committed keys")
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM financial_events WHERE owner_id = $1`, owner.String()).Scan(&remaining); err != nil || remaining < 8 {
		t.Fatalf("down migration lost events: count=%d err=%v", remaining, err)
	}
}

func financeCounts(t *testing.T, pool *pgxpool.Pool, owner identity.OwnerID) [3]int {
	t.Helper()
	var result [3]int
	for i, table := range []string{"financial_events", "account_movements", "category_allocations"} {
		// Table names are a fixed test-only literal list, never user input.
		if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table+" WHERE owner_id = $1", owner.String()).Scan(&result[i]); err != nil {
			t.Fatal(err)
		}
	}
	return result
}
