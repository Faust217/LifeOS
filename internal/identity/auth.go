package identity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Faust217/LifeOS/internal/platform/config"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
)

const (
	stateCookieName   = "lifeos_oidc_state"
	sessionCookieName = "lifeos_session"
	attemptLifetime   = 5 * time.Minute
	sessionLifetime   = 24 * time.Hour
)

// Auth is the server-side OIDC and session boundary. Its repository and verified
// identity values stay private to this package.
type Auth struct {
	pool         *pgxpool.Pool
	repo         *repository
	client       *http.Client
	oauth        oauth2.Config
	verifier     *oidc.IDTokenVerifier
	publicOrigin string
	secureCookie bool
}

// NewAuth discovers the configured provider and prepares the code flow. Empty
// client credentials disable sign-in in local Foundation mode.
func NewAuth(ctx context.Context, pool *pgxpool.Pool, cfg config.AuthConfig) (*Auth, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, nil
	}
	return newAuth(ctx, pool, cfg, &http.Client{Timeout: 10 * time.Second})
}

func newAuth(ctx context.Context, pool *pgxpool.Pool, cfg config.AuthConfig, client *http.Client) (*Auth, error) {
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, client), cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	return &Auth{
		pool:   pool,
		repo:   newRepository(pool),
		client: client,
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.PublicOrigin + "/api/v1/auth/callback",
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "email"},
		},
		verifier:     provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		publicOrigin: cfg.PublicOrigin,
		secureCookie: strings.HasPrefix(cfg.PublicOrigin, "https://"),
	}, nil
}

// Router exposes only the small authentication and current-user surface.
func (a *Auth) Router() http.Handler {
	router := chi.NewRouter()
	a.RegisterRoutes(router)
	return router
}

// RegisterRoutes adds identity endpoints to a shared /api/v1 router.
func (a *Auth) RegisterRoutes(router chi.Router) {
	router.Get("/auth/start", a.start)
	router.Get("/auth/callback", a.callback)
	router.Post("/auth/logout", a.logout)
	router.Get("/me", a.me)
	router.Get("/admin/status", a.adminStatus)
}

func (a *Auth) start(w http.ResponseWriter, r *http.Request) {
	state, err := randomToken()
	if err != nil {
		http.Error(w, "Sign-in unavailable", http.StatusServiceUnavailable)
		return
	}
	nonce, err := randomToken()
	if err != nil {
		http.Error(w, "Sign-in unavailable", http.StatusServiceUnavailable)
		return
	}
	verifier := oauth2.GenerateVerifier()
	_, err = a.pool.Exec(r.Context(), `INSERT INTO oidc_attempts (state_hash, nonce_hash, pkce_verifier, expires_at)
		VALUES ($1, $2, $3, now() + interval '5 minutes')`, digest(state), digest(nonce), verifier)
	if err != nil {
		http.Error(w, "Sign-in unavailable", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookieName, Value: state, Path: "/api/v1/auth/callback", MaxAge: int(attemptLifetime.Seconds()), HttpOnly: true, Secure: a.secureCookie, SameSite: http.SameSiteLaxMode})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, a.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("nonce", nonce)), http.StatusFound)
}

func (a *Auth) callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	stateCookie, err := r.Cookie(stateCookieName)
	clearCookie(w, stateCookieName, "/api/v1/auth/callback", a.secureCookie)
	state := r.URL.Query().Get("state")
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(state)) != 1 {
		http.Error(w, "Invalid sign-in attempt", http.StatusBadRequest)
		return
	}
	var nonceHash []byte
	var pkceVerifier string
	err = a.pool.QueryRow(r.Context(), `UPDATE oidc_attempts SET consumed_at = now()
		WHERE state_hash = $1 AND consumed_at IS NULL AND expires_at > now()
		RETURNING nonce_hash, pkce_verifier`, digest(state)).Scan(&nonceHash, &pkceVerifier)
	if err != nil {
		http.Error(w, "Sign-in attempt expired or used", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" || r.URL.Query().Get("error") != "" {
		http.Error(w, "Sign-in failed", http.StatusUnauthorized)
		return
	}
	ctx := oidc.ClientContext(r.Context(), a.client)
	token, err := a.oauth.Exchange(ctx, code, oauth2.VerifierOption(pkceVerifier))
	if err != nil {
		http.Error(w, "Sign-in failed", http.StatusUnauthorized)
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		http.Error(w, "Sign-in failed", http.StatusUnauthorized)
		return
	}
	idToken, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		http.Error(w, "Sign-in failed", http.StatusUnauthorized)
		return
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Nonce         string `json:"nonce"`
	}
	if err := idToken.Claims(&claims); err != nil || subtle.ConstantTimeCompare(nonceHash, digest(claims.Nonce)) != 1 || !claims.EmailVerified {
		http.Error(w, "Sign-in failed", http.StatusUnauthorized)
		return
	}
	identity := verifiedGoogleIdentity{sub: idToken.Subject, email: claims.Email, emailVerified: true}
	user, err := a.admitVerifiedIdentity(ctx, identity)
	if err != nil {
		if errors.Is(err, ErrInvitationUnavailable) || errors.Is(err, ErrConflict) || errors.Is(err, ErrNotAuthorized) || errors.Is(err, ErrInvalidInput) {
			http.Error(w, "Access denied", http.StatusForbidden)
		} else {
			http.Error(w, "Sign-in unavailable", http.StatusServiceUnavailable)
		}
		return
	}
	secret, err := a.createSession(ctx, user.ID)
	if err != nil {
		http.Error(w, "Sign-in unavailable", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: secret, Path: "/", MaxAge: int(sessionLifetime.Seconds()), HttpOnly: true, Secure: a.secureCookie, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, a.publicOrigin+"/", http.StatusSeeOther)
}

