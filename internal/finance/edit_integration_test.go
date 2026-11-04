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
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAtomicEditIntegration(t *testing.T) {
	pool, runner := financeTestDatabase(t, "LIFEOS_EDIT_TEST_DATABASE_URL")
	ctx := context.Background()
	if err := runner.Migrate(9); err != nil {
		t.Fatal(err)
	}
	const uid = "35353535-3535-4535-8535-353535353535"
	owner := verifiedTestOwner(t, pool, uid, "edit-owner-session")
	other := verifiedTestOwner(t, pool, "36363636-3636-4636-8636-363636363636", "edit-other-session")
	_, _ = pool.Exec(ctx, `UPDATE lifeos_users SET time_zone='Asia/Kuala_Lumpur'`)
	service := NewService(pool)
	legacy, err := legacyTestAccount(ctx, service, owner, AccountInput{Name: "Legacy", Type: "cash", Currency: "MYR", OpeningBalanceMinor: 10000})
	if err != nil {
		t.Fatal(err)
	}
	// A truthful old-schema fixture, not a v11 service writing to v9.
	category := Category{ID: newFinancialWriteKey(), Name: "Archived baseline", Kind: "expense"}
	_, err = pool.Exec(ctx, `INSERT INTO finance_categories(id,owner_id,name,kind,archived_at) VALUES($1,$2,$3,$4,now())`, category.ID, owner.String(), category.Name, category.Kind)
	if err != nil {
		t.Fatal(err)
	}
	baselineIDs := []string{newFinancialWriteKey(), newFinancialWriteKey(), newFinancialWriteKey()}
	for i, id := range baselineIDs {
		_, err = pool.Exec(ctx, `INSERT INTO financial_events(id,owner_id,kind,amount_minor,currency,occurred_at,note,status,voided_at,void_reason,deleted_at)
          VALUES($1,$2,'expense',100,'MYR','2026-10-01T00:00:00.123456Z','truthful v9 baseline',CASE WHEN $3=1 THEN 'voided' ELSE 'posted' END,CASE WHEN $3=1 THEN now() END,CASE WHEN $3=1 THEN 'old audit text' END,CASE WHEN $3=2 THEN now() END)`, id, uid, i)
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO account_movements(id,owner_id,event_id,account_id,amount_minor,currency) VALUES($1,$2,$3,$4,-100,'MYR')`, newFinancialWriteKey(), uid, id, legacy.ID)
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, id := range baselineIDs {
		_, err = pool.Exec(ctx, `INSERT INTO category_allocations(id,owner_id,event_id,category_id,kind,amount_minor,currency) VALUES($1,$2,$3,$4,'expense',100,'MYR')`, newFinancialWriteKey(), uid, id, category.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	retainedKey := newFinancialWriteKey()
	_, digest, _ := canonicalFinancialCommand(owner, financialCommand{operation: "event.void", target: baselineIDs[1], parameters: []any{"explicit", "old audit text"}})
	_, err = pool.Exec(ctx, `INSERT INTO finance_write_requests(owner_id,request_key,operation,fingerprint_version,request_fingerprint) VALUES($1,$2,'event.void',1,$3)`, uid, retainedKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	if err = runner.Up(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM financial_event_revisions WHERE action='legacy_baseline' AND request_key IS NULL AND snapshot->'event'->>'note'='truthful v9 baseline'`).Scan(&count); err != nil || count != 3 {
		t.Fatal("baseline backfill", count, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM financial_event_revisions WHERE jsonb_array_length(snapshot->'movements')=1 AND jsonb_array_length(snapshot->'allocations')=1 AND snapshot->'event'->>'occurred_at'='2026-10-01T00:00:00.123456Z'`).Scan(&count); err != nil || count != 3 {
		t.Fatal("baseline incomplete", count, err)
	}
	if err = service.VoidEvent(ctx, owner, baselineIDs[1], "old audit text", retainedKey); !errors.Is(err, ErrAlreadyApplied) {
		t.Fatal("retained v9 key", err)
	}
	if err = service.VoidEvent(ctx, owner, baselineIDs[0], "", newFinancialWriteKey()); !errors.Is(err, ErrRevisionRequired) {
		t.Fatal("new v9-shaped request executed", err)
	}
	for _, sql := range []string{`UPDATE financial_event_revisions SET action='edit'`, `DELETE FROM financial_event_revisions`, `TRUNCATE financial_event_revisions`} {
		if _, err = pool.Exec(ctx, sql); err == nil {
			t.Fatal("mutable audit", sql)
		}
	}
	event, err := service.GetEvent(ctx, owner, baselineIDs[0])
	if err != nil || event.Revision != 1 || event.OccurredAt.Nanosecond() != 123456000 {
		t.Fatal(event, err)
	}
	if _, err = service.GetEvent(ctx, owner, baselineIDs[2]); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted visible", err)
	}
	if _, err = service.GetEvent(ctx, other, event.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign visible", err)
	}
	input := EditInput{Kind: "expense", EventInput: EventInput{AccountID: legacy.ID, AmountMinor: 120, Currency: "MYR", OccurredAt: event.OccurredAt, Note: "corrected", Allocations: []AllocationInput{}}}
	key := newFinancialWriteKey()
	edited, err := service.EditEvent(ctx, owner, event.ID, 1, input, key)
	if err != nil || edited.ID != event.ID || edited.Revision != 2 || edited.Movements[0].ID != event.Movements[0].ID {
		t.Fatal("edit", edited, err)
	}
	balance, err := service.Balance(ctx, owner, legacy.ID)
	if err != nil || balance.AmountMinor != 9880 {
		t.Fatal("balance", balance, err)
	}
	if _, err = NewService(pool).EditEvent(ctx, owner, event.ID, 1, input, key); !errors.Is(err, ErrAlreadyApplied) {
		t.Fatal("restart replay", err)
	}
	input.Note = "changed key intent"
	if _, err = service.EditEvent(ctx, owner, event.ID, 1, input, key); !errors.Is(err, ErrKeyConflict) {
		t.Fatal("key conflict", err)
	}
	if _, err = service.EditEvent(ctx, owner, event.ID, 1, input, newFinancialWriteKey()); !errors.Is(err, ErrRevisionConflict) {
		t.Fatal("stale", err)
	}

	t.Run("complete splits and archived associations", func(t *testing.T) {
		food, _ := service.CreateCategory(ctx, owner, CategoryInput{Name: "Food", Kind: "expense"})
		medical, _ := service.CreateCategory(ctx, owner, CategoryInput{Name: "Medical", Kind: "expense"})
		e, err := service.PostExpense(ctx, owner, EventInput{AccountID: legacy.ID, AmountMinor: 3000, Currency: "MYR", OccurredAt: time.Now(), Allocations: []AllocationInput{{food.ID, 2000}, {medical.ID, 1000}}}, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		if err = service.ArchiveAccount(ctx, owner, legacy.ID); err != nil {
			t.Fatal(err)
		}
		if err = service.ArchiveCategory(ctx, owner, medical.ID); err != nil {
			t.Fatal(err)
		}
		in := EditInput{Kind: "expense", EventInput: EventInput{AccountID: legacy.ID, AmountMinor: 3500, Currency: "MYR", OccurredAt: e.OccurredAt, Allocations: []AllocationInput{{food.ID, 2000}, {medical.ID, 1500}}}}
		e2, err := service.EditEvent(ctx, owner, e.ID, 1, in, newFinancialWriteKey())
		if err != nil || len(e2.Allocations) != 2 {
			t.Fatal(err)
		}
		for _, a := range e.Allocations {
			found := false
			for _, b := range e2.Allocations {
				if a.ID == b.ID && a.CategoryID == b.CategoryID {
					found = true
				}
			}
			if !found {
				t.Fatal("allocation identity lost")
			}
		}
		spending, err := service.CategorySpending(ctx, owner, medical.ID, "MYR")
		if err != nil || spending != 1500 {
			t.Fatal(spending, err)
		}
		in.Allocations = nil
		e3, err := service.EditEvent(ctx, owner, e.ID, 2, in, newFinancialWriteKey())
		if err != nil || len(e3.Allocations) != 0 {
			t.Fatal("uncategorized", err)
		}
		in.Allocations = []AllocationInput{{medical.ID, 3500}}
		if _, err = service.EditEvent(ctx, owner, e.ID, 3, in, newFinancialWriteKey()); !errors.Is(err, ErrArchived) {
			t.Fatal("new archived association", err)
		}
		foreign, err := legacyTestAccount(ctx, service, other, AccountInput{Name: "Foreign", Type: "cash", Currency: "MYR"})
		if err != nil {
			t.Fatal(err)
		}
		in.AccountID = foreign.ID
		in.Allocations = nil
		if _, err = service.EditEvent(ctx, owner, e.ID, 3, in, newFinancialWriteKey()); !errors.Is(err, ErrNotFound) {
			t.Fatal("foreign account selected", err)
		}
		archived, err := legacyTestAccount(ctx, service, owner, AccountInput{Name: "Archived new selection", Type: "cash", Currency: "MYR"})
		if err != nil {
			t.Fatal(err)
		}
		if err = service.ArchiveAccount(ctx, owner, archived.ID); err != nil {
			t.Fatal(err)
		}
		in.AccountID = archived.ID
		if _, err = service.EditEvent(ctx, owner, e.ID, 3, in, newFinancialWriteKey()); !errors.Is(err, ErrArchived) {
			t.Fatal("new archived account selected", err)
		}
		in.AccountID = legacy.ID
		in.Allocations = []AllocationInput{{food.ID, 100}}
		if _, err = service.EditEvent(ctx, owner, e.ID, 3, in, newFinancialWriteKey()); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("partial split", err)
		}
		_ = service.RestoreAccount(ctx, owner, legacy.ID)
	})
	t.Run("transfer anchors and endpoint identity", func(t *testing.T) {
		at := time.Now().UTC().Add(-72 * time.Hour).Truncate(time.Microsecond)
		later := at.Add(48 * time.Hour)
		cash, err := service.CreateAccount(ctx, owner, AccountInput{Name: "Cash", Type: "cash", Currency: "MYR", OpeningBalanceMinor: 10000, OpeningBalanceAt: &at}, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		savings, err := service.CreateAccount(ctx, owner, AccountInput{Name: "Savings", Type: "bank", Currency: "MYR", OpeningBalanceMinor: 20000, OpeningBalanceAt: &later}, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		tr, err := service.Transfer(ctx, owner, TransferInput{FromAccountID: cash.ID, ToAccountID: savings.ID, AmountMinor: 5000, Currency: "MYR", OccurredAt: at.Add(24 * time.Hour)}, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		b, _ := service.Balance(ctx, owner, savings.ID)
		if b.AmountMinor != 20000 {
			t.Fatal(b)
		}
		in := EditInput{Kind: "transfer", FromAccountID: cash.ID, ToAccountID: savings.ID, EventInput: EventInput{AmountMinor: 5000, Currency: "MYR", OccurredAt: later}}
		tr2, err := service.EditEvent(ctx, owner, tr.ID, 1, in, newFinancialWriteKey())
		if err != nil || len(tr2.Movements) != 2 || len(tr2.Allocations) != 0 {
			t.Fatal(tr2, err)
		}
		b, _ = service.Balance(ctx, owner, savings.ID)
		if b.AmountMinor != 25000 {
			t.Fatal(b)
		}
		in.ToAccountID = legacy.ID
		tr3, err := service.EditEvent(ctx, owner, tr.ID, 2, in, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		b, _ = service.Balance(ctx, owner, savings.ID)
		if b.AmountMinor != 20000 {
			t.Fatal(b)
		}
		if tr3.ID != tr.ID {
			t.Fatal("transfer event duplicated")
		}
	})
	t.Run("income account change followed by Void and Delete", func(t *testing.T) {
		first, err := legacyTestAccount(ctx, service, owner, AccountInput{Name: "Income source", Type: "cash", Currency: "MYR"})
		if err != nil {
			t.Fatal(err)
		}
		next, err := legacyTestAccount(ctx, service, owner, AccountInput{Name: "Income destination", Type: "cash", Currency: "MYR", OpeningBalanceMinor: -500})
		if err != nil {
			t.Fatal(err)
		}
		e, err := service.PostIncome(ctx, owner, EventInput{AccountID: first.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: time.Now()}, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		in := EditInput{Kind: "income", EventInput: EventInput{AccountID: next.ID, AmountMinor: 250, Currency: "MYR", OccurredAt: e.OccurredAt, Note: "Correct account"}}
		updated, err := service.EditEvent(ctx, owner, e.ID, 1, in, newFinancialWriteKey())
		if err != nil || updated.Movements[0].ID == e.Movements[0].ID {
			t.Fatal("new relationship reused old ID", err)
		}
		oldBalance, _ := service.Balance(ctx, owner, first.ID)
		newBalance, _ := service.Balance(ctx, owner, next.ID)
		if oldBalance.AmountMinor != 0 || newBalance.AmountMinor != -250 {
			t.Fatal("income replacement effects", oldBalance, newBalance)
		}
		if err = service.VoidEvent(ctx, owner, e.ID, "", newFinancialWriteKey(), 2); err != nil {
			t.Fatal(err)
		}
		newBalance, _ = service.Balance(ctx, owner, next.ID)
		if newBalance.AmountMinor != -500 {
			t.Fatal("Void used old movement", newBalance)
		}
		if err = service.DeleteEvent(ctx, owner, e.ID, newFinancialWriteKey(), 3); err != nil {
			t.Fatal(err)
		}
		newBalance, _ = service.Balance(ctx, owner, next.ID)
		if newBalance.AmountMinor != -500 {
			t.Fatal("Delete counted voided movement twice")
		}
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM financial_event_revisions WHERE owner_id=$1 AND event_id=$2`, owner.String(), e.ID).Scan(&count); err != nil || count != 4 {
			t.Fatal("incomplete status audit", count, err)
		}
	})
	t.Run("wide arithmetic and rollback", func(t *testing.T) {
		a, err := legacyTestAccount(ctx, service, owner, AccountInput{Name: "Int64", Type: "cash", Currency: "MYR", OpeningBalanceMinor: math.MaxInt64})
		if err != nil {
			t.Fatal(err)
		}
		e, err := service.PostExpense(ctx, owner, EventInput{AccountID: a.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: time.Now()}, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		_, err = service.PostIncome(ctx, owner, EventInput{AccountID: a.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: time.Now()}, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		in := EditInput{Kind: "expense", EventInput: EventInput{AccountID: a.ID, AmountMinor: 200, Currency: "MYR", OccurredAt: e.OccurredAt}}
		e2, err := service.EditEvent(ctx, owner, e.ID, 1, in, newFinancialWriteKey())
		if err != nil {
			t.Fatal("valid net rejected", err)
		}
		in.Kind = "income"
		if _, err = service.EditEvent(ctx, owner, e.ID, 2, in, newFinancialWriteKey()); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("kind changed", err)
		}
		in.Kind = "expense"
		in.AmountMinor = 1
		k := newFinancialWriteKey()
		if _, err = service.EditEvent(ctx, owner, e.ID, 2, in, k); !errors.Is(err, ErrOverflow) {
			t.Fatal("overflow allowed", err)
		}
		read, _ := service.GetEvent(ctx, owner, e.ID)
		if read.Revision != e2.Revision || read.AmountMinor != 200 {
			t.Fatal("partial rollback")
		}
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM finance_write_requests WHERE owner_id=$1 AND request_key=$2`, uid, k).Scan(&count); err != nil || count != 0 {
			t.Fatal("failed marker retained", err)
		}
		_, err = pool.Exec(ctx, `CREATE FUNCTION fail_i35_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic failure'; END $$; CREATE TRIGGER fail_i35 BEFORE INSERT ON financial_event_revisions FOR EACH ROW EXECUTE FUNCTION fail_i35_snapshot()`)
		if err != nil {
			t.Fatal(err)
		}
		in.AmountMinor = 300
		k = newFinancialWriteKey()
		if _, err = service.EditEvent(ctx, owner, e.ID, 2, in, k); err == nil {
			t.Fatal("injected failure missing")
		}
		_, _ = pool.Exec(ctx, `DROP TRIGGER fail_i35 ON financial_event_revisions; DROP FUNCTION fail_i35_snapshot()`)
		read, _ = service.GetEvent(ctx, owner, e.ID)
		if read.AmountMinor != 200 || read.Revision != 2 {
			t.Fatal("audit failure did not rollback ledger")
		}
	})
	for _, op := range []string{"edit", "void", "delete"} {
		t.Run("concurrent edit/"+op, func(t *testing.T) {
			e, err := service.PostExpense(ctx, owner, EventInput{AccountID: legacy.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: time.Now()}, newFinancialWriteKey())
			if err != nil {
				t.Fatal(err)
			}
			in := EditInput{Kind: "expense", EventInput: EventInput{AccountID: legacy.ID, AmountMinor: 200, Currency: "MYR", OccurredAt: e.OccurredAt}}
			start := make(chan struct{})
			results := make(chan error, 2)
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				_, err := service.EditEvent(ctx, owner, e.ID, 1, in, newFinancialWriteKey())
				results <- err
			}()
			go func() {
				defer wg.Done()
				<-start
				var err error
				switch op {
				case "edit":
					_, err = service.EditEvent(ctx, owner, e.ID, 1, in, newFinancialWriteKey())
				case "void":
					err = service.VoidEvent(ctx, owner, e.ID, "", newFinancialWriteKey(), 1)
				case "delete":
					err = service.DeleteEvent(ctx, owner, e.ID, newFinancialWriteKey(), 1)
				}
				results <- err
			}()
			close(start)
			wg.Wait()
			close(results)
			success := 0
			for err := range results {
				if err == nil {
					success++
				} else if !errors.Is(err, ErrRevisionConflict) && !errors.Is(err, ErrNotFound) {
					t.Fatal(err)
				}
			}
			if success != 1 {
				t.Fatal("race successes", success)
			}
		})
	}

	t.Run("history header and children share a snapshot", func(t *testing.T) {
		e, err := service.PostExpense(ctx, owner, EventInput{AccountID: legacy.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: time.Now()}, newFinancialWriteKey())
		if err != nil {
			t.Fatal(err)
		}
		trace := &editSnapshotTracer{reached: make(chan struct{}), release: make(chan struct{})}
		cfg := pool.Config()
		cfg.ConnConfig.Tracer = trace
		reader, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		result := make(chan []Event, 1)
		failure := make(chan error, 1)
		go func() {
			events, err := NewService(reader).History(ctx, owner, 100, 0)
			if err != nil {
				failure <- err
			} else {
				result <- events
			}
		}()
		select {
		case <-trace.reached:
		case <-time.After(3 * time.Second):
			t.Fatal("history snapshot did not start")
		}
		in := EditInput{Kind: "expense", EventInput: EventInput{AccountID: legacy.ID, AmountMinor: 200, Currency: "MYR", OccurredAt: e.OccurredAt}}
		if _, err := service.EditEvent(ctx, owner, e.ID, 1, in, newFinancialWriteKey()); err != nil {
			t.Fatal(err)
		}
		close(trace.release)
		select {
		case events := <-result:
			found := false
			for _, v := range events {
				if v.ID == e.ID {
					found = true
					if v.Revision != 1 || v.AmountMinor != 100 || v.Movements[0].AmountMinor != -100 {
						t.Fatal("mixed history revision")
					}
				}
			}
			if !found {
				t.Fatal("event absent")
			}
		case err := <-failure:
			t.Fatal(err)
		}
	})
	t.Run("guarded API and response-loss replay", func(t *testing.T) {
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": "http://" + r.Host, "authorization_endpoint": "http://" + r.Host + "/authorize", "token_endpoint": "http://" + r.Host + "/token", "jwks_uri": "http://" + r.Host + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
		}))
		defer provider.Close()
		router, _, _ := idempotencyTestRouter(t, pool, provider.URL)
		request := func(method, path, body, key, tag, cookie, origin, token string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
			req.AddCookie(&http.Cookie{Name: "lifeos_session", Value: cookie})
			if method == "POST" {
				req.Header.Set("Origin", origin)
				req.Header.Set("X-CSRF-Token", token)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", key)
			}
			if tag != "" {
				req.Header.Set("If-Match", tag)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			return rec
		}
		me := request("GET", "/me", "", "", "", "edit-owner-session", "", "")
		var auth struct {
			Token string `json:"csrf_token"`
		}
		_ = json.Unmarshal(me.Body.Bytes(), &auth)
		body := fmt.Sprintf(`{"kind":"expense","currency":"MYR","amount_minor":"140","account_id":%q,"occurred_at":%q,"note":"API edit","allocations":[]}`, legacy.ID, event.OccurredAt.Format(time.RFC3339Nano))
		k := newFinancialWriteKey()
		get := request("GET", "/events/"+event.ID, "", "", "", "edit-owner-session", "", "")
		moneyStatus(t, get, 200)
		if get.Header().Get("ETag") != `"2"` {
			t.Fatal("etag")
		}
		for _, tag := range []string{`W/"2"`, `*`, `"0"`, `"02"`, `"2", "3"`, `"9223372036854775808"`} {
			moneyStatus(t, request("POST", "/events/"+event.ID+"/edit", body, k, tag, "edit-owner-session", moneyTestOrigin, auth.Token), 400)
		}
		moneyStatus(t, request("POST", "/events/"+event.ID+"/edit", body, k, "", "edit-owner-session", moneyTestOrigin, auth.Token), 428)
		moneyStatus(t, request("POST", "/events/"+event.ID+"/edit", body, k, `"2"`, "edit-owner-session", "https://foreign.invalid", auth.Token), 403)
		moneyStatus(t, request("POST", "/events/"+event.ID+"/edit", body, k, `"2"`, "edit-owner-session", moneyTestOrigin, "bad"), 403)
		for _, bad := range []string{strings.Replace(body, `"allocations":[]`, `"allocations":null`, 1), strings.Replace(body, `"allocations":[]`, `"allocations":[{"category_id":"`+category.ID+`","amount_minor":"140","unknown":true}]`, 1), strings.Replace(body, `"note":"API edit"`, `"note":null`, 1), strings.Replace(body, `"note":"API edit"`, `"note":"API edit","unexpected":true`, 1)} {
			moneyStatus(t, request("POST", "/events/"+event.ID+"/edit", bad, newFinancialWriteKey(), `"2"`, "edit-owner-session", moneyTestOrigin, auth.Token), 400)
		}
		otherMe := request("GET", "/me", "", "", "", "edit-other-session", "", "")
		var otherAuth struct {
			Token string `json:"csrf_token"`
		}
		_ = json.Unmarshal(otherMe.Body.Bytes(), &otherAuth)
		moneyStatus(t, request("POST", "/events/"+event.ID+"/edit", body, newFinancialWriteKey(), `"2"`, "edit-other-session", moneyTestOrigin, otherAuth.Token), 404)
		first := request("POST", "/events/"+event.ID+"/edit", body, k, `"2"`, "edit-owner-session", moneyTestOrigin, auth.Token)
		moneyStatus(t, first, 200)
		process := exec.Command(os.Args[0], "-test.run=^TestFinancialWriteRestartWorker$")
		process.Env = append(os.Environ(), "LIFEOS_I34_RESTART_WORKER=1", "LIFEOS_IDEMPOTENCY_TEST_DATABASE_URL="+os.Getenv("LIFEOS_EDIT_TEST_DATABASE_URL"), "LIFEOS_I34_ISSUER="+provider.URL, "LIFEOS_I34_BODY="+body, "LIFEOS_I34_KEY="+k, "LIFEOS_I34_STATUS=200", "LIFEOS_I35_PATH=/api/v1/events/"+event.ID+"/edit", "LIFEOS_I35_TAG=\"2\"", "LIFEOS_I35_SESSION=edit-owner-session")
		if output, err := process.CombinedOutput(); err != nil {
			t.Fatalf("Edit process restart: %s %v", output, err)
		}
		// Deliberately discard the successful DTO, then retry through a new router/service.
		router, _, _ = idempotencyTestRouter(t, pool, provider.URL)
		replay := request("POST", "/events/"+event.ID+"/edit", body, k, `"2"`, "edit-owner-session", moneyTestOrigin, auth.Token)
		moneyStatus(t, replay, 200)
		if strings.TrimSpace(replay.Body.String()) != `{"outcome":"already_applied"}` || replay.Header().Get("ETag") != "" {
			t.Fatal("replay disclosed old contents")
		}
		moneyStatus(t, request("POST", "/events/"+event.ID+"/void", "{}", newFinancialWriteKey(), `"2"`, "edit-owner-session", moneyTestOrigin, auth.Token), 412)
		deleteBefore, _ := service.Balance(ctx, owner, legacy.ID)
		if err := service.DeleteEvent(ctx, owner, event.ID, newFinancialWriteKey(), 3); err != nil {
			t.Fatal(err)
		}
		deleteAfter, _ := service.Balance(ctx, owner, legacy.ID)
		if deleteAfter.AmountMinor != deleteBefore.AmountMinor+140 {
			t.Fatal("Delete used pre-edit amount")
		}
		replay = request("POST", "/events/"+event.ID+"/edit", body, k, `"2"`, "edit-owner-session", moneyTestOrigin, auth.Token)
		moneyStatus(t, replay, 200)
		moneyStatus(t, request("GET", "/events/"+event.ID, "", "", "", "edit-owner-session", "", ""), 404)
		_, _ = pool.Exec(ctx, `UPDATE lifeos_sessions SET revoked_at=now() WHERE user_id=$1`, uid)
		moneyStatus(t, request("POST", "/events/"+event.ID+"/edit", body, k, `"2"`, "edit-owner-session", moneyTestOrigin, auth.Token), 401)
	})
	if err = runner.Steps(-1); err == nil {
		t.Fatal("populated audit rollback permitted")
	}
}

// Old functional regressions use a freshly reviewed version; stale races have explicit tokens above.
func testEventRevision(t *testing.T, s *Service, owner identity.OwnerID, id string) int64 {
	t.Helper()
	var revision int64
	if err := s.repo.pool.QueryRow(context.Background(), `SELECT event_revision FROM financial_events WHERE owner_id=$1 AND id=$2`, owner.String(), id).Scan(&revision); err != nil {
		return 1
	}
	return revision
}

type editSnapshotContextKey struct{}
type editSnapshotTracer struct {
	once             sync.Once
	reached, release chan struct{}
}

func (t *editSnapshotTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, editSnapshotContextKey{}, strings.Contains(data.SQL, "ORDER BY occurred_at DESC, id DESC"))
}
func (t *editSnapshotTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if match, _ := ctx.Value(editSnapshotContextKey{}).(bool); match {
		t.once.Do(func() { close(t.reached); <-t.release })
	}
}
