package finance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/Faust217/LifeOS/internal/platform/config"
	"github.com/Faust217/LifeOS/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

func TestOpeningBalanceIntegration(t *testing.T) {
	pool, runner := financeTestDatabase(t, "LIFEOS_OPENING_TEST_DATABASE_URL")
	ctx := context.Background()
	if err := runner.Migrate(7); err != nil {
		t.Fatal(err)
	}
	owner := verifiedTestOwner(t, pool, "11111111-1111-4111-8111-111111111111", "anchor-owner")
	other := verifiedTestOwner(t, pool, "22222222-2222-4222-8222-222222222222", "anchor-other")
	if _, err := pool.Exec(ctx, `UPDATE lifeos_users SET time_zone='Asia/Kuala_Lumpur' WHERE id IN($1,$2)`, owner.String(), other.String()); err != nil {
		t.Fatal(err)
	}
	service := NewService(pool)
	legacy := func(name string, opening int64) Account {
		a, err := legacyTestAccount(ctx, service, owner, AccountInput{Name: name, Type: "bank", Currency: "MYR", OpeningBalanceMinor: opening})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	bank := legacy("Old bank", 1000)
	seed := func(account Account, kind string, amount int64, when time.Time) string {
		id, _ := newUUID()
		movement, _ := newUUID()
		signed := amount
		if kind == "expense" {
			signed = -amount
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		_, err = tx.Exec(ctx, `INSERT INTO financial_events(id,owner_id,kind,amount_minor,currency,occurred_at) VALUES($1,$2,$3,$4,'MYR',$5)`, id, owner.String(), kind, amount, when)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO account_movements(id,owner_id,event_id,account_id,amount_minor,currency) VALUES($1,$2,$3,$4,$5,'MYR')`, movement, owner.String(), id, account.ID, signed)
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return id
	}
	seed(bank, "expense", 100, at.Add(-time.Microsecond))
	seed(bank, "expense", 50, at)
	var old int64
	if err := pool.QueryRow(ctx, `SELECT a.opening_balance_minor+sum(m.amount_minor) FROM finance_accounts a JOIN account_movements m ON m.account_id=a.id WHERE a.id=$1 GROUP BY a.id`, bank.ID).Scan(&old); err != nil || old != 850 {
		t.Fatal(old, err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	balance := func(a Account, want int64) {
		t.Helper()
		b, e := service.Balance(ctx, owner, a.ID)
		if e != nil || b.AmountMinor != want {
			t.Fatalf("%s balance=%d want=%d err=%v", a.Name, b.AmountMinor, want, e)
		}
	}
	t.Run("v7 retained and preview complete", func(t *testing.T) {
		accounts, err := service.ListAccounts(ctx, owner)
		if err != nil || accounts[0].OpeningBalanceAt != nil || accounts[0].OpeningBalanceMinor != 1000 {
			t.Fatal(accounts, err)
		}
		balance(bank, 850)
		p, err := service.PreviewOpeningBalance(ctx, owner, bank.ID, at, nil)
		if err != nil || p.SuggestedOpeningMinor != "900" || *p.BalanceAfterMinor != "850" || *p.BalanceChangeMinor != "0" || p.BeforeCount != 1 || p.OnOrAfterCount != 1 {
			t.Fatal(p, err)
		}
		chosen := int64(1000)
		p, err = service.PreviewOpeningBalance(ctx, owner, bank.ID, at, &chosen)
		if err != nil || *p.BalanceAfterMinor != "950" || *p.BalanceChangeMinor != "100" {
			t.Fatal(p, err)
		}
		if _, err = service.ConfirmOpeningBalance(ctx, owner, bank.ID, at, chosen, 0, false, newFinancialWriteKey()); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("unacknowledged change", err)
		}
		balance(bank, 850)
	})
	t.Run("stale preview and one time preservation", func(t *testing.T) {
		if _, err := service.PostExpense(ctx, owner, EventInput{AccountID: bank.ID, AmountMinor: 1, Currency: "MYR", OccurredAt: at.Add(-2 * time.Microsecond)}, newFinancialWriteKey()); err != nil {
			t.Fatal(err)
		}
		if _, err := service.ConfirmOpeningBalance(ctx, owner, bank.ID, at, 900, 0, false, newFinancialWriteKey()); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
		p, err := service.PreviewOpeningBalance(ctx, owner, bank.ID, at, nil)
		if err != nil {
			t.Fatal(err)
		}
		revision, _ := strconv.ParseInt(p.BalanceRevision, 10, 64)
		if _, err = service.ConfirmOpeningBalance(ctx, owner, bank.ID, at, 899, revision, false, newFinancialWriteKey()); err != nil {
			t.Fatal(err)
		}
		balance(bank, 849)
		if _, err = service.ConfirmOpeningBalance(ctx, owner, bank.ID, at, 899, revision, false, newFinancialWriteKey()); !errors.Is(err, ErrConflict) {
			t.Fatal("repeat", err)
		}
		var count int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM finance_opening_confirmations WHERE account_id=$1`, bank.ID).Scan(&count); err != nil || count != 1 {
			t.Fatal(count, err)
		}
	})
	makeAccount := func(name, currency string, opening int64, when time.Time) Account {
		t.Helper()
		a, e := service.CreateAccount(ctx, owner, AccountInput{Name: name, Type: "bank", Currency: currency, OpeningBalanceMinor: opening, OpeningBalanceAt: &when}, newFinancialWriteKey())
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	t.Run("new account validation and exact instant", func(t *testing.T) {
		if _, err := service.CreateAccount(ctx, owner, AccountInput{Name: "Missing date", Type: "bank", Currency: "MYR"}, newFinancialWriteKey()); !errors.Is(err, ErrInvalidInput) {
			t.Fatal(err)
		}
		future := time.Now().Add(time.Hour)
		if _, err := service.CreateAccount(ctx, owner, AccountInput{Name: "Future", Type: "bank", Currency: "MYR", OpeningBalanceAt: &future}, newFinancialWriteKey()); !errors.Is(err, ErrInvalidInput) {
			t.Fatal(err)
		}
		a := makeAccount("Exact boundary", "MYR", 1000, at)
		for _, when := range []time.Time{at.Add(-time.Microsecond), at, at.Add(time.Microsecond)} {
			if _, e := service.PostExpense(ctx, owner, EventInput{AccountID: a.ID, AmountMinor: 10, Currency: "MYR", OccurredAt: when}, newFinancialWriteKey()); e != nil {
				t.Fatal(e)
			}
		}
		balance(a, 980)
		// The application truncates sub-microsecond input before both validation and PostgreSQL storage.
		e, err := service.PostExpense(ctx, owner, EventInput{AccountID: a.ID, AmountMinor: 10, Currency: "MYR", OccurredAt: at.Add(-time.Nanosecond)}, newFinancialWriteKey())
		if err != nil || !e.OccurredAt.Equal(at.Add(-time.Microsecond)) {
			t.Fatal("inconsistent timestamp precision", e, err)
		}
		balance(a, 980)
		if _, e := service.PostIncome(ctx, owner, EventInput{AccountID: a.ID, AmountMinor: 1, Currency: "MYR", OccurredAt: time.Now().Add(24 * time.Hour)}, newFinancialWriteKey()); e != nil {
			t.Fatal(e)
		}
		balance(a, 981)
	})
	t.Run("mixed transfer and pre anchor void delete", func(t *testing.T) {
		a := makeAccount("Dated", "MYR", 1000, at)
		b := legacy("Undated", 200)
		e, err := service.Transfer(ctx, owner, TransferInput{FromAccountID: a.ID, ToAccountID: b.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: at.Add(-time.Microsecond)}, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		balance(a, 1000)
		balance(b, 300)
		if err = service.VoidEvent(ctx, owner, e.ID, "", newFinancialWriteKey()); err != nil {
			t.Fatal(err)
		}
		balance(a, 1000)
		balance(b, 200)
		if err = service.DeleteEvent(ctx, owner, e.ID, newFinancialWriteKey()); err != nil {
			t.Fatal(err)
		}
		if err = service.DeleteEvent(ctx, owner, e.ID, newFinancialWriteKey()); err != nil {
			t.Fatal(err)
		}
		balance(a, 1000)
		balance(b, 200)
		e, err = service.Transfer(ctx, owner, TransferInput{FromAccountID: a.ID, ToAccountID: b.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: at}, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		balance(a, 900)
		balance(b, 300)
		if err = service.DeleteEvent(ctx, owner, e.ID, newFinancialWriteKey()); err != nil {
			t.Fatal(err)
		}
		balance(a, 1000)
		balance(b, 200)
		c := makeAccount("Later date", "MYR", 400, at.Add(time.Hour))
		e, err = service.Transfer(ctx, owner, TransferInput{FromAccountID: a.ID, ToAccountID: c.ID, AmountMinor: 50, Currency: "MYR", OccurredAt: at}, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		balance(a, 950)
		balance(c, 400)
		history, err := service.History(ctx, owner, 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range history {
			if row.ID == e.ID {
				found = true
				if len(row.Movements) != 2 {
					t.Fatal("transfer split")
				}
			}
		}
		if !found {
			t.Fatal("missing history")
		}
	})
	t.Run("pre date spending and exact allocations stay meaningful", func(t *testing.T) {
		a := makeAccount("Spending history", "MYR", 1000, at)
		c1, err := service.CreateCategory(ctx, owner, CategoryInput{Name: "Before date spending", Kind: "expense"})
		if err != nil {
			t.Fatal(err)
		}
		c2, err := service.CreateCategory(ctx, owner, CategoryInput{Name: "Second allocation", Kind: "expense"})
		if err != nil {
			t.Fatal(err)
		}
		e, err := service.PostExpense(ctx, owner, EventInput{AccountID: a.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: at.Add(-time.Second), Allocations: []AllocationInput{{CategoryID: c1.ID, AmountMinor: 70}, {CategoryID: c2.ID, AmountMinor: 30}}}, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		balance(a, 1000)
		spent, err := service.CategorySpending(ctx, owner, c1.ID, "MYR")
		if err != nil || spent != 70 {
			t.Fatal(spent, err)
		}
		if err = service.VoidEvent(ctx, owner, e.ID, "", newFinancialWriteKey()); err != nil {
			t.Fatal(err)
		}
		balance(a, 1000)
		spent, err = service.CategorySpending(ctx, owner, c1.ID, "MYR")
		if err != nil || spent != 0 {
			t.Fatal(spent, err)
		}
		e, err = service.PostExpense(ctx, owner, EventInput{AccountID: a.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: at, Allocations: []AllocationInput{{CategoryID: c1.ID, AmountMinor: 100}}}, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		balance(a, 900)
		if err = service.DeleteEvent(ctx, owner, e.ID, newFinancialWriteKey()); err != nil {
			t.Fatal(err)
		}
		balance(a, 1000)
		spent, err = service.CategorySpending(ctx, owner, c1.ID, "MYR")
		if err != nil || spent != 0 {
			t.Fatal(spent, err)
		}
	})
	t.Run("wide overflow and rollback", func(t *testing.T) {
		a := legacy("Wide suggestion", math.MaxInt64)
		seed(a, "income", 1, at.Add(-time.Second))
		seed(a, "expense", 1, at)
		p, err := service.PreviewOpeningBalance(ctx, owner, a.ID, at, nil)
		if err != nil || p.SuggestedRepresentable || p.SuggestedOpeningMinor != "9223372036854775808" || p.ChosenOpeningMinor != nil {
			t.Fatal(p, err)
		}
		if _, err = service.ConfirmOpeningBalance(ctx, owner, a.ID, at, math.MaxInt64, 0, true, newFinancialWriteKey()); err != nil {
			t.Fatal(err)
		}
		balance(a, math.MaxInt64-1)
		max := makeAccount("Maximum", "MYR", math.MaxInt64, at)
		if _, err = service.PostIncome(ctx, owner, EventInput{AccountID: max.ID, AmountMinor: 1, Currency: "MYR", OccurredAt: at.Add(-time.Microsecond)}, newFinancialWriteKey()); err != nil {
			t.Fatal("pre anchor overflow false positive", err)
		}
		balance(max, math.MaxInt64)
		if _, err = service.PostIncome(ctx, owner, EventInput{AccountID: max.ID, AmountMinor: 1, Currency: "MYR", OccurredAt: at}, newFinancialWriteKey()); !errors.Is(err, ErrOverflow) {
			t.Fatal(err)
		}
		balance(max, math.MaxInt64)
		min := makeAccount("Minimum", "MYR", math.MinInt64, at)
		if _, err = service.PostExpense(ctx, owner, EventInput{AccountID: min.ID, AmountMinor: 1, Currency: "MYR", OccurredAt: at}, newFinancialWriteKey()); !errors.Is(err, ErrOverflow) {
			t.Fatal(err)
		}
		balance(min, math.MinInt64)
	})
	t.Run("confirmation evidence failure rolls back", func(t *testing.T) {
		a := legacy("Rollback", 50)
		_, err := pool.Exec(ctx, `CREATE FUNCTION opening_test_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic failure'; END $$; CREATE TRIGGER opening_test_failure BEFORE INSERT ON finance_opening_confirmations FOR EACH ROW EXECUTE FUNCTION opening_test_fail()`)
		if err != nil {
			t.Fatal(err)
		}
		_, err = service.ConfirmOpeningBalance(ctx, owner, a.ID, at, 50, 0, false, newFinancialWriteKey())
		if err == nil {
			t.Fatal("injected failure succeeded")
		}
		_, err = pool.Exec(ctx, `DROP TRIGGER opening_test_failure ON finance_opening_confirmations; DROP FUNCTION opening_test_fail()`)
		if err != nil {
			t.Fatal(err)
		}
		var date *time.Time
		var revision int64
		if err = pool.QueryRow(ctx, `SELECT opening_balance_at,balance_revision FROM finance_accounts WHERE id=$1`, a.ID).Scan(&date, &revision); err != nil || date != nil || revision != 0 {
			t.Fatal(date, revision, err)
		}
		balance(a, 50)
	})
	t.Run("concurrent post versus confirmation", func(t *testing.T) {
		a := legacy("Concurrent", 1000)
		var wg sync.WaitGroup
		var confirmErr, postErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, confirmErr = service.ConfirmOpeningBalance(ctx, owner, a.ID, at, 1000, 0, false, newFinancialWriteKey())
		}()
		go func() {
			defer wg.Done()
			_, postErr = service.PostExpense(ctx, owner, EventInput{AccountID: a.ID, AmountMinor: 10, Currency: "MYR", OccurredAt: at.Add(-time.Second)}, newFinancialWriteKey())
		}()
		wg.Wait()
		if postErr != nil || confirmErr != nil && !errors.Is(confirmErr, ErrConflict) {
			t.Fatal(confirmErr, postErr)
		}
		if confirmErr == nil {
			balance(a, 1000)
		} else {
			balance(a, 990)
		}
	})
	t.Run("concurrent void delete versus confirmation", func(t *testing.T) {
		for _, remove := range []string{"void", "delete"} {
			for _, pre := range []bool{true, false} {
				a := legacy(fmt.Sprintf("Race %s %v", remove, pre), 1000)
				when := at
				if pre {
					when = at.Add(-time.Second)
				}
				e, err := service.PostExpense(ctx, owner, EventInput{AccountID: a.ID, AmountMinor: 10, Currency: "MYR", OccurredAt: when}, newFinancialWriteKey())
				if err != nil {
					t.Fatal(err)
				}
				p, err := service.PreviewOpeningBalance(ctx, owner, a.ID, at, nil)
				if err != nil {
					t.Fatal(err)
				}
				amount, _ := strconv.ParseInt(*p.ChosenOpeningMinor, 10, 64)
				revision, _ := strconv.ParseInt(p.BalanceRevision, 10, 64)
				var wg sync.WaitGroup
				var confirmErr, removeErr error
				wg.Add(2)
				go func() {
					defer wg.Done()
					_, confirmErr = service.ConfirmOpeningBalance(ctx, owner, a.ID, at, amount, revision, false, newFinancialWriteKey())
				}()
				go func() {
					defer wg.Done()
					if remove == "void" {
						removeErr = service.VoidEvent(ctx, owner, e.ID, "", newFinancialWriteKey())
					} else {
						removeErr = service.DeleteEvent(ctx, owner, e.ID, newFinancialWriteKey())
					}
				}()
				wg.Wait()
				if removeErr != nil || confirmErr != nil && !errors.Is(confirmErr, ErrConflict) {
					t.Fatal(remove, pre, confirmErr, removeErr)
				}
				want := int64(1000)
				if confirmErr == nil && pre {
					want = 990
				}
				balance(a, want)
			}
		}
	})
	t.Run("guarded real API preview and confirm", func(t *testing.T) {
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": "http://" + r.Host, "authorization_endpoint": "http://" + r.Host + "/authorize", "token_endpoint": "http://" + r.Host + "/token", "jwks_uri": "http://" + r.Host + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
		}))
		defer provider.Close()
		auth, err := identity.NewAuth(ctx, pool, config.AuthConfig{ClientID: "anchor-test", ClientSecret: "synthetic", Issuer: provider.URL, PublicOrigin: moneyTestOrigin})
		if err != nil {
			t.Fatal(err)
		}
		api := chi.NewRouter()
		auth.RegisterRoutes(api)
		RegisterRoutes(api, auth, service)
		server := httptest.NewServer(httpapi.NewRouter(slog.New(slog.NewJSONHandler(io.Discard, nil)), moneyTestReadiness{}, api))
		defer server.Close()
		request := func(path, body, secret, origin, csrf string) (int, []byte) {
			t.Helper()
			method := "POST"
			if path == "/me" {
				method = "GET"
			}
			req, _ := http.NewRequest(method, server.URL+"/api/v1"+path, strings.NewReader(body))
			if secret != "" {
				req.AddCookie(&http.Cookie{Name: "lifeos_session", Value: secret})
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", origin)
			req.Header.Set("X-CSRF-Token", csrf)
			req.Header.Set("Idempotency-Key", newFinancialWriteKey())
			res, e := server.Client().Do(req)
			if e != nil {
				t.Fatal(e)
			}
			defer res.Body.Close()
			b, e := io.ReadAll(res.Body)
			if e != nil {
				t.Fatal(e)
			}
			return res.StatusCode, b
		}
		_, body := request("/me", "", "anchor-owner", moneyTestOrigin, "")
		var me struct {
			CSRF string `json:"csrf_token"`
		}
		if json.Unmarshal(body, &me) != nil || me.CSRF == "" {
			t.Fatal("csrf")
		}
		a := legacy("API conversion", 1000)
		path := "/accounts/" + a.ID + "/opening-balance/preview"
		input := `{"opening_balance_at":"2026-10-01T00:00:00Z"}`
		for _, tc := range []struct {
			secret, origin, csrf string
			status               int
		}{{"", moneyTestOrigin, "", 401}, {"anchor-owner", "https://evil.invalid", me.CSRF, 403}, {"anchor-owner", moneyTestOrigin, "bad", 403}, {"anchor-owner", moneyTestOrigin, me.CSRF, 200}} {
			status, b := request(path, input, tc.secret, tc.origin, tc.csrf)
			if status != tc.status {
				t.Fatalf("status=%d want=%d body=%s", status, tc.status, b)
			}
		}
		if status, _ := request(path, `{"opening_balance_at":"2026-10-01T00:00:00Z","owner_id":"`+other.String()+`"}`, "anchor-owner", moneyTestOrigin, me.CSRF); status != 400 {
			t.Fatal(status)
		}
		for _, amount := range []string{"null", `""`, "123"} {
			status, _ := request(path, `{"opening_balance_at":"2026-10-01T00:00:00Z","opening_balance_minor":`+amount+`}`, "anchor-owner", moneyTestOrigin, me.CSRF)
			if status != 400 {
				t.Fatal("invalid optional amount", amount, status)
			}
		}
		if _, err = service.PreviewOpeningBalance(ctx, other, a.ID, at, nil); !errors.Is(err, ErrNotFound) {
			t.Fatal("foreign owner", err)
		}
		confirmPath := "/accounts/" + a.ID + "/opening-balance/confirm"
		confirmInput := `{"opening_balance_at":"2026-10-01T00:00:00Z","opening_balance_minor":"1000","balance_revision":"0","acknowledge_change":false}`
		for _, tc := range []struct {
			secret, origin, csrf string
			status               int
		}{{"", moneyTestOrigin, "", 401}, {"anchor-owner", "https://evil.invalid", me.CSRF, 403}, {"anchor-owner", moneyTestOrigin, "bad", 403}} {
			status, b := request(confirmPath, confirmInput, tc.secret, tc.origin, tc.csrf)
			if status != tc.status {
				t.Fatal(status, tc.status, string(b))
			}
		}
		if status, _ := request(confirmPath, strings.ReplaceAll(confirmInput, `,"acknowledge_change":false`, ""), "anchor-owner", moneyTestOrigin, me.CSRF); status != 400 {
			t.Fatal("missing acknowledgement field", status)
		}
		if _, err = service.ConfirmOpeningBalance(ctx, other, a.ID, at, 1000, 0, false, newFinancialWriteKey()); !errors.Is(err, ErrNotFound) {
			t.Fatal("foreign confirmation", err)
		}
		if status, _ := request("/accounts/"+a.ID+"/opening-balance/confirm", `{"opening_balance_at":"2026-10-01T00:00:00Z","opening_balance_minor":"1000","balance_revision":"0","acknowledge_change":false}`, "anchor-owner", moneyTestOrigin, me.CSRF); status != 200 {
			t.Fatal(status)
		}
		if status, _ := request("/accounts", `{"name":"New API","type":"bank","currency":"MYR","opening_balance_minor":"0","opening_balance_at":"2026-10-01T00:00:00Z"}`, "anchor-owner", moneyTestOrigin, me.CSRF); status != 201 {
			t.Fatal(status)
		}
		if status, _ := request("/accounts", `{"name":"Old client","type":"bank","currency":"MYR","opening_balance_minor":"0"}`, "anchor-owner", moneyTestOrigin, me.CSRF); status != 400 {
			t.Fatal(status)
		}
	})
	t.Run("down migration refuses confirmed data", func(t *testing.T) {
		sql, err := os.ReadFile("../../db/migrations/000008_account_opening_balance_instant.down.sql")
		if err != nil {
			t.Fatal(err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		script := strings.ReplaceAll(strings.ReplaceAll(string(sql), "BEGIN;", ""), "COMMIT;", "")
		if _, err = tx.Exec(ctx, script); err == nil {
			t.Fatal("unsafe down succeeded")
		}
	})
	t.Log(fmt.Sprintf("populated v7 to v8; anchors, guards, wide arithmetic, concurrency and rollback verified"))
}
