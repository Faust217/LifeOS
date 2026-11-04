package planning

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Faust217/LifeOS/internal/finance"
	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
)

func TestGoalSoftDeleteIntegration(t *testing.T) {
	pool, runner := goalTestDatabase(t, "LIFEOS_GOAL_DELETE_TEST_DATABASE_URL")
	ctx := context.Background()
	if err := runner.Migrate(11); err != nil {
		t.Fatal(err)
	}
	cookie := goalTestSession(t, pool, "61616161-6161-4161-8161-616161616161", "active")
	foreignCookie := goalTestSession(t, pool, "62626262-6262-4262-8262-626262626262", "active")
	auth := goalTestAuth(t, pool)
	owner := goalTestOwner(t, auth, cookie)
	other := goalTestOwner(t, auth, foreignCookie)
	const baseline = "63636363-6363-4363-8363-636363636363"
	if _, err := pool.Exec(ctx, `INSERT INTO goals(id,owner_id,title,completed_at,archived_at) VALUES($1,$2,'v11 retained baseline',now(),now());`, baseline, owner.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO goal_checklist_items(id,owner_id,goal_id,title,completed_at) VALUES(gen_random_uuid(),$1,$2,'v11 retained step',now())`, owner.String(), baseline); err != nil {
		t.Fatal(err)
	}
	digest := func() string {
		t.Helper()
		var s string
		if err := pool.QueryRow(ctx, `SELECT md5(jsonb_build_object('goals',(SELECT jsonb_agg(to_jsonb(g)-'deleted_at' ORDER BY id) FROM goals g),'items',(SELECT jsonb_agg(to_jsonb(i) ORDER BY id) FROM goal_checklist_items i))::text)`).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if _, err := pool.Exec(ctx, `UPDATE lifeos_users SET time_zone='Asia/Kuala_Lumpur' WHERE id=$1`, owner.String()); err != nil {
		t.Fatal(err)
	}
	f := finance.NewService(pool)
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	account, err := f.CreateAccount(ctx, owner, finance.AccountInput{Name: "Goal deletion invariant bank", Type: "bank", Currency: "MYR", OpeningBalanceMinor: 10000, OpeningBalanceAt: &at}, "64646464-6464-4464-8464-646464646464")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		key, _ := newUUID()
		if _, err = f.PostExpense(ctx, owner, finance.EventInput{AccountID: account.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: at.Add(time.Duration(i) * time.Hour)}, key); err != nil {
			t.Fatal(err)
		}
	}
	history, err := f.HistoryPage(ctx, owner, finance.HistoryQuery{Limit: 1})
	if err != nil || history.NextCursor == nil {
		t.Fatal(history, err)
	}
	financeDigest := func() string {
		t.Helper()
		var s string
		err := pool.QueryRow(ctx, `SELECT md5(jsonb_build_object('accounts',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM finance_accounts a),'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM financial_events e),'movements',(SELECT jsonb_agg(to_jsonb(m) ORDER BY id) FROM account_movements m),'allocations',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM category_allocations a),'audit',(SELECT jsonb_agg(to_jsonb(a) ORDER BY event_id,revision) FROM financial_event_revisions a),'keys',(SELECT jsonb_agg(to_jsonb(k) ORDER BY request_key) FROM finance_write_requests k),'history',(SELECT jsonb_agg(to_jsonb(h) ORDER BY owner_id) FROM finance_history_revisions h))::text)`).Scan(&s)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	financeBefore := financeDigest()
	before := digest()
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if digest() != before {
		t.Fatal("upgrade changed parent/child facts")
	}
	if err := runner.Up(); !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	if err := runner.Steps(-1); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	service := NewService(pool)
	if financeDigest() != financeBefore {
		t.Fatal("v12 changed Finance facts")
	}

	makeGoal := func(name string) Goal {
		t.Helper()
		g, e := service.CreateGoal(ctx, owner, name)
		if e != nil {
			t.Fatal(e)
		}
		g, e = service.AddItem(ctx, owner, g.ID, "Retained step")
		if e != nil {
			t.Fatal(e)
		}
		return g
	}
	api := chi.NewRouter()
	auth.RegisterRoutes(api)
	RegisterRoutes(api, auth, service)
	me := goalTestDecode[struct {
		CSRF string `json:"csrf_token"`
	}](t, goalTestRequest(t, api, "GET", "/me", "", cookie, "", ""), 200)
	// Every lifecycle state is eligible; snapshots of child rows must remain exact.
	for _, state := range []string{"active", "completed", "archived", "completed-archived", "empty"} {
		t.Run(state, func(t *testing.T) {
			var g Goal
			err = nil
			if state == "empty" {
				g, err = service.CreateGoal(ctx, owner, state)
			} else {
				g = makeGoal(state)
			}
			if err != nil {
				t.Fatal(err)
			}
			if state == "completed" || state == "completed-archived" {
				g, err = service.SetItemCompleted(ctx, owner, g.ID, g.Items[0].ID, true)
				if err != nil {
					t.Fatal(err)
				}
				g, err = service.SetGoalCompleted(ctx, owner, g.ID, true)
				if err != nil {
					t.Fatal(err)
				}
			}
			if state == "archived" || state == "completed-archived" {
				g, err = service.ArchiveGoal(ctx, owner, g.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			var children string
			if err = pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(i) ORDER BY id),'[]')::text FROM goal_checklist_items i WHERE owner_id=$1 AND goal_id=$2`, owner.String(), g.ID).Scan(&children); err != nil {
				t.Fatal(err)
			}
			response := goalTestRequest(t, api, "DELETE", "/goals/"+g.ID, "", cookie, goalTestOrigin, me.CSRF)
			goalTestStatus(t, response, 204)
			if response.Body.Len() != 0 || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("unsafe 204")
			}
			var deleted, updated time.Time
			var parent string
			if err = pool.QueryRow(ctx, `SELECT deleted_at,updated_at,(to_jsonb(g)-'deleted_at'-'updated_at')::text FROM goals g WHERE id=$1`, g.ID).Scan(&deleted, &updated, &parent); err != nil {
				t.Fatal(err)
			}
			// Reconstruct the service to prove retries use durable state, not service memory.
			if err = NewService(pool).DeleteGoal(ctx, owner, g.ID); err != nil {
				t.Fatal(err)
			}
			var same bool
			var after string
			if err = pool.QueryRow(ctx, `SELECT deleted_at=$2 AND updated_at=$3 FROM goals WHERE id=$1`, g.ID, deleted, updated).Scan(&same); err != nil || !same {
				t.Fatal("retry changed timestamps", err)
			}
			if err = pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(i) ORDER BY id),'[]')::text FROM goal_checklist_items i WHERE owner_id=$1 AND goal_id=$2`, owner.String(), g.ID).Scan(&after); err != nil || after != children {
				t.Fatal("children changed", err)
			}
			if _, err = service.GetGoal(ctx, owner, g.ID); !errors.Is(err, ErrNotFound) {
				t.Fatal(err)
			}
			goalTestError(t, goalTestRequest(t, api, "GET", "/goals/"+g.ID, "", cookie, "", ""), 404, "not_found")
			for _, mutation := range []struct{ method, suffix, body string }{{"POST", "/rename", `{"title":"Blocked"}`}, {"POST", "/completion", `{"completed":true}`}, {"POST", "/completion", `{"completed":false}`}, {"POST", "/archive", ""}, {"POST", "/restore", ""}, {"POST", "/items", `{"title":"Blocked"}`}} {
				goalTestError(t, goalTestRequest(t, api, mutation.method, "/goals/"+g.ID+mutation.suffix, mutation.body, cookie, goalTestOrigin, me.CSRF), 404, "not_found")
			}
			if len(g.Items) > 0 {
				base := "/goals/" + g.ID + "/items/" + g.Items[0].ID
				for _, mutation := range []struct{ method, suffix, body string }{{"POST", "/rename", `{"title":"Blocked"}`}, {"POST", "/completion", `{"completed":true}`}, {"POST", "/completion", `{"completed":false}`}, {"DELETE", "", ""}} {
					goalTestError(t, goalTestRequest(t, api, mutation.method, base+mutation.suffix, mutation.body, cookie, goalTestOrigin, me.CSRF), 404, "not_found")
				}
			}
			if err = service.DeleteGoal(ctx, other, g.ID); !errors.Is(err, ErrNotFound) {
				t.Fatal("foreign delete", err)
			}
			goals, e := service.ListGoals(ctx, owner)
			if e != nil {
				t.Fatal(e)
			}
			for _, visible := range goals {
				if visible.ID == g.ID {
					t.Fatal("deleted Goal leaked")
				}
			}
		})
	}
	t.Run("guards and malformed commands", func(t *testing.T) {
		g := makeGoal("Guarded Goal")
		path := "/goals/" + g.ID
		goalTestError(t, goalTestRequest(t, api, "DELETE", path, "", nil, goalTestOrigin, me.CSRF), 401, "authentication_required")
		for _, state := range []string{"expired", "revoked", "disabled"} {
			id, _ := newUUID()
			c := goalTestSession(t, pool, id, state)
			goalTestStatus(t, goalTestRequest(t, api, "DELETE", path, "", c, goalTestOrigin, me.CSRF), 401)
		}
		goalTestError(t, goalTestRequest(t, api, "DELETE", path, "", cookie, "https://evil.invalid", me.CSRF), 403, "invalid_origin")
		goalTestError(t, goalTestRequest(t, api, "DELETE", path, "", cookie, "", me.CSRF), 403, "invalid_origin")
		goalTestError(t, goalTestRequest(t, api, "DELETE", path, "", cookie, goalTestOrigin, "bad"), 403, "invalid_csrf")
		goalTestStatus(t, goalTestRequest(t, api, "DELETE", path+"?owner_id="+other.String(), "", cookie, goalTestOrigin, me.CSRF), 400)
		goalTestStatus(t, goalTestRequest(t, api, "DELETE", path+"?x=1", "", cookie, goalTestOrigin, me.CSRF), 400)
		goalTestStatus(t, goalTestRequest(t, api, "DELETE", path, `{}`, cookie, goalTestOrigin, me.CSRF), 400)
		goalTestStatus(t, goalTestRequest(t, api, "DELETE", "/goals/bad", "", cookie, goalTestOrigin, me.CSRF), 400)
		if _, err = service.GetGoal(ctx, owner, g.ID); err != nil {
			t.Fatal("guard denial mutated Goal", err)
		}
		foreignMe := goalTestDecode[struct {
			CSRF string `json:"csrf_token"`
		}](t, goalTestRequest(t, api, "GET", "/me", "", foreignCookie, "", ""), 200)
		goalTestError(t, goalTestRequest(t, api, "DELETE", path, "", foreignCookie, goalTestOrigin, foreignMe.CSRF), 404, "not_found")
		missing, _ := newUUID()
		goalTestError(t, goalTestRequest(t, api, "DELETE", "/goals/"+missing, "", cookie, goalTestOrigin, me.CSRF), 404, "not_found")
	})
	t.Run("lost HTTP response then guarded same ID retry", func(t *testing.T) {
		g := makeGoal("Lost reply Goal")
		var drop atomic.Bool
		drop.Store(true)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if drop.Swap(false) {
				record := httptest.NewRecorder()
				api.ServeHTTP(record, r)
				if record.Code != 204 {
					t.Error("first delete failed", record.Code)
				}
				connection, _, e := w.(http.Hijacker).Hijack()
				if e != nil {
					t.Error(e)
					return
				}
				_ = connection.Close()
				return
			}
			api.ServeHTTP(w, r)
		}))
		defer server.Close()
		client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 5 * time.Second}
		send := func() (*http.Response, error) {
			req, e := http.NewRequest("DELETE", server.URL+"/goals/"+g.ID, nil)
			if e != nil {
				t.Fatal(e)
			}
			req.AddCookie(cookie)
			req.Header.Set("Origin", goalTestOrigin)
			req.Header.Set("X-CSRF-Token", me.CSRF)
			return client.Do(req)
		}
		if response, e := send(); e == nil {
			response.Body.Close()
			t.Fatal("lost response unexpectedly confirmed")
		}
		var first time.Time
		if e := pool.QueryRow(ctx, `SELECT deleted_at FROM goals WHERE id=$1`, g.ID).Scan(&first); e != nil {
			t.Fatal(e)
		}
		response, e := send()
		if e != nil {
			t.Fatal(e)
		}
		data, e := io.ReadAll(response.Body)
		response.Body.Close()
		if e != nil || response.StatusCode != 204 || len(data) != 0 {
			t.Fatal("unsafe retry", e)
		}
		var stable bool
		if e = pool.QueryRow(ctx, `SELECT deleted_at=$2 FROM goals WHERE id=$1`, g.ID, first).Scan(&stable); e != nil || !stable {
			t.Fatal("retry changed marker", e)
		}
	})
	t.Run("rollback and cancellation", func(t *testing.T) {
		g := makeGoal("Failure rollback")
		if _, err = pool.Exec(ctx, `CREATE FUNCTION fail_goal_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.deleted_at IS NOT NULL THEN RAISE EXCEPTION 'synthetic failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_goal_delete AFTER UPDATE ON goals FOR EACH ROW EXECUTE FUNCTION fail_goal_delete()`); err != nil {
			t.Fatal(err)
		}
		if err = service.DeleteGoal(ctx, owner, g.ID); err == nil {
			t.Fatal("injected failure accepted")
		}
		failed := goalTestRequest(t, api, "DELETE", "/goals/"+g.ID, "", cookie, goalTestOrigin, me.CSRF)
		goalTestError(t, failed, 500, "internal_error")
		if strings.Contains(failed.Body.String(), "synthetic failure") {
			t.Fatal("SQL error leaked")
		}
		if _, err = pool.Exec(ctx, `DROP TRIGGER fail_goal_delete ON goals; DROP FUNCTION fail_goal_delete()`); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `CREATE FUNCTION fail_goal_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.deleted_at IS NOT NULL THEN RAISE EXCEPTION 'synthetic commit failure'; END IF; RETURN NEW; END $$; CREATE CONSTRAINT TRIGGER fail_goal_commit AFTER UPDATE ON goals DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fail_goal_commit()`); err != nil {
			t.Fatal(err)
		}
		if err = service.DeleteGoal(ctx, owner, g.ID); err == nil {
			t.Fatal("commit failure accepted")
		}
		if _, err = pool.Exec(ctx, `DROP TRIGGER fail_goal_commit ON goals; DROP FUNCTION fail_goal_commit()`); err != nil {
			t.Fatal(err)
		}
		visible, e := service.GetGoal(ctx, owner, g.ID)
		if e != nil || !visible.UpdatedAt.Equal(g.UpdatedAt) || len(visible.Items) != 1 {
			t.Fatal("partial rollback", e)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if err = service.DeleteGoal(cancelled, owner, g.ID); err == nil {
			t.Fatal("cancelled deletion accepted")
		}
		locked, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer locked.Rollback(ctx)
		if _, e = locked.Exec(ctx, `SELECT id FROM goals WHERE id=$1 FOR UPDATE`, g.ID); e != nil {
			t.Fatal(e)
		}
		if e = service.DeleteGoal(ctx, owner, g.ID); e == nil {
			t.Fatal("lock timeout bypassed")
		}
		_ = locked.Rollback(ctx)
		if _, e = service.GetGoal(ctx, owner, g.ID); e != nil {
			t.Fatal("timeout changed Goal", e)
		}
	})
	verifyGoalDeleteRaces(t, pool, service, owner)
	if financeDigest() != financeBefore {
		t.Fatal("Goal deletion changed Finance")
	}
	b, e := f.Balance(ctx, owner, account.ID)
	if e != nil || b.AmountMinor != 9800 {
		t.Fatal(b, e)
	}
	if _, e = f.HistoryPage(ctx, owner, finance.HistoryQuery{Limit: 1, Revision: history.Revision, Cursor: *history.NextCursor}); e != nil {
		t.Fatal("Goal deletion invalidated Finance history", e)
	}
	if e = runner.Steps(-1); e == nil {
		t.Fatal("populated down resurrected Goals")
	}
	var retained int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM goals WHERE deleted_at IS NOT NULL`).Scan(&retained); e != nil || retained == 0 {
		t.Fatal("failed down lost markers", e)
	}
}
