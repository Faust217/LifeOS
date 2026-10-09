package identity

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Faust217/LifeOS/internal/platform/config"
	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

type oidcFlow struct{ nonce, challenge string }

type testProvider struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	mu     sync.Mutex
	flows  map[string]oidcFlow
}

func newTestProvider(t *testing.T) *testProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &testProvider{key: key, flows: make(map[string]oidcFlow)}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": p.server.URL, "authorization_endpoint": p.server.URL + "/authorize", "token_endpoint": p.server.URL + "/token", "jwks_uri": p.server.URL + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "test-key", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(p.key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(p.key.E)).Bytes())}}})
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			code := r.FormValue("code")
			p.mu.Lock()
			flow, ok := p.flows[code]
			p.mu.Unlock()
			verifier := r.FormValue("code_verifier")
			challenge := sha256.Sum256([]byte(verifier))
			if !ok || verifier == "" || subtle.ConstantTimeCompare([]byte(flow.challenge), []byte(base64.RawURLEncoding.EncodeToString(challenge[:]))) != 1 {
				http.Error(w, "invalid PKCE verifier", http.StatusBadRequest)
				return
			}
			if code == "invalid-signature" {
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test", "token_type": "Bearer", "id_token": "not.a.jwt"})
				return
			}
			sub, email := code, code+"@example.com"
			if strings.HasPrefix(code, "concurrent-") {
				email = "concurrent@example.com"
			}
			if code == "owner-email-changed" {
				sub, email = "owner", "changed@example.com"
			}
			claims := map[string]any{"iss": p.server.URL, "aud": "lifeos-test-client", "sub": sub, "email": email, "email_verified": true, "nonce": flow.nonce, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()}
			switch code {
			case "bad-audience":
				claims["aud"] = "other-client"
			case "bad-issuer":
				claims["iss"] = "https://other.example"
			case "expired-token":
				claims["exp"] = time.Now().Add(-time.Hour).Unix()
			case "bad-nonce":
				claims["nonce"] = "wrong"
			case "unverified":
				claims["email_verified"] = false
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test", "token_type": "Bearer", "id_token": p.sign(t, claims)})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.server.Close)
	return p
}

func (p *testProvider) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "test-key", "typ": "JWT"})
	body, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body)
	hash := sha256.Sum256([]byte(input))
	signature, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func authTestDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("LIFEOS_AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set LIFEOS_AUTH_TEST_DATABASE_URL to an empty disposable *_test PostgreSQL database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(strings.TrimPrefix(parsed.Path, "/"), "_test") {
		t.Fatal("test DSN must name a disposable *_test database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var existing bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&existing); err != nil || existing {
		t.Fatalf("test database must be empty: existing=%t err=%v", existing, err)
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
	if err := runner.Steps(2); err != nil {
		t.Fatalf("upgrade empty database through identity version 2: %v", err)
	}
	const upgradeUserID = "11111111-1111-4111-8111-111111111111"
	if _, err := pool.Exec(ctx, "INSERT INTO lifeos_users (id, email) VALUES ($1, 'upgrade-fixture@example.com')", upgradeUserID); err != nil {
		t.Fatalf("seed existing identity before auth migration: %v", err)
	}
	if err := runner.Steps(1); err != nil {
		t.Fatalf("upgrade populated identity version 2 to auth version 3: %v", err)
	}
	if version, dirty, err := runner.Version(); err != nil || version != 3 || dirty {
		t.Fatalf("migration version=%d dirty=%t err=%v", version, dirty, err)
	}
	var retainedID string
	if err := pool.QueryRow(ctx, "SELECT id FROM lifeos_users WHERE id = $1", upgradeUserID).Scan(&retainedID); err != nil || retainedID != upgradeUserID {
		t.Fatalf("auth migration lost pre-existing user: id=%q err=%v", retainedID, err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM lifeos_users WHERE id = $1", upgradeUserID); err != nil {
		t.Fatalf("remove disposable upgrade fixture: %v", err)
	}
	return pool
}

func startFlow(t *testing.T, handler http.Handler, p *testProvider, code string) (*http.Cookie, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/start", nil))
	if recorder.Code != http.StatusFound {
		t.Fatalf("start status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	location, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	q := location.Query()
	if q.Get("state") == "" || q.Get("nonce") == "" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("missing OIDC defense in authorization URL: %s", location.String())
	}
	p.mu.Lock()
	p.flows[code] = oidcFlow{nonce: q.Get("nonce"), challenge: q.Get("code_challenge")}
	p.mu.Unlock()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == stateCookieName {
			return cookie, q.Get("state")
		}
	}
	t.Fatal("missing state cookie")
	return nil, ""
}

func finishFlow(handler http.Handler, stateCookie *http.Cookie, state, code string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?state="+url.QueryEscape(state)+"&code="+url.QueryEscape(code)+"&next="+url.QueryEscape("https://evil.example/redirect"), nil)
	request.AddCookie(stateCookie)
	handler.ServeHTTP(recorder, request)
	return recorder
}

func sessionFrom(t *testing.T, recorder *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			return cookie
		}
	}
	t.Fatal("missing session cookie")
	return nil
}

