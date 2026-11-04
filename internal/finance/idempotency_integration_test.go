package finance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/Faust217/LifeOS/internal/platform/config"
	"github.com/Faust217/LifeOS/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func idempotencyTestRouter(t *testing.T, pool *pgxpool.Pool, issuer string) (http.Handler, *identity.Auth, *Service) {
	t.Helper()
	auth, err := identity.NewAuth(context.Background(), pool, config.AuthConfig{ClientID: "idempotency-test", ClientSecret: "test-only", Issuer: issuer, PublicOrigin: moneyTestOrigin})
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(pool)
	api := chi.NewRouter()
	auth.RegisterRoutes(api)
	RegisterRoutes(api, auth, s)
	return httpapi.NewRouter(slog.New(slog.NewJSONHandler(io.Discard, nil)), moneyTestReadiness{}, api), auth, s
}

func TestFinancialWriteIdempotencyIntegration(t *testing.T) {
	pool, runner := financeTestDatabase(t, "LIFEOS_IDEMPOTENCY_TEST_DATABASE_URL")
	ctx := context.Background()
	if err := runner.Migrate(8); err != nil {
		t.Fatal(err)
	}
	const user = "11111111-1111-4111-8111-111111111111"
	cookie := moneyTestSession(t, pool, user, "idempotency-owner", "active")
	otherCookie := moneyTestSession(t, pool, "22222222-2222-4222-8222-222222222222", "idempotency-other", "active")
	if _, err := pool.Exec(ctx, `UPDATE lifeos_users SET time_zone='Asia/Kuala_Lumpur'`); err != nil {
		t.Fatal(err)
	}
	// Populated v8 upgrade must not reinterpret the ledger or introduce anchors.
	legacyID := newFinancialWriteKey()
	if _, err := pool.Exec(ctx, `INSERT INTO finance_accounts(id,owner_id,name,account_type,currency,opening_balance_minor) VALUES($1,$2,'Legacy at v8','bank','MYR',5000)`, legacyID, user); err != nil {
		t.Fatal(err)
	}
	if err := runner.Migrate(9); err != nil {
		t.Fatal(err)
	}
	var opening int64
	var anchor *time.Time
	if err := pool.QueryRow(ctx, `SELECT opening_balance_minor,opening_balance_at FROM finance_accounts WHERE id=$1`, legacyID).Scan(&opening, &anchor); err != nil || opening != 5000 || anchor != nil {
		t.Fatal("v8 upgrade changed legacy account", err)
	}
	if err := runner.Steps(-1); err != nil {
		t.Fatal("empty v9 down", err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": "http://" + r.Host, "authorization_endpoint": "http://" + r.Host + "/authorize", "token_endpoint": "http://" + r.Host + "/token", "jwks_uri": "http://" + r.Host + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
	}))
	defer provider.Close()
	router, auth, service := idempotencyTestRouter(t, pool, provider.URL)
	// Fault injection is outside the production handler. The real guarded router commits first.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-I34-Lose-Reply") == "1" {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, r)
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		router.ServeHTTP(w, r)
	}))
	defer server.Close()
	ownerFrom := func(c *http.Cookie) identity.OwnerID {
		var owner identity.OwnerID
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(c)
		auth.RequireRead(func(_ http.ResponseWriter, _ *http.Request, o identity.OwnerID) { owner = o })(httptest.NewRecorder(), req)
		if !owner.Valid() {
			t.Fatal("missing test owner")
		}
		return owner
	}
	owner := ownerFrom(cookie)
	var tagsMu sync.Mutex
	tags := map[string]string{}
	send := func(path, body, key string, c *http.Cookie, token string, change func(*http.Request)) (*httptest.ResponseRecorder, error) {
		req, _ := http.NewRequest("POST", server.URL+"/api/v1"+path, strings.NewReader(body))
		req.AddCookie(c)
		req.Header.Set("Origin", moneyTestOrigin)
		req.Header.Set("X-CSRF-Token", token)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		if strings.HasSuffix(path, "/void") || strings.HasSuffix(path, "/delete") {
			tagsMu.Lock()
			tag, ok := tags[key]
			if !ok {
				var rev int64
				id := strings.Split(strings.TrimPrefix(path, "/events/"), "/")[0]
				_ = pool.QueryRow(ctx, `SELECT event_revision FROM financial_events WHERE id=$1`, id).Scan(&rev)
				if rev == 0 {
					rev = 1
				}
				tag = strconv.Quote(strconv.FormatInt(rev, 10))
				tags[key] = tag
			}
			tagsMu.Unlock()
			req.Header.Set("If-Match", tag)
		}
		if change != nil {
			change(req)
		}
		res, err := server.Client().Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}
		rec := httptest.NewRecorder()
		for k, v := range res.Header {
			rec.Header()[k] = v
		}
		rec.WriteHeader(res.StatusCode)
		_, _ = rec.Write(data)
		return rec, nil
	}
	csrf := func(c *http.Cookie) string {
		req := httptest.NewRequest("GET", "/api/v1/me", nil)
		req.AddCookie(c)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		var body struct {
			Token string `json:"csrf_token"`
		}
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Token == "" {
			t.Fatal("csrf setup", rec.Code)
		}
		return body.Token
	}
	token, otherToken := csrf(cookie), csrf(otherCookie)
	request := func(path, body, key string) *httptest.ResponseRecorder {
		rec, err := send(path, body, key, cookie, token, nil)
		if err != nil {
			t.Fatal(err)
		}
		return rec
	}
	jsonBody := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	receipt := func(rec *httptest.ResponseRecorder) {
		t.Helper()
		moneyStatus(t, rec, 200)
		if strings.TrimSpace(rec.Body.String()) != `{"outcome":"already_applied"}` || rec.Header().Get("Idempotency-Replayed") != "true" || !strings.Contains(rec.Header().Get("Cache-Control"), "private") {
			t.Fatalf("unsafe replay %s %v", rec.Body.String(), rec.Header())
		}
	}
	account := func(name string) accountResponse {
		key := newFinancialWriteKey()
		body := jsonBody(map[string]any{"name": name, "type": "bank", "currency": "MYR", "opening_balance_minor": "10000", "opening_balance_at": "2026-10-01T00:00:00Z"})
		res := request("/accounts", body, key)
		a := moneyDecode[accountResponse](t, res, 201)
		if res.Header().Get("Idempotency-Replayed") != "false" {
			t.Fatal("missing initial success header")
		}
		receipt(request("/accounts", body, key))
		return a
	}
	bank, cash := account("Bank"), account("Cash")
	food, err := service.CreateCategory(ctx, owner, CategoryInput{Name: "Food", Kind: "expense"})
	if err != nil {
		t.Fatal(err)
	}
	medical, err := service.CreateCategory(ctx, owner, CategoryInput{Name: "Medical", Kind: "expense"})
	if err != nil {
		t.Fatal(err)
	}
	event := func(note string, amount string) string {
		return jsonBody(map[string]any{"kind": "expense", "account_id": bank.ID, "amount_minor": amount, "currency": "MYR", "occurred_at": "2026-10-02T08:00:00.123456999+08:00", "note": note, "allocations": []any{}})
	}
	balance := func(id string) int64 {
		b, err := service.Balance(ctx, owner, id)
		if err != nil {
			t.Fatal(err)
		}
		return b.AmountMinor
	}
	markerCount := func(o string) int64 {
		var n int64
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM finance_write_requests WHERE owner_id=$1`, o).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("canonical splits conflicts and later Void Delete", func(t *testing.T) {
		key := newFinancialWriteKey()
		body := jsonBody(map[string]any{"kind": "expense", "account_id": strings.ToUpper(bank.ID), "amount_minor": "1250", "currency": "MYR", "occurred_at": "2026-10-02T08:00:00.123456999+08:00", "note": " exact note\n", "allocations": []any{map[string]string{"category_id": medical.ID, "amount_minor": "500"}, map[string]string{"category_id": food.ID, "amount_minor": "750"}}})
		before := balance(bank.ID)
		e := moneyDecode[eventResponse](t, request("/events", body, key), 201)
		if balance(bank.ID) != before-1250 {
			t.Fatal("expense effect")
		}
		var parsed map[string]any
		_ = json.Unmarshal([]byte(body), &parsed)
		parsed["account_id"] = bank.ID
		parsed["occurred_at"] = "2026-10-02T00:00:00.123456Z"
		pairs := parsed["allocations"].([]any)
		pairs[0], pairs[1] = pairs[1], pairs[0]
		equivalent := jsonBody(parsed)
		receipt(request("/events", equivalent, key))
		parsed["note"] = "different"
		moneyStatus(t, request("/events", jsonBody(parsed), key), 409)
		voidKey := newFinancialWriteKey()
		moneyStatus(t, request("/events/"+e.ID+"/void", `{}`, voidKey), 204)
		receipt(request("/events/"+e.ID+"/void", `{"reason":"  "}`, voidKey))
		deleteKey := newFinancialWriteKey()
		moneyStatus(t, request("/events/"+e.ID+"/delete", "", deleteKey), 204)
		receipt(request("/events/"+e.ID+"/delete", "", deleteKey))
		receipt(request("/events", equivalent, key))
		receipt(request("/events/"+e.ID+"/void", `{}`, voidKey))
		if balance(bank.ID) != before {
			t.Fatal("replay reversed twice")
		}
		// New intent still observes deleted status, and a different operation cannot reuse K.
		moneyStatus(t, request("/events/"+e.ID+"/void", `{}`, newFinancialWriteKey()), 404)
		moneyStatus(t, request("/events/"+e.ID+"/delete", "", key), 409)
	})
	t.Run("income transfer and posted deletion preserve amounts", func(t *testing.T) {
		before, otherBefore := balance(bank.ID), balance(cash.ID)
		key := newFinancialWriteKey()
		body := jsonBody(map[string]any{"kind": "income", "account_id": bank.ID, "amount_minor": "50", "currency": "MYR", "occurred_at": "2026-10-02T00:00:00Z", "allocations": []any{}})
		e := moneyDecode[eventResponse](t, request("/events", body, key), 201)
		receipt(request("/events", body, key))
		tk := newFinancialWriteKey()
		tb := jsonBody(map[string]any{"from_account_id": bank.ID, "to_account_id": cash.ID, "amount_minor": "100", "currency": "MYR", "occurred_at": "2026-10-02T00:00:00Z"})
		tr := moneyDecode[eventResponse](t, request("/transfers", tb, tk), 201)
		receipt(request("/transfers", tb, tk))
		if len(tr.Movements) != 2 || balance(bank.ID) != before-50 || balance(cash.ID) != otherBefore+100 {
			t.Fatal("transfer duplicated")
		}
		moneyStatus(t, request("/events/"+tr.ID+"/delete", "", newFinancialWriteKey()), 204)
		moneyStatus(t, request("/events/"+e.ID+"/delete", "", newFinancialWriteKey()), 204)
		receipt(request("/transfers", tb, tk))
		receipt(request("/events", body, key))
		if balance(bank.ID) != before || balance(cash.ID) != otherBefore {
			t.Fatal("delete/replay balance")
		}
	})
	t.Run("lost response and concurrent identical requests", func(t *testing.T) {
		key := newFinancialWriteKey()
		body := event("lost reply", "5")
		before := balance(bank.ID)
		if _, err := send("/events", body, key, cookie, token, func(r *http.Request) { r.Header.Set("X-I34-Lose-Reply", "1") }); err == nil {
			t.Fatal("reply was not lost")
		}
		receipt(request("/events", body, key))
		if balance(bank.ID) != before-5 {
			t.Fatal("lost response duplicate")
		}
		key = newFinancialWriteKey()
		body = event("concurrent same key", "7")
		before = balance(bank.ID)
		var wg sync.WaitGroup
		results := make(chan int, 24)
		for i := 0; i < 24; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec, err := send("/events", body, key, cookie, token, nil)
				if err != nil {
					results <- 0
				} else {
					results <- rec.Code
				}
			}()
		}
		wg.Wait()
		close(results)
		created := 0
		for status := range results {
			if status == 201 {
				created++
			} else if status != 200 && status != 503 {
				t.Fatalf("concurrent status %d", status)
			}
		}
		if created != 1 {
			t.Fatalf("created=%d", created)
		}
		receipt(request("/events", body, key))
		if balance(bank.ID) != before-7 {
			t.Fatal("concurrent duplicated")
		}
	})
	t.Run("opening confirmation replay before revision check", func(t *testing.T) {
		preview, err := service.PreviewOpeningBalance(ctx, owner, legacyID, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), nil)
		if err != nil {
			t.Fatal(err)
		}
		key := newFinancialWriteKey()
		body := jsonBody(map[string]any{"opening_balance_at": preview.OpeningBalanceAt, "opening_balance_minor": *preview.ChosenOpeningMinor, "balance_revision": preview.BalanceRevision, "acknowledge_change": false})
		path := "/accounts/" + legacyID + "/opening-balance/confirm"
		moneyStatus(t, request(path, body, key), 200)
		var advance map[string]any
		_ = json.Unmarshal([]byte(event("advance revision", "1")), &advance)
		advance["account_id"] = legacyID
		moneyStatus(t, request("/events", jsonBody(advance), newFinancialWriteKey()), 201)
		receipt(request(path, body, key))
		moneyStatus(t, request(path, body, newFinancialWriteKey()), 409)
	})
	t.Run("guards validation and owner isolation apply on replays", func(t *testing.T) {
		for i := 0; i < cap(service.repo.writeSlots); i++ {
			service.repo.writeSlots <- struct{}{}
		}
		busy, err := send("/events", event("admission", "1"), newFinancialWriteKey(), cookie, token, nil)
		for i := 0; i < cap(service.repo.writeSlots); i++ {
			<-service.repo.writeSlots
		}
		if err != nil {
			t.Fatal(err)
		}
		moneyStatus(t, busy, 503)
		if busy.Header().Get("Retry-After") != "1" {
			t.Fatal("missing busy retry delay")
		}
		key := newFinancialWriteKey()
		body := event("guarded", "1")
		moneyStatus(t, request("/events", body, key), 201)
		for _, change := range []func(*http.Request){func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, func(r *http.Request) { r.Header.Set("X-CSRF-Token", "wrong") }} {
			rec, err := send("/events", body, key, cookie, token, change)
			if err != nil {
				t.Fatal(err)
			}
			moneyStatus(t, rec, 403)
		}
		res, err := send("/events", body, key, otherCookie, otherToken, nil)
		if err != nil {
			t.Fatal(err)
		}
		moneyStatus(t, res, 404)
		// Same key is independent for another owner, but cannot authorize a foreign account.
		if markerCount("22222222-2222-4222-8222-222222222222") != 0 {
			t.Fatal("foreign failed command bound a key")
		}
		otherAccountBody := jsonBody(map[string]any{"name": "Other owner account", "type": "bank", "currency": "USD", "opening_balance_minor": "0", "opening_balance_at": "2026-10-01T00:00:00Z"})
		independent, err := send("/accounts", otherAccountBody, key, otherCookie, otherToken, nil)
		if err != nil {
			t.Fatal(err)
		}
		moneyStatus(t, independent, 201)
		moneyStatus(t, request("/events", body, ""), 400)
		for _, bad := range []string{`null`, `{"kind":"expense","kind":"income"}`, `{"unknown":true}`} {
			moneyStatus(t, request("/events", bad, newFinancialWriteKey()), 400)
		}
		rec, err := send("/events", body, newFinancialWriteKey(), cookie, token, func(r *http.Request) { r.Header.Add("Idempotency-Key", newFinancialWriteKey()) })
		if err != nil {
			t.Fatal(err)
		}
		moneyStatus(t, rec, 400)
		receipt(request("/events", body, key))
	})
	t.Run("rollback crash and transient owner lock", func(t *testing.T) {
		key := newFinancialWriteKey()
		before := balance(bank.ID)
		n := markerCount(user)
		input := EventInput{AccountID: bank.ID, AmountMinor: 13, Currency: "MYR", OccurredAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), Allocations: []AllocationInput{{food.ID, 13}}}
		_, err := pool.Exec(ctx, `CREATE FUNCTION i34_fail_allocation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic failure'; END $$; CREATE TRIGGER i34_fail BEFORE INSERT ON category_allocations FOR EACH ROW EXECUTE FUNCTION i34_fail_allocation()`)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = service.PostExpense(ctx, owner, input, key); err == nil {
			t.Fatal("injected write succeeded")
		}
		if markerCount(user) != n || balance(bank.ID) != before {
			t.Fatal("partial rollback")
		}
		_, _ = pool.Exec(ctx, `DROP TRIGGER i34_fail ON category_allocations; DROP FUNCTION i34_fail_allocation()`)
		if _, err = service.PostExpense(ctx, owner, input, key); err != nil {
			t.Fatal("rolled back key poisoned", err)
		}
		crashKey := newFinancialWriteKey()
		before = balance(bank.ID)
		n = markerCount(user)
		pidReady := make(chan int32)
		resume := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			_, err := executeFinancialWrite(ctx, service.repo, owner, crashKey, financialCommand{operation: "event.create", parameters: []any{"crash"}}, func(ctx context.Context, tx pgx.Tx) (Event, error) {
				e, err := service.repo.postInTx(ctx, tx, owner, postSpec{requestKey: crashKey, kind: "expense", amount: 3, currency: "MYR", occurredAt: input.OccurredAt, movements: []movementSpec{{bank.ID, -3}}})
				if err != nil {
					return e, err
				}
				var pid int32
				if err = tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
					return e, err
				}
				pidReady <- pid
				<-resume
				return e, nil
			})
			done <- err
		}()
		select {
		case pid := <-pidReady:
			if _, err := pool.Exec(ctx, `SELECT pg_terminate_backend($1)`, pid); err != nil {
				t.Fatal(err)
			}
			close(resume)
		case <-time.After(5 * time.Second):
			t.Fatal("crash fixture did not reach uncommitted mutation")
		}
		if err := <-done; err == nil {
			t.Fatal("terminated transaction committed")
		}
		if markerCount(user) != n || balance(bank.ID) != before {
			t.Fatal("crash retained partial data")
		}
		input.AmountMinor = 3
		input.Allocations = nil
		if _, err := service.PostExpense(ctx, owner, input, crashKey); err != nil {
			t.Fatal("crash retry", err)
		}
		// Holding a covered owner write blocks another key only until the configured lock deadline.
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var keyHash [32]byte = sha256.Sum256([]byte("lifeos.finance.write-owner.v1|" + user))
		var ownerLock int64
		for _, b := range keyHash[:8] {
			ownerLock = ownerLock<<8 | int64(b)
		}
		_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, ownerLock)
		if err != nil {
			t.Fatal(err)
		}
		_, err = service.PostExpense(ctx, owner, input, newFinancialWriteKey())
		_ = tx.Rollback(ctx)
		if !errors.Is(err, ErrWriteBusy) {
			t.Fatal("unbounded or wrong lock failure", err)
		}
	})
	t.Run("real 90000 warning and 100000 hard limit", func(t *testing.T) {
		capCookie := moneyTestSession(t, pool, "33333333-3333-4333-8333-333333333333", "capacity-private-owner", "active")
		_, _ = pool.Exec(ctx, `UPDATE lifeos_users SET time_zone='Asia/Kuala_Lumpur' WHERE id='33333333-3333-4333-8333-333333333333'`)
		capToken := csrf(capCookie)
		b := jsonBody(map[string]any{"name": "Private capacity account", "type": "bank", "currency": "MYR", "opening_balance_minor": "0", "opening_balance_at": "2026-10-01T00:00:00Z"})
		rec, err := send("/accounts", b, newFinancialWriteKey(), capCookie, capToken, nil)
		if err != nil {
			t.Fatal(err)
		}
		a := moneyDecode[accountResponse](t, rec, 201)
		fill := func(n int) {
			_, err := pool.Exec(ctx, `INSERT INTO finance_write_requests(owner_id,request_key,operation,fingerprint_version,request_fingerprint) SELECT '33333333-3333-4333-8333-333333333333',gen_random_uuid(),'event.create',1,decode(repeat('00',32),'hex') FROM generate_series(1,$1::int)`, n)
			if err != nil {
				t.Fatal(err)
			}
		}
		fill(89998)
		var logs bytes.Buffer
		oldLogger := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
		defer slog.SetDefault(oldLogger)
		key := newFinancialWriteKey()
		b = jsonBody(map[string]any{"kind": "income", "account_id": a.ID, "amount_minor": "1", "currency": "MYR", "occurred_at": "2026-10-02T00:00:00Z", "allocations": []any{}})
		rec, err = send("/events", b, key, capCookie, capToken, nil)
		if err != nil {
			t.Fatal(err)
		}
		moneyStatus(t, rec, 201)
		if !strings.Contains(logs.String(), `"band":"approaching"`) {
			t.Fatal("no capacity warning")
		}
		fill(10000)
		rec, err = send("/events", b, newFinancialWriteKey(), capCookie, capToken, nil)
		if err != nil {
			t.Fatal(err)
		}
		moneyStatus(t, rec, 409)
		if !strings.Contains(rec.Body.String(), "idempotency_capacity_exceeded") {
			t.Fatal(rec.Body.String())
		}
		rec, err = send("/events", b, key, capCookie, capToken, nil)
		if err != nil {
			t.Fatal(err)
		}
		receipt(rec)
		if markerCount("33333333-3333-4333-8333-333333333333") != 100000 {
			t.Fatal("capacity mutated")
		}
		if strings.Contains(logs.String(), a.ID) || strings.Contains(logs.String(), "capacity-private") || strings.Contains(logs.String(), key) || strings.Contains(logs.String(), "33333333-") {
			t.Fatal("private capacity logs")
		}
	})
	t.Run("process restart uses PostgreSQL outcome", func(t *testing.T) {
		key := newFinancialWriteKey()
		body := event("child process restart", "2")
		for _, want := range []string{"201", "200"} {
			child := exec.Command(os.Args[0], "-test.run=^TestFinancialWriteRestartWorker$")
			child.Env = append(os.Environ(), "LIFEOS_I34_RESTART_WORKER=1", "LIFEOS_I34_ISSUER="+provider.URL, "LIFEOS_I34_BODY="+body, "LIFEOS_I34_KEY="+key, "LIFEOS_I34_STATUS="+want)
			if output, err := child.CombinedOutput(); err != nil {
				t.Fatalf("restart worker %v: %s", err, output)
			}
		}
		receipt(request("/events", body, key))
	})
	t.Run("revoked session after commit releases no contents", func(t *testing.T) {
		_, err := pool.Exec(ctx, `CREATE FUNCTION i34_revoke() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.note='revoke-session' THEN UPDATE lifeos_sessions SET revoked_at=clock_timestamp() WHERE user_id=NEW.owner_id; END IF; RETURN NEW; END $$; CREATE TRIGGER i34_revoke AFTER INSERT ON financial_events FOR EACH ROW EXECUTE FUNCTION i34_revoke()`)
		if err != nil {
			t.Fatal(err)
		}
		key := newFinancialWriteKey()
		body := event("revoke-session", "1")
		res := request("/events", body, key)
		moneyStatus(t, res, 401)
		if strings.Contains(res.Body.String(), "revoke-session") {
			t.Fatal("contents released")
		}
		// Same owner reauthenticates with a new active session and fresh CSRF.
		secret := "idempotency-renewed-session"
		hash := sha256.Sum256([]byte(secret))
		if _, err = pool.Exec(ctx, `INSERT INTO lifeos_sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, hash[:], user); err != nil {
			t.Fatal(err)
		}
		cookie = &http.Cookie{Name: "lifeos_session", Value: secret}
		token = csrf(cookie)
		receipt(request("/events", body, key))
	})
	if err := runner.Steps(-1); err == nil {
		t.Fatal("populated v9 down discarded committed keys")
	}
}

// Invoked twice by the parent integration test in distinct OS processes.
func TestFinancialWriteRestartWorker(t *testing.T) {
	if os.Getenv("LIFEOS_I34_RESTART_WORKER") != "1" {
		return
	}
	dsn := os.Getenv("LIFEOS_IDEMPOTENCY_TEST_DATABASE_URL")
	if !strings.Contains(dsn, "_test?") {
		t.Fatal("worker requires disposable test DSN")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	router, _, _ := idempotencyTestRouter(t, pool, os.Getenv("LIFEOS_I34_ISSUER"))
	secret := os.Getenv("LIFEOS_I35_SESSION")
	if secret == "" {
		secret = "idempotency-owner"
	}
	cookie := &http.Cookie{Name: "lifeos_session", Value: secret}
	r := httptest.NewRequest("GET", "/api/v1/me", nil)
	r.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, r)
	var me struct {
		CSRF string `json:"csrf_token"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &me) != nil || me.CSRF == "" {
		t.Fatal("worker session")
	}
	path := os.Getenv("LIFEOS_I35_PATH")
	if path == "" {
		path = "/api/v1/events"
	}
	r = httptest.NewRequest("POST", path, strings.NewReader(os.Getenv("LIFEOS_I34_BODY")))
	if tag := os.Getenv("LIFEOS_I35_TAG"); tag != "" {
		r.Header.Set("If-Match", tag)
	}
	r.AddCookie(cookie)
	r.Header.Set("Origin", moneyTestOrigin)
	r.Header.Set("X-CSRF-Token", me.CSRF)
	r.Header.Set("Idempotency-Key", os.Getenv("LIFEOS_I34_KEY"))
	r.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, r)
	if strconv.Itoa(rec.Code) != os.Getenv("LIFEOS_I34_STATUS") {
		t.Fatal(fmt.Sprintf("restart status %d: %s", rec.Code, rec.Body.String()))
	}
}
