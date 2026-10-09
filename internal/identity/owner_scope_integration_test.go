package identity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// scopeProbe is a test-only stand-in for a future owner-owned domain record.
// It deliberately has no production migration or public route.
type scopeProbeRepository struct{ pool *pgxpool.Pool }

func (repo scopeProbeRepository) read(ctx context.Context, owner OwnerID, id string) (string, error) {
	if !owner.Valid() {
		return "", ErrNotAuthorized
	}
	var value string
	err := repo.pool.QueryRow(ctx, `SELECT value FROM issue20_scope_probe
		WHERE owner_id = $1 AND id = $2`, owner.String(), id).Scan(&value)
	return value, err
}

func (repo scopeProbeRepository) update(ctx context.Context, owner OwnerID, id, value string) error {
	if !owner.Valid() {
		return ErrNotAuthorized
	}
	var updatedID string
	return repo.pool.QueryRow(ctx, `UPDATE issue20_scope_probe SET value = $3
		WHERE owner_id = $1 AND id = $2 RETURNING id`, owner.String(), id, value).Scan(&updatedID)
}

type scopeProbeService struct{ repo scopeProbeRepository }

func (service scopeProbeService) read(ctx context.Context, owner OwnerID, id string) (string, error) {
	return service.repo.read(ctx, owner, id)
}

func (service scopeProbeService) update(ctx context.Context, owner OwnerID, id, value string) error {
	if strings.TrimSpace(value) == "" {
		return ErrInvalidInput
	}
	return service.repo.update(ctx, owner, id, value)
}

type scopeProbeHandler struct{ service scopeProbeService }

func (handler scopeProbeHandler) read(w http.ResponseWriter, r *http.Request, owner OwnerID) {
	value, err := handler.service.read(r.Context(), owner, chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Read unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"value": value})
}