func withCookie(handler http.Handler, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestOIDCAndSessionIntegration(t *testing.T) {
	pool := authTestDatabase(t)
	provider := newTestProvider(t)
	auth, err := newAuth(context.Background(), pool, config.AuthConfig{ClientID: "lifeos-test-client", ClientSecret: "test-secret", Issuer: provider.server.URL, PublicOrigin: "http://127.0.0.1:4173"}, provider.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Mount("/api/v1", auth.Router())
	ctx := context.Background()
	repo := newRepository(pool)

	if got := withCookie(router, "/api/v1/me", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("anonymous current user status=%d", got)
	}
	if got := withCookie(router, "/api/v1/admin/status", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("anonymous admin status=%d", got)
	}
	for _, path := range []string{"/api/v1/auth/bootstrap", "/api/v1/admin/invitations", "/api/v1/invitations"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("operator action unexpectedly exposed at %s: status=%d", path, recorder.Code)
		}
	}
	for _, code := range []string{"bad-audience", "bad-issuer", "expired-token", "bad-nonce", "unverified", "invalid-signature"} {
		cookie, state := startFlow(t, router, provider, code)
		if got := finishFlow(router, cookie, state, code).Code; got != http.StatusUnauthorized {
			t.Errorf("invalid token %s status=%d", code, got)
		}
	}
	cookie, state := startFlow(t, router, provider, "uninvited")
	if got := finishFlow(router, cookie, state, "uninvited").Code; got != http.StatusForbidden {
		t.Fatalf("uninvited user status=%d", got)
	}

	ownerInvite, err := repo.bootstrapOwnerInvitation(ctx, operatorAuthorization{approved: true}, "owner@example.com", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	cookie, state = startFlow(t, router, provider, "owner")
	if got := finishFlow(router, cookie, "wrong-state", "owner").Code; got != http.StatusBadRequest {
		t.Fatalf("state mismatch status=%d", got)
	}
	ownerCallback := finishFlow(router, cookie, state, "owner")
	if ownerCallback.Code != http.StatusSeeOther {
		t.Fatalf("owner callback status=%d body=%s", ownerCallback.Code, ownerCallback.Body.String())
	}
	if location := ownerCallback.Header().Get("Location"); location != "http://127.0.0.1:4173/" {
		t.Fatalf("callback redirect accepted caller target: %q", location)
	}
	if got := finishFlow(router, cookie, state, "owner").Code; got != http.StatusBadRequest {
		t.Fatalf("replayed callback status=%d", got)
	}
	ownerCookie := sessionFrom(t, ownerCallback)
	if ownerCookie.HttpOnly != true || ownerCookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("session cookie lacks protections")
	}
	protected := auth.RequireMutation(func(w http.ResponseWriter, _ *http.Request, owner OwnerID) {
		w.Header().Set("X-LifeOS-Owner", owner.String())
		w.WriteHeader(http.StatusNoContent)
	})
	protectedRequest := httptest.NewRequest(http.MethodPost, "/future-write", nil)
	protectedRequest.AddCookie(ownerCookie)
	protectedRequest.Header.Set("Origin", "http://127.0.0.1:4173")
	protectedRequest.Header.Set("X-CSRF-Token", csrfToken(ownerCookie.Value))
	protectedResponse := httptest.NewRecorder()
	protected.ServeHTTP(protectedResponse, protectedRequest)
	if protectedResponse.Code != http.StatusNoContent || protectedResponse.Header().Get("X-LifeOS-Owner") == "" {
		t.Fatalf("future mutation guard denied valid owner: status=%d", protectedResponse.Code)
	}
	if got := withCookie(router, "/api/v1/admin/status", ownerCookie).Code; got != http.StatusNoContent {
		t.Fatalf("owner admin status=%d", got)
	}
	secureAuth, err := newAuth(ctx, pool, config.AuthConfig{ClientID: "lifeos-test-client", ClientSecret: "test-secret", Issuer: provider.server.URL, PublicOrigin: "https://lifeos.example.test"}, provider.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	secureRouter := chi.NewRouter()
	secureRouter.Mount("/api/v1", secureAuth.Router())
	secureStateCookie, secureState := startFlow(t, secureRouter, provider, "owner-email-changed")
	if !secureStateCookie.Secure {
		t.Fatal("HTTPS state cookie is not Secure")
	}
	secureCallback := finishFlow(secureRouter, secureStateCookie, secureState, "owner-email-changed")
	secureSession := sessionFrom(t, secureCallback)
	if secureCallback.Code != http.StatusSeeOther || !secureSession.Secure {
		t.Fatalf("HTTPS session cookie not issued securely: status=%d", secureCallback.Code)
	}
	owner, err := repo.findByGoogleSub(ctx, "owner")
	if err != nil || !owner.IsOwnerAdmin {
		t.Fatalf("owner binding=%+v err=%v", owner, err)
	}
	if ownerInvite == "" {
		t.Fatal("owner bootstrap invitation missing")
	}

	for _, email := range []string{"revoked@example.com", "tester@example.com", "concurrent@example.com"} {
		id, err := repo.createInvitation(ctx, owner.ID, email, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if email == "revoked@example.com" {
			if err := repo.revokeInvitation(ctx, owner.ID, id); err != nil {
				t.Fatal(err)
			}
		}
	}
	cookie, state = startFlow(t, router, provider, "revoked")
	if got := finishFlow(router, cookie, state, "revoked").Code; got != http.StatusForbidden {
		t.Fatalf("revoked invitation status=%d", got)
	}
	cookie, state = startFlow(t, router, provider, "tester")
	testerCallback := finishFlow(router, cookie, state, "tester")
	if testerCallback.Code != http.StatusSeeOther {
		t.Fatalf("tester callback status=%d body=%s", testerCallback.Code, testerCallback.Body.String())
	}
	testerCookie := sessionFrom(t, testerCallback)
	if got := withCookie(router, "/api/v1/admin/status", testerCookie).Code; got != http.StatusForbidden {
		t.Fatalf("non-admin access status=%d", got)
	}
	t.Run("owner-scoped authorization", func(t *testing.T) {
		testOwnerScope(t, pool, auth, owner, ownerCookie, testerCookie)
	})

	var wg sync.WaitGroup
	results := make(chan int, 2)
	for _, code := range []string{"concurrent-1", "concurrent-2"} {
		cookie, state := startFlow(t, router, provider, code)
		wg.Add(1)
		go func(code string, cookie *http.Cookie, state string) {
			defer wg.Done()
			results <- finishFlow(router, cookie, state, code).Code
		}(code, cookie, state)
	}
	wg.Wait()
	close(results)
	success, denied := 0, 0
	for status := range results {
		if status == http.StatusSeeOther {
			success++
		} else if status == http.StatusForbidden {
			denied++
		} else {
			t.Fatalf("concurrent callback status=%d", status)
		}
	}
	if success != 1 || denied != 1 {
		t.Fatalf("concurrent admission success=%d denied=%d", success, denied)
	}
	var admins int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM lifeos_users WHERE is_owner_admin").Scan(&admins); err != nil || admins != 1 {
		t.Fatalf("admin count=%d err=%v", admins, err)
	}

	me := withCookie(router, "/api/v1/me", testerCookie)
	if me.Code != http.StatusOK {
		t.Fatalf("current user status=%d", me.Code)
	}
	var current struct {
		CSRFToken string `json:"csrf_token"`
		IsAdmin   bool   `json:"is_owner_admin"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &current); err != nil || current.CSRFToken == "" || current.IsAdmin {
		t.Fatalf("current user=%+v err=%v", current, err)
	}
	postLogout := func(origin, csrf string) int {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		request.AddCookie(testerCookie)
		request.Header.Set("Origin", origin)
		request.Header.Set("X-CSRF-Token", csrf)
		router.ServeHTTP(recorder, request)
		return recorder.Code
	}
	if got := postLogout("https://evil.example", current.CSRFToken); got != http.StatusForbidden {
		t.Fatalf("foreign origin logout=%d", got)
	}
	if got := postLogout("http://127.0.0.1:4173", "wrong"); got != http.StatusForbidden {
		t.Fatalf("bad CSRF logout=%d", got)
	}
	if got := postLogout("http://127.0.0.1:4173", current.CSRFToken); got != http.StatusNoContent {
		t.Fatalf("valid logout=%d", got)
	}
	if got := withCookie(router, "/api/v1/me", testerCookie).Code; got != http.StatusUnauthorized {
		t.Fatalf("revoked session status=%d", got)
	}
	if _, err := pool.Exec(ctx, "UPDATE lifeos_sessions SET expires_at = now() - interval '1 second' WHERE token_hash = $1", digest(ownerCookie.Value)); err == nil {
		t.Fatal("database accepted expiry before creation")
	}
	if _, err := pool.Exec(ctx, "UPDATE lifeos_sessions SET created_at = created_at - interval '2 days', expires_at = now() - interval '1 second' WHERE token_hash = $1", digest(ownerCookie.Value)); err != nil {
		t.Fatal(err)
	}
	if got := withCookie(router, "/api/v1/me", ownerCookie).Code; got != http.StatusUnauthorized {
		t.Fatalf("expired session status=%d", got)
	}
	if _, err := pool.Exec(ctx, "UPDATE lifeos_users SET status = 'disabled' WHERE id = $1", owner.ID); err != nil {
		t.Fatal(err)
	}
	if got := withCookie(secureRouter, "/api/v1/me", secureSession).Code; got != http.StatusUnauthorized {
		t.Fatalf("disabled user current-user status=%d", got)
	}
	if got := withCookie(secureRouter, "/api/v1/admin/status", secureSession).Code; got != http.StatusUnauthorized {
		t.Fatalf("disabled owner admin status=%d", got)
	}
	cookie, state = startFlow(t, router, provider, "owner")
	if got := finishFlow(router, cookie, state, "owner").Code; got != http.StatusForbidden {
		t.Fatalf("disabled user sign-in status=%d", got)
	}
	path, err := filepath.Abs("../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	dsn := os.Getenv("LIFEOS_AUTH_TEST_DATABASE_URL")
	runner, err := migrate.New((&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if err := runner.Steps(-1); err == nil {
		t.Fatal("down migration removed authentication history")
	}
	var remaining int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM lifeos_sessions").Scan(&remaining); err != nil || remaining == 0 {
		t.Fatalf("auth history lost after refused down migration: rows=%d err=%v", remaining, err)
	}
}
