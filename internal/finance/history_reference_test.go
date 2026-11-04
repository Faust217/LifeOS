package finance

import (
	"context"
	"math/big"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Independent reference: forward from the anchor, or backwards by subtracting
// later pre-anchor movements. It does not repeat the SQL prefix-minus-Q formula.
func verifyHistoryReference(t *testing.T, fixture *pgxpool.Pool, service *Service, unrelated identity.OwnerID) {
	t.Helper()
	ctx := context.Background()
	owner := verifiedTestOwner(t, fixture, "43434343-4343-4343-8343-434343434343", "history-reference")
	if _, err := fixture.Exec(ctx, `UPDATE lifeos_users SET time_zone='Asia/Kuala_Lumpur' WHERE id=$1`, owner.String()); err != nil {
		t.Fatal(err)
	}
	anchor := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	accounts := []Account{}
	for i, name := range []string{"Reference bank", "Reference cash", "Reference dollars"} {
		currency := "MYR"
		if i == 2 {
			currency = "USD"
		}
		a, err := service.CreateAccount(ctx, owner, AccountInput{Name: name, Type: "bank", Currency: currency, OpeningBalanceMinor: 10000, OpeningBalanceAt: &anchor}, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		accounts = append(accounts, a)
	}
	legacy, err := legacyTestAccount(ctx, NewService(fixture), owner, AccountInput{Name: "Reference legacy", Type: "cash", Currency: "MYR", OpeningBalanceMinor: 10000})
	if err != nil {
		t.Fatal(err)
	}
	accounts = append(accounts, legacy)
	c1, err := service.CreateCategory(ctx, owner, CategoryInput{Name: "Reference Food", Kind: "expense"})
	if err != nil {
		t.Fatal(err)
	}
	c2, err := service.CreateCategory(ctx, owner, CategoryInput{Name: "Reference Medical", Kind: "expense"})
	if err != nil {
		t.Fatal(err)
	}
	random := rand.New(rand.NewPCG(36, 20261102))
	for i := 0; i < 102; i++ {
		a := accounts[random.IntN(len(accounts))]
		amount := int64(random.IntN(99) + 2)
		instant := anchor.Add(time.Duration(random.IntN(11)-5) * time.Hour)
		input := EventInput{AccountID: a.ID, Currency: a.Currency, AmountMinor: amount, OccurredAt: instant}
		var event Event
		if i%5 == 0 {
			from := accounts[0]
			to := accounts[1]
			if i%2 == 0 {
				to = legacy
			}
			event, err = service.Transfer(ctx, owner, TransferInput{FromAccountID: from.ID, ToAccountID: to.ID, Currency: "MYR", AmountMinor: amount, OccurredAt: instant}, newFinancialWriteKey())
		} else if i%3 == 0 {
			event, err = service.PostIncome(ctx, owner, input, newFinancialWriteKey())
		} else {
			if i%4 == 0 {
				input.Allocations = []AllocationInput{{c1.ID, amount / 2}, {c2.ID, amount - amount/2}}
			}
			event, err = service.PostExpense(ctx, owner, input, newFinancialWriteKey())
		}
		if err != nil {
			t.Fatal(err)
		}
		if i%13 == 0 {
			if err = service.VoidEvent(ctx, owner, event.ID, "Synthetic reference", newFinancialWriteKey(), 1); err != nil {
				t.Fatal(err)
			}
			if i%26 == 0 {
				if err = service.DeleteEvent(ctx, owner, event.ID, newFinancialWriteKey(), 2); err != nil {
					t.Fatal(err)
				}
			}
		} else if i%17 == 0 {
			if err = service.DeleteEvent(ctx, owner, event.ID, newFinancialWriteKey(), 1); err != nil {
				t.Fatal(err)
			}
		} else if i%19 == 0 {
			edit := EditInput{Kind: event.Kind, EventInput: EventInput{AmountMinor: event.AmountMinor + 3, Currency: event.Currency, OccurredAt: instant.Add(-time.Hour)}}
			if event.Kind == "transfer" {
				for _, m := range event.Movements {
					if m.AmountMinor < 0 {
						edit.FromAccountID = m.AccountID
					} else {
						edit.ToAccountID = m.AccountID
					}
				}
			} else {
				edit.AccountID = event.Movements[0].AccountID
				if len(event.Allocations) > 0 {
					edit.Allocations = []AllocationInput{{c1.ID, edit.AmountMinor / 2}, {c2.ID, edit.AmountMinor - edit.AmountMinor/2}}
				}
			}
			if _, err = service.EditEvent(ctx, owner, event.ID, 1, edit, newFinancialWriteKey()); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = service.ArchiveAccount(ctx, owner, accounts[1].ID); err != nil {
		t.Fatal(err)
	}
	if err = service.ArchiveCategory(ctx, owner, c2.ID); err != nil {
		t.Fatal(err)
	}
	type movement struct {
		id, event, account, currency, status string
		amount, opening                      int64
		at                                   time.Time
		anchor, deleted                      *time.Time
	}
	rows, err := fixture.Query(ctx, `SELECT m.id,e.id,m.account_id,m.currency,e.status,m.amount_minor,a.opening_balance_minor,e.occurred_at,a.opening_balance_at,e.deleted_at FROM account_movements m JOIN financial_events e ON(e.owner_id=m.owner_id AND e.id=m.event_id) JOIN finance_accounts a ON(a.owner_id=m.owner_id AND a.id=m.account_id) WHERE m.owner_id=$1`, owner.String())
	if err != nil {
		t.Fatal(err)
	}
	all := []movement{}
	byID := map[string]movement{}
	visible := map[string]bool{}
	for rows.Next() {
		var m movement
		if err = rows.Scan(&m.id, &m.event, &m.account, &m.currency, &m.status, &m.amount, &m.opening, &m.at, &m.anchor, &m.deleted); err != nil {
			t.Fatal(err)
		}
		all = append(all, m)
		byID[m.id] = m
		if m.deleted == nil {
			visible[m.event] = true
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	compare := func(a, b movement) int {
		if a.at.Before(b.at) {
			return -1
		}
		if a.at.After(b.at) {
			return 1
		}
		if a.event < b.event {
			return -1
		}
		if a.event > b.event {
			return 1
		}
		return 0
	}
	seen := map[string]bool{}
	q := HistoryQuery{Limit: 7}
	lastKey := ""
	var previousEvent *Event
	for pages := 0; ; pages++ {
		p, e := service.HistoryPage(ctx, owner, q)
		if e != nil {
			t.Fatal(e)
		}
		if pages == 0 {
			lastKey = p.Revision
			if _, e = service.CreateCategory(ctx, unrelated, CategoryInput{Name: "Other owner's reference change", Kind: "income"}); e != nil {
				t.Fatal(e)
			}
		} else if p.Revision != lastKey {
			t.Fatal("different owner invalidated projection")
		}
		for _, item := range p.Items {
			if seen[item.Event.ID] {
				t.Fatal("duplicate event across keyset pages")
			}
			seen[item.Event.ID] = true
			if previousEvent != nil && (item.Event.OccurredAt.After(previousEvent.OccurredAt) || item.Event.OccurredAt.Equal(previousEvent.OccurredAt) && item.Event.ID >= previousEvent.ID) {
				t.Fatal("unstable equal-instant order")
			}
			eventCopy := item.Event
			previousEvent = &eventCopy
			if len(item.Event.Movements) != len(item.Positions) {
				t.Fatal("movement projection cardinality")
			}
			for _, position := range item.Positions {
				m := byID[position.MovementID]
				reason := ""
				basis := "anchored"
				if m.anchor == nil {
					basis = "legacy_unconfirmed"
				} else if m.at.Before(*m.anchor) {
					basis = "reconstructed_pre_anchor"
				}
				if m.status == "voided" {
					reason = "voided"
				} else if m.anchor == nil {
					reason = "anchor_required"
				}
				if position.Basis != basis || position.Currency != m.currency || position.AccountID != m.account {
					t.Fatal("wrong account/basis context")
				}
				if reason != "" {
					if position.BalanceAfterMinor != nil || position.UnavailableReason == nil || *position.UnavailableReason != reason {
						t.Fatal("wrong unavailable position")
					}
					continue
				}
				expected := big.NewInt(m.opening)
				for _, other := range all {
					if other.account != m.account || other.status != "posted" || other.deleted != nil {
						continue
					}
					if !m.at.Before(*m.anchor) {
						if !other.at.Before(*m.anchor) && compare(other, m) <= 0 {
							expected.Add(expected, big.NewInt(other.amount))
						}
					}
					if m.at.Before(*m.anchor) {
						if other.at.Before(*m.anchor) && compare(other, m) > 0 {
							expected.Sub(expected, big.NewInt(other.amount))
						}
					}
				}
				if position.BalanceAfterMinor == nil || *position.BalanceAfterMinor != expected.String() || position.UnavailableReason != nil {
					t.Fatalf("reference mismatch event=%s got=%+v expected=%s", m.event, position, expected)
				}
			}
		}
		if !p.HasMore {
			if p.NextCursor != nil {
				t.Fatal("cursor after final page")
			}
			break
		}
		if p.NextCursor == nil || pages > 102 {
			t.Fatal("invalid continuation")
		}
		q = HistoryQuery{Limit: 7, Revision: p.Revision, Cursor: *p.NextCursor}
	}
	if len(seen) != len(visible) {
		t.Fatalf("missing visible events: %d/%d", len(seen), len(visible))
	}
}
