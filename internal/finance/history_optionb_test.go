package finance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func verifyHistoryOptionB(t *testing.T, fixture *pgxpool.Pool, service *Service) {
	t.Helper()
	ctx := context.Background()
	owner := verifiedTestOwner(t, fixture, "46464646-4646-4646-8646-464646464646", "history-option-b")
	if _, err := fixture.Exec(ctx, `UPDATE lifeos_users SET time_zone='Asia/Kuala_Lumpur' WHERE id=$1`, owner.String()); err != nil {
		t.Fatal(err)
	}
	anchor := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	a, err := service.CreateAccount(ctx, owner, AccountInput{Name: "Option B synthetic bank", Type: "bank", Currency: "MYR", OpeningBalanceMinor: 100000, OpeningBalanceAt: &anchor}, newFinancialWriteKey())
	if err != nil {
		t.Fatal(err)
	}
	post := func(amount int64, at time.Time) Event {
		t.Helper()
		e, x := service.PostExpense(ctx, owner, EventInput{AccountID: a.ID, AmountMinor: amount, Currency: "MYR", OccurredAt: at}, newFinancialWriteKey())
		if x != nil {
			t.Fatal(x)
		}
		return e
	}
	target := post(40000, anchor.Add(-8*time.Hour))
	for i := 1; i <= 7; i++ {
		post(int64(100+i), anchor.Add(time.Duration(i-8)*time.Hour))
	}
	posts := []Event{}
	for i := 0; i < 65; i++ {
		posts = append(posts, post(100, anchor.Add(time.Duration(i)*time.Hour)))
	}
	read := func() (map[string]HistoryItem, HistoryPage) {
		t.Helper()
		all := map[string]HistoryItem{}
		q := HistoryQuery{Limit: 25}
		var first HistoryPage
		var previous *Event
		for batch := 0; ; batch++ {
			p, x := service.HistoryPage(ctx, owner, q)
			if x != nil {
				t.Fatal(x)
			}
			if batch == 0 {
				first = p
			}
			for _, item := range p.Items {
				if _, duplicate := all[item.Event.ID]; duplicate {
					t.Fatal("duplicate across 25-row batches")
				}
				if previous != nil && (item.Event.OccurredAt.After(previous.OccurredAt) || item.Event.OccurredAt.Equal(previous.OccurredAt) && item.Event.ID >= previous.ID) {
					t.Fatal("batch ordering")
				}
				copy := item.Event
				previous = &copy
				all[item.Event.ID] = item
			}
			if !p.HasMore {
				break
			}
			if p.NextCursor == nil || batch > 10 {
				t.Fatal("unbounded continuation")
			}
			q = HistoryQuery{Limit: 25, Revision: p.Revision, Cursor: *p.NextCursor}
		}
		return all, first
	}
	value := func(all map[string]HistoryItem, id, want string) {
		t.Helper()
		i := all[id]
		if len(i.Positions) != 1 || i.Positions[0].BalanceAfterMinor == nil || *i.Positions[0].BalanceAfterMinor != want {
			t.Fatalf("event %s got %+v want %s", id, i.Positions, want)
		}
	}
	current := func(want int64) {
		t.Helper()
		b, x := service.Balance(ctx, owner, a.ID)
		if x != nil || b.AmountMinor != want {
			t.Fatal(b, x, want)
		}
	}
	all, old := read()
	if len(all) != 73 {
		t.Fatal("missing rows", len(all))
	}
	value(all, target.ID, "100728")
	current(93500)
	updated, err := service.EditEvent(ctx, owner, target.ID, 1, EditInput{Kind: "expense", EventInput: EventInput{AccountID: a.ID, AmountMinor: 80000, Currency: "MYR", OccurredAt: target.OccurredAt}}, newFinancialWriteKey())
	if err != nil || updated.ID != target.ID {
		t.Fatal(updated, err)
	}
	all, _ = read()
	value(all, target.ID, "100728")
	current(93500)
	if all[target.ID].Positions[0].Basis != "reconstructed_pre_anchor" {
		t.Fatal("missing basis")
	}
	if _, err = service.HistoryPage(ctx, owner, HistoryQuery{Limit: 25, Revision: old.Revision, Cursor: *old.NextCursor}); !errors.Is(err, ErrHistoryChanged) {
		t.Fatal("old continuation accepted", err)
	}
	// A newly entered later post-anchor expense cannot rewrite an earlier after-position.
	later := post(200, anchor.Add(100*time.Hour))
	all, _ = read()
	value(all, posts[0].ID, "99900")
	value(all, target.ID, "100728")
	current(93300)
	// Editing an event in an older batch changes its own and all later positions.
	_, err = service.EditEvent(ctx, owner, posts[0].ID, 1, EditInput{Kind: "expense", EventInput: EventInput{AccountID: a.ID, AmountMinor: 300, Currency: "MYR", OccurredAt: posts[0].OccurredAt}}, newFinancialWriteKey())
	if err != nil {
		t.Fatal(err)
	}
	all, _ = read()
	value(all, posts[0].ID, "99700")
	value(all, later.ID, "93100")
	current(93100)
	if err = service.DeleteEvent(ctx, owner, posts[5].ID, newFinancialWriteKey(), 1); err != nil {
		t.Fatal(err)
	}
	all, _ = read()
	if _, exists := all[posts[5].ID]; exists {
		t.Fatal("deleted older record remained")
	}
	value(all, posts[0].ID, "99700")
	current(93200)
	// Moving an edited older record reorders the same identity into the latest batch.
	_, err = service.EditEvent(ctx, owner, posts[0].ID, 2, EditInput{Kind: "expense", EventInput: EventInput{AccountID: a.ID, AmountMinor: 300, Currency: "MYR", OccurredAt: anchor.Add(101 * time.Hour)}}, newFinancialWriteKey())
	if err != nil {
		t.Fatal(err)
	}
	all, first := read()
	if first.Items[0].Event.ID != posts[0].ID || len(all) != 73 {
		t.Fatal("moved event missing or duplicated")
	}
	value(all, posts[0].ID, "93200")
	current(93200)
}
