package planning

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Faust217/LifeOS/internal/finance"
	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/Faust217/LifeOS/internal/platform/config"
	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

const goalTestOrigin = "http://127.0.0.1:4173"

func goalTestDatabase(t *testing.T, keys ...string) (*pgxpool.Pool, *migrate.Migrate) {
	t.Helper()
	key := "LIFEOS_GOALS_TEST_DATABASE_URL"
	if len(keys) > 0 {
		key = keys[0]
	}
	dsn := os.Getenv(key)
	if dsn == "" {
		t.Skipf("set %s to a fresh disposable *_test PostgreSQL database", key)
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(strings.TrimPrefix(parsed.Path, "/"), "_test") {
		t.Fatal("Goals integration DSN must name a disposable *_test database")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var existing bool
	if err := pool.QueryRow(context.Background(), `SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&existing); err != nil || existing {
		t.Fatalf("Goals test database must be empty: existing=%t err=%v", existing, err)
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
	if err := runner.Steps(4); err != nil {
		t.Fatalf("migrate fresh database to v4: %v", err)
	}
	return pool, runner
}

func goalTestAuth(t *testing.T, pool *pgxpool.Pool) *identity.Auth {
	t.Helper()
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
	t.Cleanup(provider.Close)
	auth, err := identity.NewAuth(context.Background(), pool, config.AuthConfig{
		ClientID: "goals-test", ClientSecret: "test-only", Issuer: provider.URL, PublicOrigin: goalTestOrigin,
	})
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func goalTestSession(t *testing.T, pool *pgxpool.Pool, id, state string) *http.Cookie {
	t.Helper()
	status := "active"
	if state == "disabled" {
		status = "disabled"
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO lifeos_users (id, email, status) VALUES ($1, $2, $3)`, id, id+"@example.test", status); err != nil {
		t.Fatal(err)
	}
	secret := "goals-test-" + id
	hash := sha256.Sum256([]byte(secret))
	created, expires, revoked := "now()", "now() + interval '1 hour'", "NULL"
	if state == "expired" {
		created, expires = "now() - interval '2 days'", "now() - interval '1 second'"
	}
	if state == "revoked" {
		revoked = "now()"
	}
	// Fixed test literals only; no untrusted input is interpolated into SQL.
	sql := `INSERT INTO lifeos_sessions (token_hash, user_id, created_at, expires_at, revoked_at)
		VALUES ($1, $2, ` + created + `, ` + expires + `, ` + revoked + `)`
	if _, err := pool.Exec(context.Background(), sql, hash[:], id); err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "lifeos_session", Value: secret}
}

func goalTestOwner(t *testing.T, auth *identity.Auth, cookie *http.Cookie) identity.OwnerID {
	t.Helper()
	var owner identity.OwnerID
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(cookie)
	result := httptest.NewRecorder()
	auth.RequireRead(func(w http.ResponseWriter, _ *http.Request, verified identity.OwnerID) {
		owner = verified
		w.WriteHeader(http.StatusNoContent)
	}).ServeHTTP(result, req)
	if result.Code != http.StatusNoContent || !owner.Valid() {
		t.Fatalf("verified test owner: HTTP %d", result.Code)
	}
	return owner
}

func goalTestRequest(t *testing.T, router http.Handler, method, path, body string, cookie *http.Cookie, origin, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	result := httptest.NewRecorder()
	router.ServeHTTP(result, req)
	return result
}

func goalTestStatus(t *testing.T, result *httptest.ResponseRecorder, want int) {
	t.Helper()
	if result.Code != want {
		t.Fatalf("HTTP %d want %d: %s", result.Code, want, result.Body.String())
	}
}

func goalTestError(t *testing.T, result *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	goalTestStatus(t, result, status)
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil || response.Error.Code != code {
		t.Fatalf("error=%q want=%q decode=%v", response.Error.Code, code, err)
	}
}

func goalTestDecode[T any](t *testing.T, result *httptest.ResponseRecorder, status int) T {
	t.Helper()
	goalTestStatus(t, result, status)
	var value T
	if err := json.Unmarshal(result.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestGoalsMigrationServiceAndHTTP(t *testing.T) {
	pool, runner := goalTestDatabase(t)
	ctx := context.Background()
	const ownerID = "11111111-1111-4111-8111-111111111111"
	const otherID = "22222222-2222-4222-8222-222222222222"
	ownerCookie := goalTestSession(t, pool, ownerID, "active")
	otherCookie := goalTestSession(t, pool, otherID, "active")
	expiredCookie := goalTestSession(t, pool, "33333333-3333-4333-8333-333333333333", "expired")
	revokedCookie := goalTestSession(t, pool, "44444444-4444-4444-8444-444444444444", "revoked")
	disabledCookie := goalTestSession(t, pool, "55555555-5555-4555-8555-555555555555", "disabled")
	auth := goalTestAuth(t, pool)
	owner := goalTestOwner(t, auth, ownerCookie)
	other := goalTestOwner(t, auth, otherCookie)
	financeService := finance.NewService(pool)
	bank := finance.Account{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Name: "Synthetic bank", Type: "bank", Currency: "MYR", OpeningBalanceMinor: 12345}
	_, err := pool.Exec(ctx, `INSERT INTO finance_accounts(id,owner_id,name,account_type,currency,opening_balance_minor) VALUES($1,$2,$3,'bank','MYR',12345)`, bank.ID, owner.String(), bank.Name)
	if err != nil {
		t.Fatal(err)
	}
	checkBalance := func() {
		t.Helper()
		balance, err := financeService.Balance(ctx, owner, bank.ID)
		if err != nil || balance.AmountMinor != 12345 {
			t.Fatalf("Goals affected Money: balance=%+v err=%v", balance, err)
		}
	}
	// The v4/v5 migration exercise predates the v6 Finance visibility predicate.
	// Check the persisted opening position directly until the full schema is present.
	checkStoredOpening := func() {
		t.Helper()
		var opening int64
		if err := pool.QueryRow(ctx, `SELECT opening_balance_minor FROM finance_accounts WHERE owner_id = $1 AND id = $2`, owner.String(), bank.ID).Scan(&opening); err != nil || opening != 12345 {
			t.Fatalf("Goals migration changed Money opening position=%d err=%v", opening, err)
		}
	}
	checkStoredOpening()
	if err := runner.Steps(1); err != nil {
		t.Fatalf("populated v4 to v5 migration: %v", err)
	}
	if version, dirty, err := runner.Version(); err != nil || version != 5 || dirty {
		t.Fatalf("migration version=%d dirty=%t err=%v", version, dirty, err)
	}
	checkStoredOpening()
	if err := runner.Steps(-1); err != nil {
		t.Fatalf("empty Goals v5 rollback: %v", err)
	}
	if err := runner.Steps(1); err != nil {
		t.Fatalf("v5 re-upgrade: %v", err)
	}
	checkStoredOpening()
	if err := runner.Steps(1); err != nil {
		t.Fatalf("upgrade populated Goals and Money to v6: %v", err)
	}
	if version, dirty, err := runner.Version(); err != nil || version != 6 || dirty {
		t.Fatalf("v6 migration version=%d dirty=%t err=%v", version, dirty, err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	checkBalance()

	service := NewService(pool)
	goal, err := service.CreateGoal(ctx, owner, "  Learn Go  ")
	if err != nil || goal.Title != "Learn Go" || len(goal.Items) != 0 {
		t.Fatalf("create Goal=%+v err=%v", goal, err)
	}
	if _, err := service.SetGoalCompleted(ctx, owner, goal.ID, true); !errors.Is(err, ErrConflict) {
		t.Fatalf("empty Goal completion err=%v", err)
	}
	goal, err = service.AddItem(ctx, owner, goal.ID, "Read one chapter")
	if err != nil || goal.TotalCount != 1 || goal.CompletedCount != 0 {
		t.Fatalf("add item Goal=%+v err=%v", goal, err)
	}
	itemID := goal.Items[0].ID
	foreignGoal, err := service.CreateGoal(ctx, other, "Other person's Goal")
	if err != nil {
		t.Fatal(err)
	}
	foreignGoal, err = service.AddItem(ctx, other, foreignGoal.ID, "Private step")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RenameItem(ctx, owner, goal.ID, foreignGoal.Items[0].ID, "Wrong related item"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign item under owned Goal err=%v", err)
	}
	if _, err := service.GetGoal(ctx, other, goal.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner Goal read err=%v", err)
	}
	if _, err := service.AddItem(ctx, other, goal.ID, "Foreign step"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner item create err=%v", err)
	}
	if _, err := service.RenameItem(ctx, other, goal.ID, itemID, "Foreign rename"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner item rename err=%v", err)
	}
	if _, err := service.SetItemCompleted(ctx, other, goal.ID, itemID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner item completion err=%v", err)
	}
	if _, err := service.RemoveItem(ctx, other, goal.ID, itemID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner item delete err=%v", err)
	}
	goal, err = service.RenameItem(ctx, owner, goal.ID, itemID, "Read two chapters")
	if err != nil || goal.Items[0].Title != "Read two chapters" {
		t.Fatalf("rename item Goal=%+v err=%v", goal, err)
	}
	goal, err = service.SetItemCompleted(ctx, owner, goal.ID, itemID, true)
	if err != nil || goal.CompletedCount != 1 {
		t.Fatalf("complete item Goal=%+v err=%v", goal, err)
	}
	goal, err = service.SetGoalCompleted(ctx, owner, goal.ID, true)
	if err != nil || goal.CompletedAt == nil {
		t.Fatalf("complete Goal=%+v err=%v", goal, err)
	}
	if _, err := service.AddItem(ctx, owner, goal.ID, "Cannot change completed"); !errors.Is(err, ErrConflict) {
		t.Fatalf("completed checklist edit err=%v", err)
	}
	goal, err = service.SetGoalCompleted(ctx, owner, goal.ID, false)
	if err != nil || goal.CompletedAt != nil {
		t.Fatalf("reopen Goal=%+v err=%v", goal, err)
	}
	goal, err = service.SetItemCompleted(ctx, owner, goal.ID, itemID, false)
	if err != nil || goal.CompletedCount != 0 {
		t.Fatalf("uncomplete item Goal=%+v err=%v", goal, err)
	}
	goal, err = service.RemoveItem(ctx, owner, goal.ID, itemID)
	if err != nil || goal.TotalCount != 0 {
		t.Fatalf("remove item Goal=%+v err=%v", goal, err)
	}
	goal, err = service.RenameGoal(ctx, owner, goal.ID, "Study Go")
	if err != nil || goal.Title != "Study Go" {
		t.Fatalf("rename Goal=%+v err=%v", goal, err)
	}
	goal, err = service.AddItem(ctx, owner, goal.ID, "Preserved checklist step")
	if err != nil || goal.TotalCount != 1 {
		t.Fatalf("add step before archive Goal=%+v err=%v", goal, err)
	}
	goal, err = service.ArchiveGoal(ctx, owner, goal.ID)
	if err != nil || goal.ArchivedAt == nil {
		t.Fatalf("archive Goal=%+v err=%v", goal, err)
	}
	if _, err := service.AddItem(ctx, owner, goal.ID, "Archived edit"); !errors.Is(err, ErrConflict) {
		t.Fatalf("archived checklist edit err=%v", err)
	}
	if _, err := service.RenameGoal(ctx, owner, goal.ID, "Archived rename"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived rename err=%v", err)
	}
	if _, err := service.RestoreGoal(ctx, other, goal.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner restore err=%v", err)
	}
	goal, err = service.RestoreGoal(ctx, owner, goal.ID)
	if err != nil || goal.ArchivedAt != nil || goal.Title != "Study Go" || goal.TotalCount != 1 || goal.Items[0].Title != "Preserved checklist step" {
		t.Fatalf("restored Goal=%+v err=%v", goal, err)
	}
	if _, err := service.RestoreGoal(ctx, owner, goal.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("restore active Goal err=%v", err)
	}
	checkBalance()

	// Concurrent completion and checklist insertion must serialize on the parent row.
	concurrentGoal, err := service.CreateGoal(ctx, owner, "Concurrency")
	if err != nil {
		t.Fatal(err)
	}
	concurrentGoal, err = service.AddItem(ctx, owner, concurrentGoal.ID, "Already done")
	if err != nil {
		t.Fatal(err)
	}
	concurrentGoal, err = service.SetItemCompleted(ctx, owner, concurrentGoal.ID, concurrentGoal.Items[0].ID, true)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := service.SetGoalCompleted(ctx, owner, concurrentGoal.ID, true)
		results <- err
	}()
	go func() {
		defer wg.Done()
		_, err := service.AddItem(ctx, owner, concurrentGoal.ID, "New unfinished step")
		results <- err
	}()
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for result := range results {
		if result == nil {
			success++
		} else if errors.Is(result, ErrConflict) {
			conflict++
		} else {
			t.Fatalf("concurrent change err=%v", result)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent success=%d conflict=%d", success, conflict)
	}
	concurrentGoal, err = service.GetGoal(ctx, owner, concurrentGoal.ID)
	if err != nil || (concurrentGoal.CompletedAt != nil && concurrentGoal.CompletedCount != concurrentGoal.TotalCount) {
		t.Fatalf("inconsistent concurrent Goal=%+v err=%v", concurrentGoal, err)
	}

	api := chi.NewRouter()
	auth.RegisterRoutes(api)
	RegisterRoutes(api, auth, service)
	request := func(method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
		return goalTestRequest(t, api, method, path, body, cookie, goalTestOrigin, csrf)
	}
	csrfFor := func(cookie *http.Cookie) string {
		t.Helper()
		var user struct {
			CSRFToken string `json:"csrf_token"`
		}
		response := request(http.MethodGet, "/me", "", cookie, "")
		goalTestStatus(t, response, http.StatusOK)
		if err := json.Unmarshal(response.Body.Bytes(), &user); err != nil || user.CSRFToken == "" {
			t.Fatalf("CSRF token: %v", err)
		}
		return user.CSRFToken
	}
	ownerCSRF, otherCSRF := csrfFor(ownerCookie), csrfFor(otherCookie)
	list := goalTestDecode[struct {
		Items []goalResponse `json:"items"`
	}](t, request(http.MethodGet, "/goals", "", ownerCookie, ""), http.StatusOK)
	if len(list.Items) != 2 {
		t.Fatalf("owner Goals count=%d", len(list.Items))
	}
	otherList := goalTestDecode[struct {
		Items []goalResponse `json:"items"`
	}](t, request(http.MethodGet, "/goals", "", otherCookie, ""), http.StatusOK)
	if len(otherList.Items) != 1 || otherList.Items[0].ID != foreignGoal.ID {
		t.Fatalf("other owner should see only their Goal: %+v", otherList)
	}
	goalTestError(t, request(http.MethodGet, "/goals", "", nil, ""), 401, "authentication_required")
	goalTestError(t, request(http.MethodPost, "/goals", `{"title":"No session"}`, nil, ownerCSRF), 401, "authentication_required")
	for _, cookie := range []*http.Cookie{expiredCookie, revokedCookie, disabledCookie} {
		goalTestError(t, request(http.MethodGet, "/goals", "", cookie, ""), 401, "authentication_required")
		goalTestError(t, request(http.MethodPost, "/goals", `{"title":"Denied"}`, cookie, ownerCSRF), 401, "authentication_required")
	}
	goalTestError(t, goalTestRequest(t, api, http.MethodPost, "/goals", `{"title":"Denied"}`, ownerCookie, "https://evil.example", ownerCSRF), 403, "invalid_origin")
	goalTestError(t, request(http.MethodPost, "/goals", `{"title":"Denied"}`, ownerCookie, "wrong"), 403, "invalid_csrf")
	goalTestError(t, request(http.MethodPost, "/goals", `{"title":"Denied"}`, ownerCookie, ""), 403, "invalid_csrf")
	goalTestError(t, request(http.MethodPost, "/goals", `{"title":"x","owner_id":"`+otherID+`"}`, ownerCookie, ownerCSRF), 400, "invalid_json")
	goalTestError(t, request(http.MethodPost, "/goals?owner_id="+otherID, `{"title":"x"}`, ownerCookie, ownerCSRF), 400, "owner_override")
	goalTestError(t, request(http.MethodPost, "/goals", `{"title":" "}`, ownerCookie, ownerCSRF), 400, "invalid_input")
	goalTestError(t, request(http.MethodPost, "/goals", `{"title":`, ownerCookie, ownerCSRF), 400, "invalid_json")
	goalTestError(t, request(http.MethodGet, "/goals/"+goal.ID, "", otherCookie, ""), 404, "not_found")
	goalTestError(t, request(http.MethodPost, "/goals/"+goal.ID+"/items", `{"title":"foreign"}`, otherCookie, otherCSRF), 404, "not_found")
	newGoal := goalTestDecode[goalResponse](t, request(http.MethodPost, "/goals", `{"title":"Run a 5K"}`, ownerCookie, ownerCSRF), 201)
	if newGoal.Title != "Run a 5K" {
		t.Fatalf("new Goal=%+v", newGoal)
	}
	newGoal = goalTestDecode[goalResponse](t, request(http.MethodPost, "/goals/"+newGoal.ID+"/items", `{"title":"Walk today"}`, ownerCookie, ownerCSRF), 201)
	if newGoal.TotalCount != 1 || newGoal.CompletedCount != 0 {
		t.Fatalf("new checklist=%+v", newGoal)
	}
	itemID = newGoal.Items[0].ID
	goalTestError(t, request(http.MethodPost, "/goals/"+newGoal.ID+"/items/"+itemID+"/completion", `{"completed":true}`, otherCookie, otherCSRF), 404, "not_found")
	newGoal = goalTestDecode[goalResponse](t, request(http.MethodPost, "/goals/"+newGoal.ID+"/items/"+itemID+"/completion", `{"completed":true}`, ownerCookie, ownerCSRF), 200)
	if newGoal.CompletedCount != 1 {
		t.Fatalf("completed checklist=%+v", newGoal)
	}
	newGoal = goalTestDecode[goalResponse](t, request(http.MethodPost, "/goals/"+newGoal.ID+"/completion", `{"completed":true}`, ownerCookie, ownerCSRF), 200)
	if newGoal.CompletedAt == nil {
		t.Fatalf("Goal not completed: %+v", newGoal)
	}
	newGoal = goalTestDecode[goalResponse](t, request(http.MethodPost, "/goals/"+newGoal.ID+"/completion", `{"completed":false}`, ownerCookie, ownerCSRF), 200)
	if newGoal.CompletedAt != nil {
		t.Fatalf("Goal not reopened: %+v", newGoal)
	}
	newGoal = goalTestDecode[goalResponse](t, request(http.MethodPost, "/goals/"+newGoal.ID+"/rename", `{"title":"Run this week"}`, ownerCookie, ownerCSRF), 200)
	if newGoal.Title != "Run this week" {
		t.Fatalf("Goal rename=%+v", newGoal)
	}
	newGoal = goalTestDecode[goalResponse](t, request(http.MethodPost, "/goals/"+newGoal.ID+"/items/"+itemID+"/rename", `{"title":"Walk outside"}`, ownerCookie, ownerCSRF), 200)
	if newGoal.Items[0].Title != "Walk outside" {
		t.Fatalf("item rename=%+v", newGoal)
	}
	newGoal = goalTestDecode[goalResponse](t, request(http.MethodDelete, "/goals/"+newGoal.ID+"/items/"+itemID, "", ownerCookie, ownerCSRF), 200)
	if newGoal.TotalCount != 0 {
		t.Fatalf("item removal=%+v", newGoal)
	}
	newGoal = goalTestDecode[goalResponse](t, request(http.MethodPost, "/goals/"+newGoal.ID+"/archive", "", ownerCookie, ownerCSRF), 200)
	if newGoal.ArchivedAt == nil {
		t.Fatalf("archive=%+v", newGoal)
	}
	stored := goalTestDecode[goalResponse](t, request(http.MethodGet, "/goals/"+newGoal.ID, "", ownerCookie, ""), 200)
	if stored.Title != newGoal.Title || stored.ArchivedAt == nil {
		t.Fatalf("persisted Goal=%+v", stored)
	}
	goalTestError(t, request(http.MethodPost, "/goals/"+newGoal.ID+"/restore", "", otherCookie, otherCSRF), 404, "not_found")
	goalTestError(t, request(http.MethodPost, "/goals/"+newGoal.ID+"/restore", "", ownerCookie, ""), 403, "invalid_csrf")
	newGoal = goalTestDecode[goalResponse](t, request(http.MethodPost, "/goals/"+newGoal.ID+"/restore", "", ownerCookie, ownerCSRF), 200)
	if newGoal.ArchivedAt != nil || newGoal.ID != stored.ID {
		t.Fatalf("restored via API=%+v", newGoal)
	}
	checkBalance()
	var movements int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM account_movements WHERE owner_id = $1`, owner.String()).Scan(&movements); err != nil || movements != 0 {
		t.Fatalf("Goals must not write account movements: %d %v", movements, err)
	}
	fmt.Fprint(os.Stdout, "Goals PostgreSQL migration, service, API, session, isolation, and concurrency integration passed\n")
}