func (handler scopeProbeHandler) update(w http.ResponseWriter, r *http.Request, owner OwnerID) {
	var input struct {
		Value string `json:"value"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}
	if err := handler.service.update(r.Context(), owner, chi.URLParam(r, "id"), input.Value); errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
	} else if errors.Is(err, ErrInvalidInput) {
		http.Error(w, "Invalid input", http.StatusBadRequest)
	} else if err != nil {
		http.Error(w, "Write unavailable", http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func scopeProbeRequest(router http.Handler, method, id string, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/scope-probe/"+id, strings.NewReader(body))
	if cookie != nil {
		request.AddCookie(cookie)
		request.Header.Set("Origin", "http://127.0.0.1:4173")
		request.Header.Set("X-CSRF-Token", csrfToken(cookie.Value))
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func testOwnerScope(t *testing.T, pool *pgxpool.Pool, auth *Auth, owner User, ownerCookie, testerCookie *http.Cookie) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `CREATE TABLE issue20_scope_probe (
		id uuid PRIMARY KEY,
		owner_id uuid NOT NULL REFERENCES lifeos_users(id),
		value text NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	const ownerRecord = "22222222-2222-4222-8222-222222222222"
	const testerRecord = "33333333-3333-4333-8333-333333333333"
	tester, err := newRepository(pool).findByGoogleSub(ctx, "tester")
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []struct{ id, ownerID, value string }{
		{ownerRecord, owner.ID, "owner-private"},
		{testerRecord, tester.ID, "tester-private"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO issue20_scope_probe (id, owner_id, value) VALUES ($1, $2, $3)`, record.id, record.ownerID, record.value); err != nil {
			t.Fatal(err)
		}
	}
	repo := scopeProbeRepository{pool: pool}
	if _, err := repo.read(ctx, OwnerID{}, ownerRecord); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("zero owner read: %v", err)
	}
	if err := repo.update(ctx, OwnerID{}, ownerRecord, "unsafe"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("zero owner write: %v", err)
	}
	handler := scopeProbeHandler{service: scopeProbeService{repo: repo}}
	router := chi.NewRouter()
	router.MethodFunc(http.MethodGet, "/scope-probe/{id}", auth.RequireRead(handler.read))
	router.MethodFunc(http.MethodPut, "/scope-probe/{id}", auth.RequireMutation(handler.update))
	check := func(label string, want int, got *httptest.ResponseRecorder) {
		t.Helper()
		if got.Code != want {
			t.Fatalf("%s: status=%d want=%d body=%s", label, got.Code, want, got.Body.String())
		}
	}
	check("anonymous read", http.StatusUnauthorized, scopeProbeRequest(router, http.MethodGet, ownerRecord, nil, ""))
	anonymousWrite := httptest.NewRequest(http.MethodPut, "/scope-probe/"+ownerRecord, strings.NewReader(`{"value":"unsafe"}`))
	anonymousWrite.Header.Set("Origin", "http://127.0.0.1:4173")
	anonymousResponse := httptest.NewRecorder()
	router.ServeHTTP(anonymousResponse, anonymousWrite)
	check("anonymous mutation", http.StatusUnauthorized, anonymousResponse)
	ownerRead := scopeProbeRequest(router, http.MethodGet, ownerRecord, ownerCookie, "")
	check("owner read", http.StatusOK, ownerRead)
	if !strings.Contains(ownerRead.Body.String(), "owner-private") {
		t.Fatal("owner did not receive own record")
	}
	check("tester own read", http.StatusOK, scopeProbeRequest(router, http.MethodGet, testerRecord, testerCookie, ""))
	check("owner cross-user read", http.StatusNotFound, scopeProbeRequest(router, http.MethodGet, testerRecord, ownerCookie, ""))
	check("tester cross-user read", http.StatusNotFound, scopeProbeRequest(router, http.MethodGet, ownerRecord, testerCookie, ""))
	check("owner cross-user write", http.StatusNotFound, scopeProbeRequest(router, http.MethodPut, testerRecord, ownerCookie, `{"value":"unsafe"}`))
	check("tester cross-user write", http.StatusNotFound, scopeProbeRequest(router, http.MethodPut, ownerRecord, testerCookie, `{"value":"unsafe"}`))
	check("owner write", http.StatusNoContent, scopeProbeRequest(router, http.MethodPut, ownerRecord, ownerCookie, `{"value":"updated"}`))
	updated := scopeProbeRequest(router, http.MethodGet, ownerRecord, ownerCookie, "")
	check("updated own record", http.StatusOK, updated)
	if !strings.Contains(updated.Body.String(), "updated") {
		t.Fatal("own update not persisted")
	}
	check("body owner override", http.StatusBadRequest, scopeProbeRequest(router, http.MethodPut, ownerRecord, testerCookie, `{"value":"unsafe","owner_id":"`+owner.ID+`"}`))
	queryOverride := httptest.NewRequest(http.MethodGet, "/scope-probe/"+ownerRecord+"?owner_id="+owner.ID, nil)
	queryOverride.AddCookie(testerCookie)
	queryResponse := httptest.NewRecorder()
	router.ServeHTTP(queryResponse, queryOverride)
	check("query owner override", http.StatusBadRequest, queryResponse)
	headerOverride := httptest.NewRequest(http.MethodGet, "/scope-probe/"+ownerRecord, nil)
	headerOverride.AddCookie(testerCookie)
	headerOverride.Header.Set("X-LifeOS-Owner", owner.ID)
	headerResponse := httptest.NewRecorder()
	router.ServeHTTP(headerResponse, headerOverride)
	check("header owner override", http.StatusBadRequest, headerResponse)
	badCSRF := httptest.NewRequest(http.MethodPut, "/scope-probe/"+ownerRecord, strings.NewReader(`{"value":"unsafe"}`))
	badCSRF.AddCookie(ownerCookie)
	badCSRF.Header.Set("Origin", "http://127.0.0.1:4173")
	badCSRF.Header.Set("X-CSRF-Token", "invalid")
	badResponse := httptest.NewRecorder()
	router.ServeHTTP(badResponse, badCSRF)
	check("invalid CSRF", http.StatusForbidden, badResponse)
	foreign := httptest.NewRequest(http.MethodPut, "/scope-probe/"+ownerRecord, strings.NewReader(`{"value":"unsafe"}`))
	foreign.AddCookie(ownerCookie)
	foreign.Header.Set("Origin", "https://evil.example")
	foreign.Header.Set("X-CSRF-Token", csrfToken(ownerCookie.Value))
	foreignResponse := httptest.NewRecorder()
	router.ServeHTTP(foreignResponse, foreign)
	check("foreign origin", http.StatusForbidden, foreignResponse)

	revokedSecret, err := auth.createSession(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	revokedCookie := &http.Cookie{Name: sessionCookieName, Value: revokedSecret}
	if _, err := pool.Exec(ctx, `UPDATE lifeos_sessions SET revoked_at = now() WHERE token_hash = $1`, digest(revokedSecret)); err != nil {
		t.Fatal(err)
	}
	check("revoked read", http.StatusUnauthorized, scopeProbeRequest(router, http.MethodGet, ownerRecord, revokedCookie, ""))
	check("revoked write", http.StatusUnauthorized, scopeProbeRequest(router, http.MethodPut, ownerRecord, revokedCookie, `{"value":"unsafe"}`))
	expiredSecret, err := auth.createSession(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	expiredCookie := &http.Cookie{Name: sessionCookieName, Value: expiredSecret}
	if _, err := pool.Exec(ctx, `UPDATE lifeos_sessions SET created_at = now() - interval '2 days', expires_at = now() - interval '1 second' WHERE token_hash = $1`, digest(expiredSecret)); err != nil {
		t.Fatal(err)
	}
	check("expired read", http.StatusUnauthorized, scopeProbeRequest(router, http.MethodGet, ownerRecord, expiredCookie, ""))
	check("expired write", http.StatusUnauthorized, scopeProbeRequest(router, http.MethodPut, ownerRecord, expiredCookie, `{"value":"unsafe"}`))
	if _, err := pool.Exec(ctx, `UPDATE lifeos_users SET status = 'disabled' WHERE id = $1`, tester.ID); err != nil {
		t.Fatal(err)
	}
	check("disabled read", http.StatusUnauthorized, scopeProbeRequest(router, http.MethodGet, testerRecord, testerCookie, ""))
	check("disabled write", http.StatusUnauthorized, scopeProbeRequest(router, http.MethodPut, testerRecord, testerCookie, `{"value":"unsafe"}`))
	if _, err := pool.Exec(ctx, `UPDATE lifeos_users SET status = 'active' WHERE id = $1`, tester.ID); err != nil {
		t.Fatal(err)
	}
	var ownerValue, testerValue string
	if err := pool.QueryRow(ctx, `SELECT value FROM issue20_scope_probe WHERE id = $1`, ownerRecord).Scan(&ownerValue); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT value FROM issue20_scope_probe WHERE id = $1`, testerRecord).Scan(&testerValue); err != nil {
		t.Fatal(err)
	}
	if ownerValue != "updated" || testerValue != "tester-private" {
		t.Fatalf("cross-user or rejected write changed data: owner=%q tester=%q", ownerValue, testerValue)
	}
}