// admitVerifiedIdentity is the service gate between verified provider claims and
// private identity persistence. Email is used only for initial invitation lookup.
func (a *Auth) admitVerifiedIdentity(ctx context.Context, identity verifiedGoogleIdentity) (User, error) {
	user, err := a.repo.findByGoogleSub(ctx, identity.sub)
	if errors.Is(err, pgx.ErrNoRows) {
		invitationID, findErr := a.repo.findPendingInvitationByEmail(ctx, identity.email)
		if errors.Is(findErr, pgx.ErrNoRows) {
			return User{}, ErrInvitationUnavailable
		}
		if findErr != nil {
			return User{}, findErr
		}
		user, err = a.repo.acceptInvitation(ctx, invitationID, identity)
	}
	if err != nil {
		return User{}, err
	}
	if user.Status != "active" {
		return User{}, ErrNotAuthorized
	}
	return user, nil
}

func (a *Auth) me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user, secret, err := a.currentUser(r)
	if err != nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		ID           string `json:"id"`
		Email        string `json:"email"`
		IsOwnerAdmin bool   `json:"is_owner_admin"`
		CSRFToken    string `json:"csrf_token"`
	}{user.ID, user.Email, user.IsOwnerAdmin, csrfToken(secret)})
}

func (a *Auth) adminStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user, _, err := a.currentUser(r)
	if err != nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	if !user.IsOwnerAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	_, secret, ok := a.authorizeMutation(w, r)
	if !ok {
		return
	}
	_, err := a.pool.Exec(r.Context(), "UPDATE lifeos_sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL", digest(secret))
	if err != nil {
		http.Error(w, "Sign-out unavailable", http.StatusServiceUnavailable)
		return
	}
	clearCookie(w, sessionCookieName, "/", a.secureCookie)
	w.WriteHeader(http.StatusNoContent)
}

// authorizeMutation is the shared guard for future cookie-authenticated writes.
// Every POST/PATCH/PUT/DELETE handler must check exact Origin, active session,
// and the session-bound CSRF token before changing user-owned state.
func (a *Auth) authorizeMutation(w http.ResponseWriter, r *http.Request) (User, string, bool) {
	if r.Header.Get("Origin") != a.publicOrigin {
		writeGuardError(w, http.StatusForbidden, "invalid_origin", "Invalid origin")
		return User{}, "", false
	}
	user, secret, err := a.currentUser(r)
	if err != nil {
		writeSessionError(w, err)
		return User{}, "", false
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrfToken(secret))) != 1 {
		writeGuardError(w, http.StatusForbidden, "invalid_csrf", "Invalid CSRF token")
		return User{}, "", false
	}
	return user, secret, true
}

// RequireRead lets future modules receive a server-verified owner for GET or
// HEAD handlers. It validates the current session and user on every request.
func (a *Auth) RequireRead(next func(http.ResponseWriter, *http.Request, OwnerID)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeGuardError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
			return
		}
		user, _, err := a.currentUser(r)
		if err != nil {
			writeSessionError(w, err)
			return
		}
		if clientOwnerOverride(r) {
			writeGuardError(w, http.StatusBadRequest, "owner_override", "Owner override is not accepted")
			return
		}
		next(w, r, OwnerID{value: user.ID})
	}
}

// RequireMutation reuses the existing Origin/session/CSRF guard and passes
// only the server-verified owner ID. Handlers must validate their domain inputs
// and repositories must include both owner_id and record ID in SQL predicates.
func (a *Auth) RequireMutation(next func(http.ResponseWriter, *http.Request, OwnerID)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch && r.Method != http.MethodDelete {
			writeGuardError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
			return
		}
		user, _, ok := a.authorizeMutation(w, r)
		if !ok {
			return
		}
		if clientOwnerOverride(r) {
			writeGuardError(w, http.StatusBadRequest, "owner_override", "Owner override is not accepted")
			return
		}
		next(w, r, OwnerID{value: user.ID})
	}
}

func writeSessionError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotAuthorized) || errors.Is(err, pgx.ErrNoRows) {
		writeGuardError(w, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	writeGuardError(w, http.StatusServiceUnavailable, "session_unavailable", "Session validation unavailable")
}

func writeGuardError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func clientOwnerOverride(r *http.Request) bool {
	return r.URL.Query().Has("owner_id") || r.Header.Get("X-LifeOS-Owner") != ""
}

func (a *Auth) createSession(ctx context.Context, userID string) (string, error) {
	secret, err := randomToken()
	if err != nil {
		return "", err
	}
	_, err = a.pool.Exec(ctx, `INSERT INTO lifeos_sessions (token_hash, user_id, expires_at)
		VALUES ($1, $2, now() + interval '24 hours')`, digest(secret), userID)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return secret, nil
}

func (a *Auth) currentUser(r *http.Request) (User, string, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return User{}, "", ErrNotAuthorized
	}
	var user User
	err = a.pool.QueryRow(r.Context(), `SELECT u.id, u.email, u.status, u.is_owner_admin
		FROM lifeos_sessions s JOIN lifeos_users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now()
		AND u.status = 'active'`, digest(cookie.Value)).Scan(&user.ID, &user.Email, &user.Status, &user.IsOwnerAdmin)
	if err != nil {
		return User{}, "", err
	}
	return user, cookie.Value, nil
}

func randomToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func digest(value string) []byte { sum := sha256.Sum256([]byte(value)); return sum[:] }

func csrfToken(sessionSecret string) string {
	mac := hmac.New(sha256.New, []byte(sessionSecret))
	_, _ = mac.Write([]byte("lifeos-csrf-v1"))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func clearCookie(w http.ResponseWriter, name, path string, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: name, Path: path, MaxAge: -1, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
}
