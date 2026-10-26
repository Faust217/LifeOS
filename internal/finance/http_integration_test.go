package finance

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/Faust217/LifeOS/internal/platform/config"
	"github.com/Faust217/LifeOS/internal/platform/health"
	"github.com/Faust217/LifeOS/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const moneyTestOrigin = "http://127.0.0.1:4173"

func TestMoneyEventDTOUsesUTC(t *testing.T) {
	event := Event{ID: "test", Kind: "expense", AmountMinor: 100, Currency: "MYR", OccurredAt: time.Date(2026, 10, 14, 1, 20, 0, 0, time.FixedZone("MYT", 8*3600)), Status: "posted"}
	encoded, err := json.Marshal(eventDTO(event))
	if err != nil || !strings.Contains(string(encoded), `"occurred_at":"2026-10-13T17:20:00Z"`) {
		t.Fatalf("event timestamp not normalized to UTC: body=%s err=%v", encoded, err)
	}
}

type moneyTestReadiness struct{}

func (moneyTestReadiness) Check(context.Context) health.Result {
	return health.Result{Ready: true, DatabaseStatus: health.StatusReady,
		MigrationStatus: health.StatusReady, MigrationVersion: 6, HasVersion: true, ExpectedVersion: 6}
}

func moneyTestSession(t *testing.T, pool *pgxpool.Pool, userID, secret, state string) *http.Cookie {
	t.Helper()
	userStatus := "active"
	if state == "disabled" {
		userStatus = "disabled"
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO lifeos_users (id, email, status)
        VALUES ($1, $2, $3)`, userID, userID+"@example.test", userStatus); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(secret))
	created, expires, revoked := "now()", "now() + interval '1 hour'", "NULL"
	if state == "expired" {
		created, expires = "now() - interval '2 days'", "now() - interval '1 second'"
	}
	if state == "revoked" {
		revoked = "now()"
	}
	// The SQL fragments above are fixed test literals, never request input.
	sql := `INSERT INTO lifeos_sessions (token_hash, user_id, created_at, expires_at, revoked_at)
        VALUES ($1, $2, ` + created + `, ` + expires + `, ` + revoked + `)`
	if _, err := pool.Exec(context.Background(), sql, hash[:], userID); err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "lifeos_session", Value: secret}
}

func moneyHTTP(t *testing.T, router http.Handler, method, path, body string, cookie *http.Cookie, origin, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
		request.Header.Set("Idempotency-Key", newFinancialWriteKey())
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func moneyStatus(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("HTTP status=%d want=%d body=%s", response.Code, status, response.Body.String())
	}
}

func moneyError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	moneyStatus(t, response, status)
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("error content type=%q", response.Header().Get("Content-Type"))
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error.Code != code {
		t.Fatalf("error code=%q want=%q decode=%v body=%s", body.Error.Code, code, err, response.Body.String())
	}
}

func moneyDecode[T any](t *testing.T, response *httptest.ResponseRecorder, status int) T {
	t.Helper()
	moneyStatus(t, response, status)
	var value T
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestParseMinorUnit(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		amount  int64
		wantErr error
	}{
		{name: "zero", input: "0", amount: 0},
		{name: "negative opening position", input: "-500", amount: -500},
		{name: "maximum", input: "9223372036854775807", amount: 9223372036854775807},
		{name: "minimum", input: "-9223372036854775808", amount: -9223372036854775808},
		{name: "fractional", input: "1.5", wantErr: ErrInvalidInput},
		{name: "exponent", input: "1e3", wantErr: ErrInvalidInput},
		{name: "leading zero", input: "01", wantErr: ErrInvalidInput},
		{name: "plus sign", input: "+1", wantErr: ErrInvalidInput},
		{name: "empty", input: "", wantErr: ErrInvalidInput},
		{name: "positive overflow", input: "9223372036854775808", wantErr: ErrOverflow},
		{name: "negative overflow", input: "-9223372036854775809", wantErr: ErrOverflow},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			amount, err := parseMinorUnit(test.input)
			if !errors.Is(err, test.wantErr) || amount != test.amount {
				t.Fatalf("parseMinorUnit(%q) = %d, %v; want %d, %v", test.input, amount, err, test.amount, test.wantErr)
			}
		})
	}
}

func TestMoneyHTTPIntegration(t *testing.T) {
	pool, runner := financeTestDatabase(t, "LIFEOS_MONEY_API_TEST_DATABASE_URL")
	if err := runner.Up(); err != nil {
		t.Fatalf("upgrade API test database to v6: %v", err)
	}
	const ownerID = "11111111-1111-4111-8111-111111111111"
	const testerID = "22222222-2222-4222-8222-222222222222"
	ownerCookie := moneyTestSession(t, pool, ownerID, "money-api-owner-test", "active")
	testerCookie := moneyTestSession(t, pool, testerID, "money-api-tester-test", "active")
	expiredCookie := moneyTestSession(t, pool, "33333333-3333-4333-8333-333333333333", "money-api-expired-test", "expired")
	revokedCookie := moneyTestSession(t, pool, "44444444-4444-4444-8444-444444444444", "money-api-revoked-test", "revoked")
	disabledCookie := moneyTestSession(t, pool, "55555555-5555-4555-8555-555555555555", "money-api-disabled-test", "disabled")

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
	defer provider.Close()
	auth, err := identity.NewAuth(context.Background(), pool, config.AuthConfig{
		ClientID: "money-api-test", ClientSecret: "test-only", Issuer: provider.URL, PublicOrigin: moneyTestOrigin})
	if err != nil {
		t.Fatal(err)
	}
	api := chi.NewRouter()
	auth.RegisterRoutes(api)
	RegisterRoutes(api, auth, NewService(pool))
	router := httpapi.NewRouter(slog.New(slog.NewJSONHandler(io.Discard, nil)), moneyTestReadiness{}, api)
	request := func(method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
		return moneyHTTP(t, router, method, path, body, cookie, moneyTestOrigin, csrf)
	}
	csrfFor := func(cookie *http.Cookie) string {
		var user struct {
			CSRFToken string `json:"csrf_token"`
		}
		response := moneyHTTP(t, router, http.MethodGet, "/api/v1/me", "", cookie, "", "")
		moneyStatus(t, response, http.StatusOK)
		if err := json.Unmarshal(response.Body.Bytes(), &user); err != nil || user.CSRFToken == "" {
			t.Fatalf("current user: %v", err)
		}
		return user.CSRFToken
	}
	ownerCSRF, testerCSRF := csrfFor(ownerCookie), csrfFor(testerCookie)
	moneyStatus(t, moneyHTTP(t, router, http.MethodGet, "/healthz", "", nil, "", ""), http.StatusOK)
	moneyStatus(t, moneyHTTP(t, router, http.MethodGet, "/readyz", "", nil, "", ""), http.StatusOK)
	moneyStatus(t, moneyHTTP(t, router, http.MethodGet, "/api/v1/auth/start", "", nil, "", ""), http.StatusFound)
	moneyStatus(t, moneyHTTP(t, router, http.MethodGet, "/api/v1/auth/callback", "", nil, "", ""), http.StatusBadRequest)

	for _, path := range []string{"/api/v1/accounts", "/api/v1/categories", "/api/v1/events"} {
		response := request(http.MethodGet, path, "", ownerCookie, "")
		moneyStatus(t, response, http.StatusOK)
		if !strings.Contains(response.Body.String(), `"items":[]`) {
			t.Fatalf("empty list was not an array: %s", response.Body.String())
		}
	}
	moneyError(t, request(http.MethodGet, "/api/v1/accounts", "", nil, ""), http.StatusUnauthorized, "authentication_required")
	moneyError(t, request(http.MethodPost, "/api/v1/accounts", `{"name":"No session"}`, nil, ""), http.StatusUnauthorized, "authentication_required")
	for _, cookie := range []*http.Cookie{expiredCookie, revokedCookie, disabledCookie} {
		moneyError(t, request(http.MethodGet, "/api/v1/accounts", "", cookie, ""), http.StatusUnauthorized, "authentication_required")
		moneyError(t, request(http.MethodPost, "/api/v1/categories", `{"name":"Denied","kind":"expense"}`, cookie, "x"), http.StatusUnauthorized, "authentication_required")
	}
	moneyError(t, moneyHTTP(t, router, http.MethodPost, "/api/v1/categories", `{"name":"Denied","kind":"expense"}`, ownerCookie, "https://evil.example", ownerCSRF), http.StatusForbidden, "invalid_origin")
	moneyError(t, request(http.MethodPost, "/api/v1/categories", `{"name":"Denied","kind":"expense"}`, ownerCookie, "wrong"), http.StatusForbidden, "invalid_csrf")
	moneyError(t, request(http.MethodPost, "/api/v1/categories", `{"name":"Denied","kind":"expense"}`, ownerCookie, ""), http.StatusForbidden, "invalid_csrf")
	wrongType := httptest.NewRequest(http.MethodPost, "/api/v1/categories", strings.NewReader(`{"name":"Denied","kind":"expense"}`))
	wrongType.AddCookie(ownerCookie)
	wrongType.Header.Set("Origin", moneyTestOrigin)
	wrongType.Header.Set("X-CSRF-Token", ownerCSRF)
	wrongType.Header.Set("Idempotency-Key", newFinancialWriteKey())
	wrongType.Header.Set("Content-Type", "text/plain")
	wrongTypeResponse := httptest.NewRecorder()
	router.ServeHTTP(wrongTypeResponse, wrongType)
	moneyError(t, wrongTypeResponse, http.StatusUnsupportedMediaType, "unsupported_media_type")
	tooLarge := `{"name":"` + strings.Repeat("x", maxMoneyRequestBytes) + `","kind":"expense"}`
	moneyError(t, request(http.MethodPost, "/api/v1/categories", tooLarge, ownerCookie, ownerCSRF), http.StatusRequestEntityTooLarge, "request_too_large")

	if _, err := pool.Exec(context.Background(), `UPDATE lifeos_users SET time_zone='Asia/Kuala_Lumpur' WHERE id IN ($1,$2)`, ownerID, testerID); err != nil {
		t.Fatal(err)
	}
	ownerBank := moneyDecode[accountResponse](t, request(http.MethodPost, "/api/v1/accounts", `{"name":"Bank","type":"bank","currency":"MYR","opening_balance_minor":"1000","opening_balance_at":"1970-01-01T00:00:00Z"}`, ownerCookie, ownerCSRF), http.StatusCreated)
	ownerCard := moneyDecode[accountResponse](t, request(http.MethodPost, "/api/v1/accounts", `{"name":"Card","type":"credit_card","currency":"MYR","opening_balance_minor":"-500","opening_balance_at":"1970-01-01T00:00:00Z"}`, ownerCookie, ownerCSRF), http.StatusCreated)
	testerBank := moneyDecode[accountResponse](t, request(http.MethodPost, "/api/v1/accounts", `{"name":"Tester","type":"bank","currency":"MYR","opening_balance_minor":"200","opening_balance_at":"1970-01-01T00:00:00Z"}`, testerCookie, testerCSRF), http.StatusCreated)
	food := moneyDecode[categoryResponse](t, request(http.MethodPost, "/api/v1/categories", `{"name":"Food","kind":"expense"}`, ownerCookie, ownerCSRF), http.StatusCreated)
	salary := moneyDecode[categoryResponse](t, request(http.MethodPost, "/api/v1/categories", `{"name":"Salary","kind":"income"}`, ownerCookie, ownerCSRF), http.StatusCreated)
	testerFood := moneyDecode[categoryResponse](t, request(http.MethodPost, "/api/v1/categories", `{"name":"Private","kind":"expense"}`, testerCookie, testerCSRF), http.StatusCreated)
	ownerAccounts := moneyDecode[struct {
		Items []accountResponse `json:"items"`
	}](t, request(http.MethodGet, "/api/v1/accounts", "", ownerCookie, ""), http.StatusOK)
	if len(ownerAccounts.Items) != 2 {
		t.Fatalf("owner account count=%d", len(ownerAccounts.Items))
	}
	ownerCategories := moneyDecode[struct {
		Items []categoryResponse `json:"items"`
	}](t, request(http.MethodGet, "/api/v1/categories", "", ownerCookie, ""), http.StatusOK)
	if len(ownerCategories.Items) != 2 {
		t.Fatalf("owner category count=%d", len(ownerCategories.Items))
	}

	at := `"2026-10-09T12:00:00Z"`
	incomeBody := `{"kind":"income","account_id":"` + ownerBank.ID + `","amount_minor":"200","currency":"MYR","occurred_at":` + at + `,"allocations":[{"category_id":"` + salary.ID + `","amount_minor":"200"}]}`
	income := moneyDecode[eventResponse](t, request(http.MethodPost, "/api/v1/events", incomeBody, ownerCookie, ownerCSRF), http.StatusCreated)
	if income.Movements[0].AmountMinor != "200" || len(income.Allocations) != 1 {
		t.Fatalf("income response=%+v", income)
	}
	expenseBody := `{"kind":"expense","account_id":"` + ownerBank.ID + `","amount_minor":"1300","currency":"MYR","occurred_at":` + at + `,"allocations":[{"category_id":"` + food.ID + `","amount_minor":"1300"}]}`
	expense := moneyDecode[eventResponse](t, request(http.MethodPost, "/api/v1/events", expenseBody, ownerCookie, ownerCSRF), http.StatusCreated)
	if expense.Movements[0].AmountMinor != "-1300" || expense.Allocations[0].AmountMinor != "1300" {
		t.Fatalf("expense response=%+v", expense)
	}
	transferBody := `{"from_account_id":"` + ownerBank.ID + `","to_account_id":"` + ownerCard.ID + `","amount_minor":"200","currency":"MYR","occurred_at":` + at + `}`
	transfer := moneyDecode[eventResponse](t, request(http.MethodPost, "/api/v1/transfers", transferBody, ownerCookie, ownerCSRF), http.StatusCreated)
	if len(transfer.Movements) != 2 || len(transfer.Allocations) != 0 ||
		transfer.Movements[0].AmountMinor != "-200" || transfer.Movements[1].AmountMinor != "200" {
		t.Fatalf("transfer response=%+v", transfer)
	}
	balance := moneyDecode[struct {
		AmountMinor string `json:"amount_minor"`
	}](t, request(http.MethodGet, "/api/v1/accounts/"+ownerBank.ID+"/balance", "", ownerCookie, ""), http.StatusOK)
	if balance.AmountMinor != "-300" {
		t.Fatalf("bank balance=%s", balance.AmountMinor)
	}
	cardBalance := moneyDecode[struct {
		AmountMinor string `json:"amount_minor"`
	}](t, request(http.MethodGet, "/api/v1/accounts/"+ownerCard.ID+"/balance", "", ownerCookie, ""), http.StatusOK)
	if cardBalance.AmountMinor != "-300" {
		t.Fatalf("card balance=%s", cardBalance.AmountMinor)
	}
	spendingPath := "/api/v1/categories/" + food.ID + "/spending?currency=MYR"
	spending := moneyDecode[struct {
		AmountMinor string `json:"amount_minor"`
	}](t, request(http.MethodGet, spendingPath, "", ownerCookie, ""), http.StatusOK)
	if spending.AmountMinor != "1300" {
		t.Fatalf("transfer counted as spending: %s", spending.AmountMinor)
	}
	history := moneyDecode[struct {
		Items  []eventResponse `json:"items"`
		Limit  int             `json:"limit"`
		Offset int             `json:"offset"`
	}](t,
		request(http.MethodGet, "/api/v1/events?limit=2&offset=0", "", ownerCookie, ""), http.StatusOK)
	if len(history.Items) != 2 || history.Limit != 2 || history.Offset != 0 {
		t.Fatalf("history page=%+v", history)
	}
	nextPage := moneyDecode[struct {
		Items []eventResponse `json:"items"`
	}](t,
		request(http.MethodGet, "/api/v1/events?limit=2&offset=2", "", ownerCookie, ""), http.StatusOK)
	if len(nextPage.Items) != 1 {
		t.Fatalf("history second page=%+v", nextPage)
	}

	moneyError(t, request(http.MethodGet, "/api/v1/accounts/"+testerBank.ID+"/balance", "", ownerCookie, ""), http.StatusNotFound, "not_found")
	moneyError(t, request(http.MethodPost, "/api/v1/accounts/"+testerBank.ID+"/archive", "", ownerCookie, ownerCSRF), http.StatusNotFound, "not_found")
	moneyError(t, request(http.MethodPost, "/api/v1/accounts/"+testerBank.ID+"/restore", "", ownerCookie, ownerCSRF), http.StatusNotFound, "not_found")
	moneyError(t, request(http.MethodGet, "/api/v1/categories/"+testerFood.ID+"/spending?currency=MYR", "", ownerCookie, ""), http.StatusNotFound, "not_found")
	moneyError(t, request(http.MethodPost, "/api/v1/events", strings.Replace(expenseBody, ownerBank.ID, testerBank.ID, 1), ownerCookie, ownerCSRF), http.StatusNotFound, "not_found")
	moneyError(t, request(http.MethodPost, "/api/v1/events", strings.Replace(expenseBody, food.ID, testerFood.ID, 1), ownerCookie, ownerCSRF), http.StatusNotFound, "not_found")
	moneyError(t, request(http.MethodPost, "/api/v1/transfers", strings.Replace(transferBody, ownerCard.ID, testerBank.ID, 1), ownerCookie, ownerCSRF), http.StatusNotFound, "not_found")
	moneyError(t, request(http.MethodPost, "/api/v1/events/"+expense.ID+"/void", `{"reason":"No"}`, testerCookie, testerCSRF), http.StatusNotFound, "not_found")
	moneyError(t, request(http.MethodPost, "/api/v1/accounts", `{"name":"Override","type":"bank","currency":"MYR","opening_balance_minor":"0","owner_id":"`+testerID+`"}`, ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_json")
	moneyError(t, request(http.MethodGet, "/api/v1/accounts?owner_id="+testerID, "", ownerCookie, ""), http.StatusBadRequest, "owner_override")
	headerOverride := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	headerOverride.AddCookie(ownerCookie)
	headerOverride.Header.Set("X-LifeOS-Owner", testerID)
	headerResponse := httptest.NewRecorder()
	router.ServeHTTP(headerResponse, headerOverride)
	moneyError(t, headerResponse, http.StatusBadRequest, "owner_override")

	for _, bad := range []string{`{`, `{"name":"Bad","type":"bank","currency":"MYR","opening_balance_minor":1.5}`,
		`{"name":"Bad","type":"bank","currency":"MYR","opening_balance_minor":"0"} {}`,
		`{"name":"Bad","type":"bank","currency":"MYR","opening_balance_minor":"0","extra":true}`} {
		moneyError(t, request(http.MethodPost, "/api/v1/accounts", bad, ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_json")
	}
	moneyError(t, request(http.MethodPost, "/api/v1/accounts", `{"name":"Bad","type":"bank","currency":"MYR","opening_balance_minor":"1.2"}`, ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodPost, "/api/v1/accounts", `{"name":"Bad","type":"bank","currency":"MYR","opening_balance_minor":"9223372036854775808"}`, ownerCookie, ownerCSRF), http.StatusUnprocessableEntity, "amount_overflow")
	moneyError(t, request(http.MethodPost, "/api/v1/events", strings.Replace(expenseBody, `"amount_minor":"1300"`, `"amount_minor":"-1"`, 1), ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodPost, "/api/v1/events", strings.Replace(expenseBody, `"amount_minor":"1300"}`, `"amount_minor":"-1"}`, 1), ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodPost, "/api/v1/transfers", strings.Replace(transferBody, `"amount_minor":"200"`, `"amount_minor":"-1"`, 1), ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodPost, "/api/v1/transfers", strings.Replace(transferBody, `"amount_minor":"200"`, `"amount_minor":"1.5"`, 1), ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodPost, "/api/v1/events", strings.Replace(expenseBody, at, `"not-a-time"`, 1), ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_json")
	moneyError(t, request(http.MethodPost, "/api/v1/transfers", strings.Replace(transferBody, `"currency":"MYR"`, `"currency":"USD"`, 1), ownerCookie, ownerCSRF), http.StatusBadRequest, "currency_mismatch")
	moneyError(t, request(http.MethodGet, "/api/v1/accounts/bad-id/balance", "", ownerCookie, ""), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodGet, "/api/v1/events?limit=101", "", ownerCookie, ""), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodGet, "/api/v1/events?offset=-1", "", ownerCookie, ""), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodGet, "/api/v1/events?limit=2&limit=3", "", ownerCookie, ""), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodGet, "/api/v1/accounts?unknown=1", "", ownerCookie, ""), http.StatusBadRequest, "invalid_input")
	malformedQuery := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	malformedQuery.URL.RawQuery = "bad=%ZZ"
	malformedQuery.AddCookie(ownerCookie)
	malformedQueryResponse := httptest.NewRecorder()
	router.ServeHTTP(malformedQueryResponse, malformedQuery)
	moneyError(t, malformedQueryResponse, http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodPost, "/api/v1/events/"+expense.ID+"/void", `{"reason":"`+strings.Repeat("x", 501)+`"}`, ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_input")

	moneyStatus(t, request(http.MethodPost, "/api/v1/accounts/"+ownerCard.ID+"/archive", "", ownerCookie, ownerCSRF), http.StatusNoContent)
	restorePath := "/api/v1/accounts/" + ownerCard.ID + "/restore"
	moneyError(t, request(http.MethodPost, restorePath, "", nil, ""), http.StatusUnauthorized, "authentication_required")
	for _, cookie := range []*http.Cookie{expiredCookie, revokedCookie, disabledCookie} {
		moneyError(t, request(http.MethodPost, restorePath, "", cookie, "x"), http.StatusUnauthorized, "authentication_required")
	}
	moneyError(t, request(http.MethodPost, restorePath, "", ownerCookie, ""), http.StatusForbidden, "invalid_csrf")
	moneyError(t, moneyHTTP(t, router, http.MethodPost, restorePath, "", ownerCookie, "https://evil.example", ownerCSRF), http.StatusForbidden, "invalid_origin")
	moneyError(t, request(http.MethodPost, restorePath, "", testerCookie, testerCSRF), http.StatusNotFound, "not_found")
	archived := moneyDecode[struct {
		Items []accountResponse `json:"items"`
	}](t,
		request(http.MethodGet, "/api/v1/accounts", "", ownerCookie, ""), http.StatusOK)
	if archived.Items[0].ID == ownerCard.ID && archived.Items[0].ArchivedAt == nil ||
		archived.Items[1].ID == ownerCard.ID && archived.Items[1].ArchivedAt == nil {
		t.Fatal("archived account missing timestamp")
	}
	moneyError(t, request(http.MethodPost, "/api/v1/transfers", transferBody, ownerCookie, ownerCSRF), http.StatusConflict, "archived_record")
	cardBalance = moneyDecode[struct {
		AmountMinor string `json:"amount_minor"`
	}](t, request(http.MethodGet, "/api/v1/accounts/"+ownerCard.ID+"/balance", "", ownerCookie, ""), http.StatusOK)
	if cardBalance.AmountMinor != "-300" {
		t.Fatalf("archived account lost balance=%s", cardBalance.AmountMinor)
	}
	moneyStatus(t, request(http.MethodPost, restorePath, "", ownerCookie, ownerCSRF), http.StatusNoContent)
	moneyError(t, request(http.MethodPost, restorePath, "", ownerCookie, ownerCSRF), http.StatusNotFound, "not_found")
	restored := moneyDecode[struct {
		Items []accountResponse `json:"items"`
	}](t, request(http.MethodGet, "/api/v1/accounts", "", ownerCookie, ""), http.StatusOK)
	for _, item := range restored.Items {
		if item.ID == ownerCard.ID && item.ArchivedAt != nil {
			t.Fatal("restored account remains archived")
		}
	}
	cardBalance = moneyDecode[struct {
		AmountMinor string `json:"amount_minor"`
	}](t, request(http.MethodGet, "/api/v1/accounts/"+ownerCard.ID+"/balance", "", ownerCookie, ""), http.StatusOK)
	if cardBalance.AmountMinor != "-300" {
		t.Fatalf("restored account lost historical balance=%s", cardBalance.AmountMinor)
	}
	moneyStatus(t, request(http.MethodPost, "/api/v1/events/"+expense.ID+"/void", `{}`, ownerCookie, ownerCSRF), http.StatusNoContent)
	moneyError(t, request(http.MethodPost, "/api/v1/events/"+expense.ID+"/void", `{"reason":"Again"}`, ownerCookie, ownerCSRF), http.StatusConflict, "conflict")
	spending = moneyDecode[struct {
		AmountMinor string `json:"amount_minor"`
	}](t, request(http.MethodGet, spendingPath, "", ownerCookie, ""), http.StatusOK)
	if spending.AmountMinor != "0" {
		t.Fatalf("voided expense still counted: %s", spending.AmountMinor)
	}
	balance = moneyDecode[struct {
		AmountMinor string `json:"amount_minor"`
	}](t, request(http.MethodGet, "/api/v1/accounts/"+ownerBank.ID+"/balance", "", ownerCookie, ""), http.StatusOK)
	if balance.AmountMinor != "1000" {
		t.Fatalf("balance after void=%s", balance.AmountMinor)
	}
	fullHistory := moneyDecode[struct {
		Items []eventResponse `json:"items"`
	}](t, request(http.MethodGet, "/api/v1/events", "", ownerCookie, ""), http.StatusOK)
	foundVoid := false
	for _, event := range fullHistory.Items {
		if event.ID == expense.ID {
			foundVoid = event.Status == "voided" && event.VoidedAt != nil && event.VoidReason == noVoidReasonProvided && len(event.Movements) == 1 && len(event.Allocations) == 1
		}
	}
	if !foundVoid {
		t.Fatal("void removed history")
	}
	categoryPath := "/api/v1/categories/" + food.ID
	moneyError(t, request(http.MethodPost, categoryPath+"/rename", `{"name":"Medical"}`, nil, ""), http.StatusUnauthorized, "authentication_required")
	for _, cookie := range []*http.Cookie{expiredCookie, revokedCookie, disabledCookie} {
		moneyError(t, request(http.MethodPost, categoryPath+"/archive", "", cookie, "x"), http.StatusUnauthorized, "authentication_required")
	}
	moneyError(t, request(http.MethodPost, categoryPath+"/rename", `{"name":"Medical"}`, ownerCookie, ""), http.StatusForbidden, "invalid_csrf")
	moneyError(t, moneyHTTP(t, router, http.MethodPost, categoryPath+"/archive", "", ownerCookie, "https://evil.example", ownerCSRF), http.StatusForbidden, "invalid_origin")
	moneyError(t, request(http.MethodPost, categoryPath+"/rename", `{"name":"Private"}`, testerCookie, testerCSRF), http.StatusNotFound, "not_found")
	moneyError(t, request(http.MethodPost, categoryPath+"/archive", "", testerCookie, testerCSRF), http.StatusNotFound, "not_found")
	moneyError(t, request(http.MethodPost, categoryPath+"/restore", "", testerCookie, testerCSRF), http.StatusNotFound, "not_found")
	moneyError(t, request(http.MethodPost, categoryPath+"/rename", `{"name":"Uncategorized"}`, ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodPost, categoryPath+"/rename", `{"name":"Medical","kind":"income"}`, ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_json")
	renamedCategory := moneyDecode[categoryResponse](t, request(http.MethodPost, categoryPath+"/rename", `{"name":"Medical"}`, ownerCookie, ownerCSRF), http.StatusOK)
	if renamedCategory.ID != food.ID || renamedCategory.Name != "Medical" || renamedCategory.Kind != "expense" {
		t.Fatalf("renamed category=%+v", renamedCategory)
	}
	moneyError(t, request(http.MethodPost, "/api/v1/categories", `{"name":"medical","kind":"expense"}`, ownerCookie, ownerCSRF), http.StatusConflict, "conflict")
	medicalCategories := moneyDecode[struct {
		Items []categoryResponse `json:"items"`
	}](t, request(http.MethodGet, "/api/v1/categories", "", ownerCookie, ""), http.StatusOK)
	if medicalCategories.Items[0].ID != food.ID || medicalCategories.Items[0].Name != "Medical" {
		t.Fatalf("renamed category list=%+v", medicalCategories.Items)
	}
	linkedHistory := moneyDecode[struct {
		Items []eventResponse `json:"items"`
	}](t, request(http.MethodGet, "/api/v1/events", "", ownerCookie, ""), http.StatusOK)
	linked := false
	for _, item := range linkedHistory.Items {
		if item.ID == expense.ID && len(item.Allocations) == 1 && item.Allocations[0].CategoryID == food.ID {
			linked = true
		}
	}
	if !linked {
		t.Fatal("category rename lost historical allocation")
	}
	moneyStatus(t, request(http.MethodPost, categoryPath+"/archive", "", ownerCookie, ownerCSRF), http.StatusNoContent)
	archivedCategories := moneyDecode[struct {
		Items []categoryResponse `json:"items"`
	}](t, request(http.MethodGet, "/api/v1/categories", "", ownerCookie, ""), http.StatusOK)
	if archivedCategories.Items[0].ArchivedAt == nil {
		t.Fatal("archived category missing timestamp")
	}
	moneyError(t, request(http.MethodPost, "/api/v1/events", expenseBody, ownerCookie, ownerCSRF), http.StatusConflict, "archived_record")
	moneyStatus(t, request(http.MethodPost, categoryPath+"/restore", "", ownerCookie, ownerCSRF), http.StatusNoContent)
	moneyError(t, request(http.MethodPost, categoryPath+"/restore", "", ownerCookie, ownerCSRF), http.StatusNotFound, "not_found")
	for _, kind := range []string{"expense", "income"} {
		body := `{"kind":"` + kind + `","account_id":"` + ownerBank.ID + `","amount_minor":"` + map[string]string{"expense": "200", "income": "300"}[kind] + `","currency":"MYR","occurred_at":` + at + `,"allocations":[]}`
		uncategorized := moneyDecode[eventResponse](t, request(http.MethodPost, "/api/v1/events", body, ownerCookie, ownerCSRF), http.StatusCreated)
		if len(uncategorized.Allocations) != 0 || len(uncategorized.Movements) != 1 {
			t.Fatalf("uncategorized %s=%+v", kind, uncategorized)
		}
	}
	balance = moneyDecode[struct {
		AmountMinor string `json:"amount_minor"`
	}](t, request(http.MethodGet, "/api/v1/accounts/"+ownerBank.ID+"/balance", "", ownerCookie, ""), http.StatusOK)
	if balance.AmountMinor != "1100" {
		t.Fatalf("uncategorized balance=%s", balance.AmountMinor)
	}
	spending = moneyDecode[struct {
		AmountMinor string `json:"amount_minor"`
	}](t, request(http.MethodGet, spendingPath, "", ownerCookie, ""), http.StatusOK)
	if spending.AmountMinor != "0" {
		t.Fatalf("uncategorized or transfer misclassified as Medical=%s", spending.AmountMinor)
	}
	for _, variant := range []struct {
		name, body, wantReason string
	}{
		{"omitted", `{}`, noVoidReasonProvided},
		{"empty", `{"reason":""}`, noVoidReasonProvided},
		{"whitespace", `{"reason":"   "}`, noVoidReasonProvided},
		{"explicit", `{"reason":"  Entered twice  "}`, "Entered twice"},
		{"unicode limit", `{"reason":"` + strings.Repeat("药", 500) + `"}`, strings.Repeat("药", 500)},
	} {
		t.Run("void reason "+variant.name, func(t *testing.T) {
			body := `{"kind":"expense","account_id":"` + ownerBank.ID + `","amount_minor":"100","currency":"MYR","occurred_at":` + at + `,"allocations":[{"category_id":"` + food.ID + `","amount_minor":"100"}]}`
			event := moneyDecode[eventResponse](t, request(http.MethodPost, "/api/v1/events", body, ownerCookie, ownerCSRF), http.StatusCreated)
			voidPath := "/api/v1/events/" + event.ID + "/void"
			if variant.name == "omitted" {
				moneyError(t, request(http.MethodPost, voidPath, variant.body, nil, ""), http.StatusUnauthorized, "authentication_required")
				moneyError(t, request(http.MethodPost, voidPath, variant.body, testerCookie, testerCSRF), http.StatusNotFound, "not_found")
				moneyError(t, request(http.MethodPost, voidPath, variant.body, ownerCookie, ""), http.StatusForbidden, "invalid_csrf")
				moneyError(t, moneyHTTP(t, router, http.MethodPost, voidPath, variant.body, ownerCookie, "https://evil.example", ownerCSRF), http.StatusForbidden, "invalid_origin")
			}
			balance := moneyDecode[struct {
				AmountMinor string `json:"amount_minor"`
			}](t,
				request(http.MethodGet, "/api/v1/accounts/"+ownerBank.ID+"/balance", "", ownerCookie, ""), http.StatusOK)
			if balance.AmountMinor != "1000" {
				t.Fatalf("balance before void=%s", balance.AmountMinor)
			}
			spending := moneyDecode[struct {
				AmountMinor string `json:"amount_minor"`
			}](t,
				request(http.MethodGet, spendingPath, "", ownerCookie, ""), http.StatusOK)
			if spending.AmountMinor != "100" {
				t.Fatalf("spending before void=%s", spending.AmountMinor)
			}
			moneyStatus(t, request(http.MethodPost, voidPath, variant.body, ownerCookie, ownerCSRF), http.StatusNoContent)
			moneyError(t, request(http.MethodPost, voidPath, variant.body, ownerCookie, ownerCSRF), http.StatusConflict, "conflict")
			balance = moneyDecode[struct {
				AmountMinor string `json:"amount_minor"`
			}](t,
				request(http.MethodGet, "/api/v1/accounts/"+ownerBank.ID+"/balance", "", ownerCookie, ""), http.StatusOK)
			spending = moneyDecode[struct {
				AmountMinor string `json:"amount_minor"`
			}](t,
				request(http.MethodGet, spendingPath, "", ownerCookie, ""), http.StatusOK)
			if balance.AmountMinor != "1100" || spending.AmountMinor != "0" {
				t.Fatalf("after void balance=%s spending=%s", balance.AmountMinor, spending.AmountMinor)
			}
			history := moneyDecode[struct {
				Items []eventResponse `json:"items"`
			}](t,
				request(http.MethodGet, "/api/v1/events?limit=100", "", ownerCookie, ""), http.StatusOK)
			found := false
			for _, item := range history.Items {
				if item.ID == event.ID {
					found = item.Status == "voided" && item.VoidedAt != nil && item.VoidReason == variant.wantReason && len(item.Movements) == 1 && len(item.Allocations) == 1
				}
			}
			if !found {
				t.Fatalf("void history missing or changed for %s", variant.name)
			}
		})
	}
	moneyError(t, request(http.MethodPost, "/api/v1/events", strings.Replace(expenseBody, `,"allocations":[{"category_id":"`+food.ID+`","amount_minor":"1300"}]`, "", 1), ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodPost, "/api/v1/events", strings.Replace(expenseBody, `"allocations":[{"category_id":"`+food.ID+`","amount_minor":"1300"}]`, `"allocations":null`, 1), ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodPost, "/api/v1/events", strings.Replace(expenseBody, `"amount_minor":"1300"}]`, `"amount_minor":"1200"}]`, 1), ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_input")
	testerAccounts := moneyDecode[struct {
		Items []accountResponse `json:"items"`
	}](t, request(http.MethodGet, "/api/v1/accounts", "", testerCookie, ""), http.StatusOK)
	if len(testerAccounts.Items) != 1 || testerAccounts.Items[0].ID != testerBank.ID {
		t.Fatalf("tester account leak=%+v", testerAccounts.Items)
	}
	deleteBody := `{"kind":"expense","account_id":"` + ownerBank.ID + `","amount_minor":"100","currency":"MYR","occurred_at":` + at + `,"allocations":[{"category_id":"` + food.ID + `","amount_minor":"100"}]}`
	toDelete := moneyDecode[eventResponse](t, request(http.MethodPost, "/api/v1/events", deleteBody, ownerCookie, ownerCSRF), http.StatusCreated)
	deletePath := "/api/v1/events/" + toDelete.ID + "/delete"
	moneyError(t, request(http.MethodPost, deletePath, "", nil, ""), http.StatusUnauthorized, "authentication_required")
	for _, cookie := range []*http.Cookie{expiredCookie, revokedCookie, disabledCookie} {
		moneyError(t, request(http.MethodPost, deletePath, "", cookie, "x"), http.StatusUnauthorized, "authentication_required")
	}
	moneyError(t, request(http.MethodPost, deletePath, "", testerCookie, testerCSRF), http.StatusNotFound, "not_found")
	moneyError(t, request(http.MethodPost, deletePath, "", ownerCookie, ""), http.StatusForbidden, "invalid_csrf")
	moneyError(t, moneyHTTP(t, router, http.MethodPost, deletePath, "", ownerCookie, "https://evil.example", ownerCSRF), http.StatusForbidden, "invalid_origin")
	moneyError(t, request(http.MethodPost, deletePath+"?unexpected=1", "", ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodPost, deletePath, `{}`, ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodPost, "/api/v1/events/bad-id/delete", "", ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_input")
	moneyError(t, request(http.MethodPost, "/api/v1/events/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/delete", "", ownerCookie, ownerCSRF), http.StatusNotFound, "not_found")
	beforeDelete := moneyDecode[struct {
		AmountMinor string `json:"amount_minor"`
	}](t,
		request(http.MethodGet, "/api/v1/accounts/"+ownerBank.ID+"/balance", "", ownerCookie, ""), http.StatusOK)
	beforeAmount, err := strconv.ParseInt(beforeDelete.AmountMinor, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	moneyStatus(t, request(http.MethodPost, deletePath, "", ownerCookie, ownerCSRF), http.StatusNoContent)
	moneyStatus(t, request(http.MethodPost, deletePath, "", ownerCookie, ownerCSRF), http.StatusNoContent)
	afterDelete := moneyDecode[struct {
		AmountMinor string `json:"amount_minor"`
	}](t,
		request(http.MethodGet, "/api/v1/accounts/"+ownerBank.ID+"/balance", "", ownerCookie, ""), http.StatusOK)
	afterAmount, err := strconv.ParseInt(afterDelete.AmountMinor, 10, 64)
	if err != nil || afterAmount != beforeAmount+100 {
		t.Fatalf("delete balance before=%d after=%d err=%v", beforeAmount, afterAmount, err)
	}
	deletedHistory := moneyDecode[struct {
		Items []eventResponse `json:"items"`
	}](t,
		request(http.MethodGet, "/api/v1/events?limit=100", "", ownerCookie, ""), http.StatusOK)
	for _, item := range deletedHistory.Items {
		if item.ID == toDelete.ID {
			t.Fatal("deleted event visible in guarded history")
		}
	}
	var linkedMovements, linkedAllocations int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM account_movements WHERE event_id = $1 AND owner_id = $2`, toDelete.ID, ownerID).Scan(&linkedMovements); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM category_allocations WHERE event_id = $1 AND owner_id = $2`, toDelete.ID, ownerID).Scan(&linkedAllocations); err != nil {
		t.Fatal(err)
	}
	if linkedMovements != 1 || linkedAllocations != 1 {
		t.Fatalf("soft delete lost ledger links: movements=%d allocations=%d", linkedMovements, linkedAllocations)
	}
	moneyError(t, request(http.MethodPost, "/api/v1/events/"+toDelete.ID+"/void", `{}`, ownerCookie, ownerCSRF), http.StatusConflict, "conflict")
	moneyStatus(t, request(http.MethodPost, "/api/v1/auth/logout", "", ownerCookie, ownerCSRF), http.StatusNoContent)
	moneyError(t, request(http.MethodGet, "/api/v1/accounts", "", ownerCookie, ""), http.StatusUnauthorized, "authentication_required")
}
