package finance

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
)

func TestFinanceSoftDelete(t *testing.T) {
	pool, runner := financeTestDatabase(t, "LIFEOS_SOFT_DELETE_TEST_DATABASE_URL")
	ctx := context.Background()
	owner := verifiedTestOwner(t, pool, "11111111-1111-4111-8111-111111111111", "soft-delete-owner-test")
	other := verifiedTestOwner(t, pool, "22222222-2222-4222-8222-222222222222", "soft-delete-other-test")
	if err := runner.Up(); err != nil {
		t.Fatalf("upgrade fresh database to v6: %v", err)
	}
	service := NewService(pool)
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	bank, err := service.CreateAccount(ctx, owner, AccountInput{Name: "Bank", Type: "bank", Currency: "MYR", OpeningBalanceMinor: 10000})
	if err != nil {
		t.Fatal(err)
	}
	savings, err := service.CreateAccount(ctx, owner, AccountInput{Name: "Savings", Type: "savings", Currency: "MYR", OpeningBalanceMinor: 2000})
	if err != nil {
		t.Fatal(err)
	}
	food, err := service.CreateCategory(ctx, owner, CategoryInput{Name: "Food", Kind: "expense"})
	if err != nil {
		t.Fatal(err)
	}
	checkBalance := func(accountID string, want int64) {
		t.Helper()
		got, err := service.Balance(ctx, owner, accountID)
		if err != nil || got.AmountMinor != want {
			t.Fatalf("balance %s = %+v, err=%v, want=%d", accountID, got, err, want)
		}
	}
	checkSpending := func(want int64) {
		t.Helper()
		got, err := service.CategorySpending(ctx, owner, food.ID, "MYR")
		if err != nil || got != want {
			t.Fatalf("spending=%d err=%v want=%d", got, err, want)
		}
	}
	checkStored := func(eventID, wantStatus string, wantMovements, wantAllocations int, wantReason string) {
		t.Helper()
		var status string
		var deleted time.Time
		var voided *time.Time
		var reason *string
		if err := pool.QueryRow(ctx, `SELECT status, deleted_at, voided_at, void_reason FROM financial_events WHERE owner_id = $1 AND id = $2`, owner.String(), eventID).Scan(&status, &deleted, &voided, &reason); err != nil {
			t.Fatal(err)
		}
		if status != wantStatus || deleted.IsZero() || (reason != nil && *reason != wantReason) || (reason == nil && wantReason != "") || (status == "posted" && voided != nil) || (status == "voided" && voided == nil) {
			t.Fatalf("retained event status=%s deleted=%v voided=%v reason=%v", status, deleted, voided, reason)
		}
		var movements, allocations int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM account_movements WHERE owner_id = $1 AND event_id = $2`, owner.String(), eventID).Scan(&movements); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM category_allocations WHERE owner_id = $1 AND event_id = $2`, owner.String(), eventID).Scan(&allocations); err != nil {
			t.Fatal(err)
		}
		if movements != wantMovements || allocations != wantAllocations {
			t.Fatalf("retained links movements=%d allocations=%d", movements, allocations)
		}
	}
	postExpense := func(amount int64, at time.Time, allocations []AllocationInput) Event {
		t.Helper()
		event, err := service.PostExpense(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: amount, Currency: "MYR", OccurredAt: at, Allocations: allocations})
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	postIncome := func(amount int64, at time.Time) Event {
		t.Helper()
		event, err := service.PostIncome(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: amount, Currency: "MYR", OccurredAt: at})
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	deleteEvent := func(event Event) {
		t.Helper()
		if err := service.DeleteEvent(ctx, owner, event.ID); err != nil {
			t.Fatalf("delete %s: %v", event.ID, err)
		}
		if err := service.DeleteEvent(ctx, owner, event.ID); err != nil {
			t.Fatalf("repeat delete %s: %v", event.ID, err)
		}
		history, err := service.History(ctx, owner, 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range history {
			if item.ID == event.ID {
				t.Fatalf("deleted event %s leaked into history", event.ID)
			}
		}
	}

	expense := postExpense(1000, now, []AllocationInput{{CategoryID: food.ID, AmountMinor: 1000}})
	checkBalance(bank.ID, 9000)
	checkSpending(1000)
	if err := service.DeleteEvent(ctx, other, expense.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner delete=%v", err)
	}
	deleteEvent(expense)
	checkBalance(bank.ID, 10000)
	checkSpending(0)
	checkStored(expense.ID, "posted", 1, 1, "")
	if err := service.VoidEvent(ctx, owner, expense.ID, "late void"); !errors.Is(err, ErrConflict) {
		t.Fatalf("void deleted event=%v", err)
	}

	income := postIncome(500, now.Add(time.Minute))
	checkBalance(bank.ID, 10500)
	deleteEvent(income)
	checkBalance(bank.ID, 10000)
	checkStored(income.ID, "posted", 1, 0, "")

	transfer, err := service.Transfer(ctx, owner, TransferInput{FromAccountID: bank.ID, ToAccountID: savings.ID, AmountMinor: 300, Currency: "MYR", OccurredAt: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	checkBalance(bank.ID, 9700)
	checkBalance(savings.ID, 2300)
	checkSpending(0)
	deleteEvent(transfer)
	checkBalance(bank.ID, 10000)
	checkBalance(savings.ID, 2000)
	checkSpending(0)
	checkStored(transfer.ID, "posted", 2, 0, "")

	usd, err := service.CreateAccount(ctx, owner, AccountInput{Name: "Dollar cash", Type: "cash", Currency: "USD", OpeningBalanceMinor: 1000})
	if err != nil {
		t.Fatal(err)
	}
	usdExpense, err := service.PostExpense(ctx, owner, EventInput{AccountID: usd.ID, AmountMinor: 100, Currency: "USD", OccurredAt: now.Add(150 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	checkBalance(usd.ID, 900)
	checkBalance(bank.ID, 10000)
	deleteEvent(usdExpense)
	checkBalance(usd.ID, 1000)
	checkBalance(bank.ID, 10000)
	checkStored(usdExpense.ID, "posted", 1, 0, "")

	uncategorized := postExpense(75, now.Add(3*time.Minute), nil)
	checkBalance(bank.ID, 9925)
	deleteEvent(uncategorized)
	checkBalance(bank.ID, 10000)
	checkStored(uncategorized.ID, "posted", 1, 0, "")

	voided := postExpense(100, now.Add(4*time.Minute), []AllocationInput{{CategoryID: food.ID, AmountMinor: 100}})
	if err := service.VoidEvent(ctx, owner, voided.ID, "Owner correction"); err != nil {
		t.Fatal(err)
	}
	checkBalance(bank.ID, 10000)
	checkSpending(0)
	visible, err := service.History(ctx, owner, 100, 0)
	if err != nil || len(visible) != 1 || visible[0].Status != "voided" {
		t.Fatalf("visible void history=%+v err=%v", visible, err)
	}
	deleteEvent(voided)
	checkBalance(bank.ID, 10000)
	checkSpending(0)
	checkStored(voided.ID, "voided", 1, 1, "Owner correction")

	first := postIncome(10, now.Add(5*time.Minute))
	middle := postIncome(20, now.Add(6*time.Minute))
	last := postIncome(30, now.Add(7*time.Minute))
	deleteEvent(middle)
	firstPage, err := service.History(ctx, owner, 1, 0)
	if err != nil || len(firstPage) != 1 || firstPage[0].ID != last.ID {
		t.Fatalf("first page=%+v err=%v", firstPage, err)
	}
	secondPage, err := service.History(ctx, owner, 1, 1)
	if err != nil || len(secondPage) != 1 || secondPage[0].ID != first.ID {
		t.Fatalf("second page=%+v err=%v", secondPage, err)
	}
	deleteEvent(first)
	deleteEvent(last)
	checkBalance(bank.ID, 10000)

	concurrent := postExpense(100, now.Add(8*time.Minute), []AllocationInput{{CategoryID: food.ID, AmountMinor: 100}})
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() { defer group.Done(); <-start; results <- service.DeleteEvent(ctx, owner, concurrent.ID) }()
	}
	close(start)
	group.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent delete=%v", err)
		}
	}
	checkBalance(bank.ID, 10000)
	checkSpending(0)

	race := postIncome(20, now.Add(9*time.Minute))
	start = make(chan struct{})
	var voidErr, deleteErr error
	group.Add(2)
	go func() {
		defer group.Done()
		<-start
		voidErr = service.VoidEvent(ctx, owner, race.ID, "Race correction")
	}()
	go func() { defer group.Done(); <-start; deleteErr = service.DeleteEvent(ctx, owner, race.ID) }()
	close(start)
	group.Wait()
	if deleteErr != nil || (voidErr != nil && !errors.Is(voidErr, ErrConflict)) {
		t.Fatalf("void/delete race void=%v delete=%v", voidErr, deleteErr)
	}
	checkBalance(bank.ID, 10000)
	var raceStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM financial_events WHERE owner_id = $1 AND id = $2 AND deleted_at IS NOT NULL`, owner.String(), race.ID).Scan(&raceStatus); err != nil || (raceStatus == "voided") != (voidErr == nil) {
		t.Fatalf("race status=%s void=%v scan=%v", raceStatus, voidErr, err)
	}

	maxAccount, err := service.CreateAccount(ctx, owner, AccountInput{Name: "Maximum", Type: "bank", Currency: "MYR", OpeningBalanceMinor: math.MaxInt64})
	if err != nil {
		t.Fatal(err)
	}
	maxExpense, err := service.PostExpense(ctx, owner, EventInput{AccountID: maxAccount.ID, AmountMinor: 1, Currency: "MYR", OccurredAt: now.Add(10 * time.Minute), Allocations: []AllocationInput{{CategoryID: food.ID, AmountMinor: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	maxIncome, err := service.PostIncome(ctx, owner, EventInput{AccountID: maxAccount.ID, AmountMinor: 1, Currency: "MYR", OccurredAt: now.Add(11 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteEvent(ctx, owner, maxExpense.ID); !errors.Is(err, ErrOverflow) {
		t.Fatalf("overflow delete=%v", err)
	}
	var deletedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT deleted_at FROM financial_events WHERE id = $1`, maxExpense.ID).Scan(&deletedAt); err != nil || deletedAt != nil {
		t.Fatalf("overflow rollback marker=%v err=%v", deletedAt, err)
	}
	maxBalance, err := service.Balance(ctx, owner, maxAccount.ID)
	if err != nil || maxBalance.AmountMinor != math.MaxInt64 {
		t.Fatalf("overflow rollback balance=%+v err=%v", maxBalance, err)
	}
	checkSpending(1)
	deleteEvent(maxIncome)
	deleteEvent(maxExpense)
	checkSpending(0)

	failed := postExpense(50, now.Add(12*time.Minute), []AllocationInput{{CategoryID: food.ID, AmountMinor: 50}})
	if _, err := pool.Exec(ctx, `CREATE FUNCTION lifeos_test_reject_soft_delete() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL THEN RAISE EXCEPTION 'synthetic delete failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER lifeos_test_reject_soft_delete BEFORE UPDATE OF deleted_at ON financial_events
		FOR EACH ROW EXECUTE FUNCTION lifeos_test_reject_soft_delete()`); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteEvent(ctx, owner, failed.ID); err == nil {
		t.Fatal("injected update failure unexpectedly succeeded")
	}
	if err := pool.QueryRow(ctx, `SELECT deleted_at FROM financial_events WHERE id = $1`, failed.ID).Scan(&deletedAt); err != nil || deletedAt != nil {
		t.Fatalf("failure rollback marker=%v err=%v", deletedAt, err)
	}
	checkBalance(bank.ID, 9950)
	checkSpending(50)
	if _, err := pool.Exec(ctx, `DROP TRIGGER lifeos_test_reject_soft_delete ON financial_events`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DROP FUNCTION lifeos_test_reject_soft_delete()`); err != nil {
		t.Fatal(err)
	}
	deleteEvent(failed)
	checkBalance(bank.ID, 10000)
	checkSpending(0)

	// The v6 down migration must refuse to reveal already deleted rows.
	if err := runner.Steps(-1); err == nil {
		t.Fatal("down migration exposed deleted rows")
	}
	if err := pool.QueryRow(ctx, `SELECT deleted_at FROM financial_events WHERE id = $1`, failed.ID).Scan(&deletedAt); err != nil || deletedAt == nil {
		t.Fatalf("failed down changed marker=%v err=%v", deletedAt, err)
	}
	if err := service.DeleteEvent(ctx, identity.OwnerID{}, expense.ID); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("zero owner delete=%v", err)
	}
}
