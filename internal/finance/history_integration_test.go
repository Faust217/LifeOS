package finance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRunningHistoryIntegration(t *testing.T) {
	fixture, runner := financeTestDatabase(t, "LIFEOS_HISTORY_TEST_DATABASE_URL")
	ctx := context.Background()
	if err := runner.Migrate(10); err != nil {
		t.Fatal(err)
	}
	owner := verifiedTestOwner(t, fixture, "37373737-3737-4737-8737-373737373737", "history-owner")
	other := verifiedTestOwner(t, fixture, "38383838-3838-4838-8838-383838383838", "history-other")
	if _, err := fixture.Exec(ctx, `UPDATE lifeos_users SET time_zone='Asia/Kuala_Lumpur' WHERE id=$1`, owner.String()); err != nil {
		t.Fatal(err)
	}
	legacy, err := legacyTestAccount(ctx, NewService(fixture), owner, AccountInput{Name: "Old bank", Type: "bank", Currency: "MYR", OpeningBalanceMinor: 1000})
	if err != nil {
		t.Fatal(err)
	}
	oldID := newFinancialWriteKey()
	if _, err = fixture.Exec(ctx, `INSERT INTO financial_events(id,owner_id,kind,amount_minor,currency,occurred_at) VALUES($1,$2,'expense',10,'MYR','2026-10-01T00:00:00Z')`, oldID, owner.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.Exec(ctx, `INSERT INTO account_movements(id,owner_id,event_id,account_id,amount_minor,currency) VALUES(gen_random_uuid(),$1,$2,$3,-10,'MYR')`, owner.String(), oldID, legacy.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.Exec(ctx, `SELECT capture_financial_event_revision($1,$2,'legacy_baseline',NULL)`, owner.String(), oldID); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"voided", "deleted"} {
		id := newFinancialWriteKey()
		if _, err = fixture.Exec(ctx, `INSERT INTO financial_events(id,owner_id,kind,amount_minor,currency,occurred_at,status,voided_at,void_reason,deleted_at) VALUES($1,$2,'expense',10,'MYR','2026-09-29T00:00:00Z',CASE WHEN $3='voided' THEN 'voided' ELSE 'posted' END,CASE WHEN $3='voided' THEN now() END,CASE WHEN $3='voided' THEN 'Synthetic prior void' END,CASE WHEN $3='deleted' THEN now() END)`, id, owner.String(), state); err != nil {
			t.Fatal(err)
		}
		if _, err = fixture.Exec(ctx, `INSERT INTO account_movements(id,owner_id,event_id,account_id,amount_minor,currency) VALUES(gen_random_uuid(),$1,$2,$3,-10,'MYR')`, owner.String(), id, legacy.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = fixture.Exec(ctx, `SELECT capture_financial_event_revision($1,$2,'legacy_baseline',NULL)`, owner.String(), id); err != nil {
			t.Fatal(err)
		}
	}
	ledgerDigest := func() string {
		t.Helper()
		var digest string
		if err := fixture.QueryRow(ctx, `SELECT md5(jsonb_build_object(
		'accounts',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM finance_accounts t WHERE owner_id=$1),
		'categories',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM finance_categories t WHERE owner_id=$1),
		'events',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM financial_events t WHERE owner_id=$1),
		'movements',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM account_movements t WHERE owner_id=$1),
		'allocations',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM category_allocations t WHERE owner_id=$1),
		'audit',(SELECT jsonb_agg(to_jsonb(t) ORDER BY event_id,revision) FROM financial_event_revisions t WHERE owner_id=$1),
		'keys',(SELECT jsonb_agg(to_jsonb(t) ORDER BY request_key) FROM finance_write_requests t WHERE owner_id=$1),
		'confirmations',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM finance_opening_confirmations t WHERE owner_id=$1))::text)`, owner.String()).Scan(&digest); err != nil {
			t.Fatal(err)
		}
		return digest
	}
	priorDigest := ledgerDigest()
	oldConn, err := pgx.Connect(ctx, os.Getenv("LIFEOS_HISTORY_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer oldConn.Close(ctx)
	if _, err = oldConn.Prepare(ctx, "old_writer", `UPDATE finance_accounts SET name='old process' WHERE id=$1`); err != nil {
		t.Fatal(err)
	}
	// An already-running v10 write must drain before the baseline/fence is installed.
	third := verifiedTestOwner(t, fixture, "39393939-3939-4939-8939-393939393939", "upgrade-inflight")
	inflight, err := oldConn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = inflight.Exec(ctx, `INSERT INTO finance_categories(id,owner_id,name,kind) VALUES(gen_random_uuid(),$1,'In-flight v10 category','expense')`, third.String()); err != nil {
		t.Fatal(err)
	}
	upgraded := make(chan error, 1)
	go func() { upgraded <- runner.Up() }()
	select {
	case err := <-upgraded:
		t.Fatalf("upgrade did not wait for old writer: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err = inflight.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-upgraded; err != nil {
		t.Fatal(err)
	}
	if ledgerDigest() != priorDigest {
		t.Fatal("v11 changed existing ledger/audit/key facts")
	}
	var baseline int64
	if err = fixture.QueryRow(ctx, `SELECT revision FROM finance_history_revisions WHERE owner_id=$1`, third.String()).Scan(&baseline); err != nil || baseline != 0 {
		t.Fatal("in-flight owner omitted from baseline", err)
	}
	if _, err = oldConn.Exec(ctx, "old_writer", legacy.ID); err == nil {
		t.Fatal("v10 prepared writer bypassed fence")
	} else {
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != "55000" {
			t.Fatal(err)
		}
	}
	// The production-path pool has no session fixture marker, including on reconnect.
	pool, err := pgxpool.New(ctx, os.Getenv("LIFEOS_HISTORY_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	service := NewService(pool)
	before, err := service.HistoryPage(ctx, owner, HistoryQuery{Limit: 10})
	if err != nil || len(before.Items) != 2 || before.Items[0].Positions[0].UnavailableReason == nil || *before.Items[0].Positions[0].UnavailableReason != "anchor_required" {
		t.Fatal(before, err)
	}
	if err = runner.Steps(-1); err != nil {
		t.Fatal("baseline-only v11 down", err)
	}
	if err = runner.Up(); err != nil {
		t.Fatal(err)
	}
	again, err := service.HistoryPage(ctx, owner, HistoryQuery{Limit: 10})
	if err != nil || again.Revision == before.Revision {
		t.Fatal("epoch reuse", err)
	}
	revision := func() int64 {
		t.Helper()
		var n int64
		if err := pool.QueryRow(ctx, `SELECT revision FROM finance_history_revisions WHERE owner_id=$1`, owner.String()).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	account := func(name, currency string, opening int64, anchor time.Time) Account {
		t.Helper()
		a, e := service.CreateAccount(ctx, owner, AccountInput{Name: name, Type: "bank", Currency: currency, OpeningBalanceMinor: opening, OpeningBalanceAt: &anchor}, newFinancialWriteKey())
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	bank := account("Everyday bank", "MYR", 100000, at)
	cash := account("Cash", "MYR", 20000, at)
	expense, err := service.PostExpense(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 5000, Currency: "MYR", OccurredAt: at.Add(24 * time.Hour)}, newFinancialWriteKey())
	if err != nil {
		t.Fatal(err)
	}
	transferKey := newFinancialWriteKey()
	transferInput := TransferInput{FromAccountID: bank.ID, ToAccountID: cash.ID, AmountMinor: 10000, Currency: "MYR", OccurredAt: at.Add(48 * time.Hour)}
	transfer, err := service.Transfer(ctx, owner, transferInput, transferKey)
	if err != nil {
		t.Fatal(err)
	}
	page := func() HistoryPage {
		t.Helper()
		p, e := service.HistoryPage(ctx, owner, HistoryQuery{Limit: 100})
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	item := func(p HistoryPage, id string) HistoryItem {
		t.Helper()
		for _, i := range p.Items {
			if i.Event.ID == id {
				return i
			}
		}
		t.Fatal("missing event", id)
		return HistoryItem{}
	}
	position := func(i HistoryItem, id string) HistoryPosition {
		t.Helper()
		for _, p := range i.Positions {
			if p.AccountID == id {
				return p
			}
		}
		t.Fatal("missing position")
		return HistoryPosition{}
	}
	value := func(p HistoryPage, event, account, want string) {
		t.Helper()
		got := position(item(p, event), account)
		if got.BalanceAfterMinor == nil || *got.BalanceAfterMinor != want {
			t.Fatalf("balance got=%+v want=%s", got, want)
		}
	}
	value(page(), expense.ID, bank.ID, "95000")
	value(page(), transfer.ID, bank.ID, "85000")
	value(page(), transfer.ID, cash.ID, "30000")
	t.Run("empty owner and account confirmation revision", func(t *testing.T) {
		empty, e := service.HistoryPage(ctx, other, HistoryQuery{Limit: 10})
		if e != nil || len(empty.Items) != 0 || empty.HasMore || empty.NextCursor != nil {
			t.Fatal(empty, e)
		}
		n := revision()
		preview, e := service.PreviewOpeningBalance(ctx, owner, legacy.ID, at, nil)
		if e != nil {
			t.Fatal(e)
		}
		if revision() != n {
			t.Fatal("preview advanced revision")
		}
		key := newFinancialWriteKey()
		amount, _ := strconv.ParseInt(preview.SuggestedOpeningMinor, 10, 64)
		rev, _ := strconv.ParseInt(preview.BalanceRevision, 10, 64)
		if _, e = service.ConfirmOpeningBalance(ctx, owner, legacy.ID, at, amount, rev, false, key); e != nil {
			t.Fatal(e)
		}
		if revision() != n+1 {
			t.Fatal("confirmation counter")
		}
		if _, e = service.ConfirmOpeningBalance(ctx, owner, legacy.ID, at, amount, rev, false, key); !errors.Is(e, ErrAlreadyApplied) || revision() != n+1 {
			t.Fatal("confirmation replay", e)
		}
	})
	t.Run("exact instant ordering and currencies", func(t *testing.T) {
		for _, currency := range []string{"MYR", "JPY", "KWD", "CLF"} {
			a := account("Order "+currency, currency, 1000, at)
			ids := []string{}
			for i := 0; i < 3; i++ {
				e, x := service.PostExpense(ctx, owner, EventInput{AccountID: a.ID, AmountMinor: 10, Currency: currency, OccurredAt: at}, newFinancialWriteKey())
				if x != nil {
					t.Fatal(x)
				}
				ids = append(ids, e.ID)
			}
			sort.Strings(ids)
			p := page()
			for i, id := range ids {
				value(p, id, a.ID, strconv.Itoa(1000-(i+1)*10))
				if position(item(p, id), a.ID).Basis != "anchored" {
					t.Fatal("anchor must precede equal-time events")
				}
			}
		}
	})
	t.Run("keyset full partitions and stale old edits", func(t *testing.T) {
		first, e := service.HistoryPage(ctx, owner, HistoryQuery{Limit: 1})
		if e != nil || !first.HasMore || first.NextCursor == nil {
			t.Fatal(first, e)
		}
		value(first, transfer.ID, bank.ID, "85000")
		second, e := service.HistoryPage(ctx, owner, HistoryQuery{Limit: 1, Revision: first.Revision, Cursor: *first.NextCursor})
		if e != nil {
			t.Fatal(e)
		}
		value(second, expense.ID, bank.ID, "95000")
		in := EditInput{Kind: "expense", EventInput: EventInput{AccountID: bank.ID, AmountMinor: 8000, Currency: "MYR", OccurredAt: expense.OccurredAt, Allocations: []AllocationInput{}}}
		key := newFinancialWriteKey()
		n := revision()
		if _, e = service.EditEvent(ctx, owner, expense.ID, 1, in, key); e != nil {
			t.Fatal(e)
		}
		if revision() != n+1 {
			t.Fatal("edit revision count")
		}
		if _, e = service.EditEvent(ctx, owner, expense.ID, 1, in, key); !errors.Is(e, ErrAlreadyApplied) || revision() != n+1 {
			t.Fatal("replay advanced history", e)
		}
		if _, e = service.HistoryPage(ctx, owner, HistoryQuery{Limit: 1, Revision: first.Revision, Cursor: *first.NextCursor}); !errors.Is(e, ErrHistoryChanged) {
			t.Fatal(e)
		}
		value(page(), expense.ID, bank.ID, "92000")
		value(page(), transfer.ID, bank.ID, "82000")
	})
	t.Run("independent complete-ledger reference across many pages", func(t *testing.T) { verifyHistoryReference(t, fixture, service, owner) })
	t.Run("Option B anchor explanation and continuous batches", func(t *testing.T) { verifyHistoryOptionB(t, fixture, service) })
	t.Run("negative positions and future posted events", func(t *testing.T) {
		a, e := service.CreateAccount(ctx, owner, AccountInput{Name: "Recorded card", Type: "credit_card", Currency: "MYR", OpeningBalanceMinor: -1000, OpeningBalanceAt: &at}, newFinancialWriteKey())
		if e != nil {
			t.Fatal(e)
		}
		future, e := service.PostExpense(ctx, owner, EventInput{AccountID: a.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: at.AddDate(1, 0, 0)}, newFinancialWriteKey())
		if e != nil {
			t.Fatal(e)
		}
		value(page(), future.ID, a.ID, "-1100")
		current, e := service.Balance(ctx, owner, a.ID)
		if e != nil || current.AmountMinor != -1100 {
			t.Fatal(current, e)
		}
	})
	t.Run("pre-anchor reconstruction and independent transfer legs", func(t *testing.T) {
		a := account("Dated bank", "MYR", 100000, at.Add(9*24*time.Hour))
		post := func(day int, amount int64) Event {
			e, x := service.PostExpense(ctx, owner, EventInput{AccountID: a.ID, AmountMinor: amount, Currency: "MYR", OccurredAt: at.Add(time.Duration(day-1) * 24 * time.Hour)}, newFinancialWriteKey())
			if x != nil {
				t.Fatal(x)
			}
			return e
		}
		e8 := post(8, 3000)
		e9 := post(9, 2000)
		e7 := post(7, 1000)
		p := page()
		value(p, e8.ID, a.ID, "102000")
		value(p, e9.ID, a.ID, "100000")
		value(p, e7.ID, a.ID, "105000")
		if position(item(p, e8.ID), a.ID).Basis != "reconstructed_pre_anchor" {
			t.Fatal("unlabelled reconstruction")
		}
		b, e := service.Balance(ctx, owner, a.ID)
		if e != nil || b.AmountMinor != 100000 {
			t.Fatal(b, e)
		}
		savings := account("Later savings", "MYR", 20000, at.Add(19*24*time.Hour))
		tr, e := service.Transfer(ctx, owner, TransferInput{FromAccountID: a.ID, ToAccountID: savings.ID, AmountMinor: 5000, Currency: "MYR", OccurredAt: at.Add(14 * 24 * time.Hour)}, newFinancialWriteKey())
		if e != nil {
			t.Fatal(e)
		}
		value(page(), tr.ID, savings.ID, "20000")
		if _, e = service.EditEvent(ctx, owner, tr.ID, 1, EditInput{Kind: "transfer", FromAccountID: a.ID, ToAccountID: savings.ID, EventInput: EventInput{AmountMinor: 5000, Currency: "MYR", OccurredAt: at.Add(20 * 24 * time.Hour)}}, newFinancialWriteKey()); e != nil {
			t.Fatal(e)
		}
		p = page()
		value(p, tr.ID, savings.ID, "25000")
		if len(item(p, tr.ID).Positions) != 2 {
			t.Fatal("split transfer")
		}
	})
	t.Run("management no-ops and archived complete splits", func(t *testing.T) {
		n := revision()
		food, e := service.CreateCategory(ctx, owner, CategoryInput{Name: "Food", Kind: "expense"})
		if e != nil || revision() != n+1 {
			t.Fatal(e)
		}
		if _, e = service.RenameCategory(ctx, owner, food.ID, "Food"); e != nil || revision() != n+1 {
			t.Fatal("same-name no-op", e)
		}
		if _, e = service.RenameCategory(ctx, owner, food.ID, "Meals"); e != nil || revision() != n+2 {
			t.Fatal("rename counter", e)
		}
		medical, e := service.CreateCategory(ctx, owner, CategoryInput{Name: "Medical", Kind: "expense"})
		if e != nil {
			t.Fatal(e)
		}
		eSplit, e := service.PostExpense(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 1200, Currency: "MYR", OccurredAt: at.Add(60 * time.Hour), Allocations: []AllocationInput{{food.ID, 700}, {medical.ID, 500}}}, newFinancialWriteKey())
		if e != nil {
			t.Fatal(e)
		}
		for _, op := range []func() error{func() error { return service.ArchiveCategory(ctx, owner, medical.ID) }, func() error { return service.RestoreCategory(ctx, owner, medical.ID) }, func() error { return service.ArchiveAccount(ctx, owner, cash.ID) }, func() error { return service.RestoreAccount(ctx, owner, cash.ID) }} {
			n = revision()
			if e = op(); e != nil || revision() != n+1 {
				t.Fatal("management history counter", e)
			}
		}
		_ = service.ArchiveCategory(ctx, owner, medical.ID)
		p := page()
		if len(item(p, eSplit.ID).Event.Allocations) != 2 {
			t.Fatal("split lost")
		}
		found := false
		for _, c := range p.Categories {
			if c.ID == medical.ID && c.ArchivedAt != nil {
				found = true
			}
		}
		if !found {
			t.Fatal("archived context missing")
		}
		n = revision()
		if e = service.ArchiveCategory(ctx, owner, medical.ID); !errors.Is(e, ErrNotFound) || revision() != n {
			t.Fatal("rejected archive no-op advanced history", e)
		}
	})
	t.Run("void delete and overflow are explicit", func(t *testing.T) {
		n := revision()
		if e := service.VoidEvent(ctx, owner, expense.ID, "", newFinancialWriteKey(), 2); e != nil {
			t.Fatal(e)
		}
		if revision() != n+1 {
			t.Fatal("void counter")
		}
		pos := position(item(page(), expense.ID), bank.ID)
		if pos.BalanceAfterMinor != nil || pos.UnavailableReason == nil || *pos.UnavailableReason != "voided" {
			t.Fatal(pos)
		}
		value(page(), transfer.ID, bank.ID, "90000")
		key := newFinancialWriteKey()
		if e := service.DeleteEvent(ctx, owner, expense.ID, key, 3); e != nil {
			t.Fatal(e)
		}
		n = revision()
		if e := service.DeleteEvent(ctx, owner, expense.ID, newFinancialWriteKey(), 1); e != nil || revision() != n {
			t.Fatal("no-op delete", e)
		}
		if e := service.DeleteEvent(ctx, owner, expense.ID, key, 3); !errors.Is(e, ErrAlreadyApplied) || revision() != n {
			t.Fatal("delete replay", e)
		}
		max := account("Limits", "MYR", math.MaxInt64, at)
		if _, e := service.PostExpense(ctx, owner, EventInput{AccountID: max.ID, AmountMinor: 1, Currency: "MYR", OccurredAt: at.Add(2 * 24 * time.Hour)}, newFinancialWriteKey()); e != nil {
			t.Fatal(e)
		}
		income, e := service.PostIncome(ctx, owner, EventInput{AccountID: max.ID, AmountMinor: 1, Currency: "MYR", OccurredAt: at.Add(24 * time.Hour)}, newFinancialWriteKey())
		if e != nil {
			t.Fatal(e)
		}
		pos = position(item(page(), income.ID), max.ID)
		if pos.BalanceAfterMinor != nil || pos.UnavailableReason == nil || *pos.UnavailableReason != "amount_out_of_range" {
			t.Fatal(pos)
		}
	})
	t.Run("timezone and owner bound pages", func(t *testing.T) {
		p := page()
		if _, e := service.HistoryPage(ctx, other, HistoryQuery{Limit: 100, Revision: p.Revision}); !errors.Is(e, ErrHistoryChanged) {
			t.Fatal("foreign token", e)
		}
		n := revision()
		if _, e := fixture.Exec(ctx, `UPDATE lifeos_users SET time_zone='America/New_York' WHERE id=$1`, owner.String()); e != nil {
			t.Fatal(e)
		}
		if _, e := service.HistoryPage(ctx, owner, HistoryQuery{Limit: 100, Revision: p.Revision}); !errors.Is(e, ErrHistoryChanged) || revision() != n {
			t.Fatal("timezone binding", e)
		}
		if page().TimeZone == nil || *page().TimeZone != "America/New_York" {
			t.Fatal("device-independent timezone")
		}
	})
	t.Run("one read snapshot under concurrent mutations", func(t *testing.T) {
		for _, mode := range []string{"Edit", "Void", "Delete"} {
			t.Run(mode, func(t *testing.T) {
				e, x := service.PostIncome(ctx, owner, EventInput{AccountID: cash.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: at}, newFinancialWriteKey())
				if x != nil {
					t.Fatal(x)
				}
				trace := &historySnapshotTracer{reached: make(chan struct{}), release: make(chan struct{})}
				previous := position(item(page(), e.ID), cash.ID).BalanceAfterMinor
				cfg := pool.Config()
				cfg.ConnConfig.Tracer = trace
				reader, x := pgxpool.NewWithConfig(ctx, cfg)
				if x != nil {
					t.Fatal(x)
				}
				defer reader.Close()
				result := make(chan HistoryPage, 1)
				failure := make(chan error, 1)
				go func() {
					p, e := NewService(reader).HistoryPage(ctx, owner, HistoryQuery{Limit: 100})
					if e != nil {
						failure <- e
					} else {
						result <- p
					}
				}()
				select {
				case <-trace.reached:
				case <-time.After(3 * time.Second):
					t.Fatal("snapshot not reached")
				}
				switch mode {
				case "Edit":
					_, x = service.EditEvent(ctx, owner, e.ID, 1, EditInput{Kind: "income", EventInput: EventInput{AccountID: cash.ID, AmountMinor: 200, Currency: "MYR", OccurredAt: at}}, newFinancialWriteKey())
				case "Void":
					x = service.VoidEvent(ctx, owner, e.ID, "Synthetic concurrent void", newFinancialWriteKey(), 1)
				case "Delete":
					x = service.DeleteEvent(ctx, owner, e.ID, newFinancialWriteKey(), 1)
				}
				if x != nil {
					t.Fatal(x)
				}
				close(trace.release)
				select {
				case p := <-result:
					old := item(p, e.ID)
					if old.Event.AmountMinor != 100 || old.Event.Revision != 1 || old.Event.Movements[0].AmountMinor != 100 {
						t.Fatal("mixed header")
					}
					oldPosition := position(old, cash.ID).BalanceAfterMinor
					if previous == nil || oldPosition == nil || *previous != *oldPosition {
						t.Fatal("mixed financial position")
					}
					if _, x = service.HistoryPage(ctx, owner, HistoryQuery{Limit: 100, Revision: p.Revision}); !errors.Is(x, ErrHistoryChanged) {
						t.Fatal(x)
					}
				case x := <-failure:
					t.Fatal(x)
				}
			})
		}
	})
	t.Run("guarded HTTP and privacy errors", func(t *testing.T) {
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": "http://" + r.Host, "authorization_endpoint": "http://" + r.Host + "/authorize", "token_endpoint": "http://" + r.Host + "/token", "jwks_uri": "http://" + r.Host + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
		}))
		defer provider.Close()
		router, _, _ := idempotencyTestRouter(t, pool, provider.URL)
		send := func(path, secret string) *httptest.ResponseRecorder {
			r := httptest.NewRequest("GET", "/api/v1/events"+path, nil)
			if secret != "" {
				r.AddCookie(&http.Cookie{Name: "lifeos_session", Value: secret})
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			return w
		}
		good := send("?view=running_balances", "history-owner")
		moneyStatus(t, good, 200)
		if !strings.Contains(good.Header().Get("Cache-Control"), "private") {
			t.Fatal("private header")
		}
		for _, path := range []string{"?view=bad", "?view=running_balances&offset=0", "?view=running_balances&limit=101", "?view=running_balances&limit=1&limit=2", "?view=running_balances&cursor=bad"} {
			moneyStatus(t, send(path, "history-owner"), 400)
		}
		denied := send("?view=running_balances", "")
		moneyStatus(t, denied, 401)
		if !strings.Contains(denied.Header().Get("Cache-Control"), "private") {
			t.Fatal("guard privacy")
		}
		moneyStatus(t, send("?view=running_balances&owner_id="+other.String(), "history-owner"), 400)
		moneyStatus(t, send("?view=running_balances&history_revision="+strings.Repeat("0", 64), "history-owner"), 409)
		moneyStatus(t, send("?limit=1&offset=0", "history-owner"), 200)
		if _, e := fixture.Exec(ctx, `UPDATE lifeos_sessions SET revoked_at=now() WHERE user_id=$1`, owner.String()); e != nil {
			t.Fatal(e)
		}
		moneyStatus(t, send("?view=running_balances", "history-owner"), 401)
	})
	t.Run("cancelled reads fail closed", func(t *testing.T) {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, e := service.HistoryPage(cancelled, owner, HistoryQuery{Limit: 10}); !errors.Is(e, ErrHistoryUnavailable) {
			t.Fatal(e)
		}
	})
	t.Run("revision overflow rolls back keys ledger and audit", func(t *testing.T) {
		var eventsBefore, auditBefore int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM financial_events WHERE owner_id=$1`, owner.String()).Scan(&eventsBefore)
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM financial_event_revisions WHERE owner_id=$1`, owner.String()).Scan(&auditBefore)
		if _, e := fixture.Exec(ctx, `UPDATE finance_history_revisions SET revision=$2 WHERE owner_id=$1`, owner.String(), int64(math.MaxInt64)); e != nil {
			t.Fatal(e)
		}
		key := newFinancialWriteKey()
		_, e := service.PostExpense(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 1, Currency: "MYR", OccurredAt: at}, key)
		if !errors.Is(e, ErrHistoryRevisionExhausted) {
			t.Fatal(e)
		}
		var after, auditAfter, keys int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM financial_events WHERE owner_id=$1`, owner.String()).Scan(&after)
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM financial_event_revisions WHERE owner_id=$1`, owner.String()).Scan(&auditAfter)
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM finance_write_requests WHERE owner_id=$1 AND request_key=$2`, owner.String(), key).Scan(&keys)
		if after != eventsBefore || auditAfter != auditBefore || keys != 0 {
			t.Fatal("partial rollback")
		}
		if _, e = service.Transfer(ctx, owner, transferInput, transferKey); !errors.Is(e, ErrAlreadyApplied) || revision() != math.MaxInt64 {
			t.Fatal("retained replay blocked at exhausted counter", e)
		}
	})
	if e := runner.Steps(-1); e == nil {
		t.Fatal("active metadata down allowed")
	}
	fmt.Println("History integration: complete-account balances, v10 upgrade/fence, stale cursors, exact overflow and rollback verified")
}

type historyTraceKey struct{}
type historySnapshotTracer struct {
	once             sync.Once
	reached, release chan struct{}
}

func (t *historySnapshotTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, historyTraceKey{}, strings.Contains(data.SQL, "SELECT epoch::text,revision"))
}
func (t *historySnapshotTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if yes, _ := ctx.Value(historyTraceKey{}).(bool); yes {
		t.once.Do(func() { close(t.reached); <-t.release })
	}
}
