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
	"strings"
	"testing"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/Faust217/LifeOS/internal/platform/config"
	"github.com/Faust217/LifeOS/internal/platform/health"
	"github.com/Faust217/LifeOS/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const moneyTestOrigin = "http://127.0.0.1:4173"

type moneyTestReadiness struct{}

func (moneyTestReadiness) Check(context.Context) health.Result {
	return health.Result{Ready: true, DatabaseStatus: health.StatusReady,
		MigrationStatus: health.StatusReady, MigrationVersion: 4, HasVersion: true, ExpectedVersion: 4}
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
	if err := runner.Steps(1); err != nil {
		t.Fatalf("upgrade API test database to v4: %v", err)
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
	wrongType.Header.Set("Content-Type", "text/plain")
	wrongTypeResponse := httptest.NewRecorder()
	router.ServeHTTP(wrongTypeResponse, wrongType)
	moneyError(t, wrongTypeResponse, http.StatusUnsupportedMediaType, "unsupported_media_type")
	tooLarge := `{"name":"` + strings.Repeat("x", maxMoneyRequestBytes) + `","kind":"expense"}`
	moneyError(t, request(http.MethodPost, "/api/v1/categories", tooLarge, ownerCookie, ownerCSRF), http.StatusRequestEntityTooLarge, "request_too_large")

	ownerBank := moneyDecode[accountResponse](t, request(http.MethodPost, "/api/v1/accounts", `{"name":"Bank","type":"bank","currency":"MYR","opening_balance_minor":"1000"}`, ownerCookie, ownerCSRF), http.StatusCreated)
	ownerCard := moneyDecode[accountResponse](t, request(http.MethodPost, "/api/v1/accounts", `{"name":"Card","type":"credit_card","currency":"MYR","opening_balance_minor":"-500"}`, ownerCookie, ownerCSRF), http.StatusCreated)
	testerBank := moneyDecode[accountResponse](t, request(http.MethodPost, "/api/v1/accounts", `{"name":"Tester","type":"bank","currency":"MYR","opening_balance_minor":"200"}`, testerCookie, testerCSRF), http.StatusCreated)
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
	moneyError(t, request(http.MethodPost, "/api/v1/events/"+expense.ID+"/void", `{"reason":"  "}`, ownerCookie, ownerCSRF), http.StatusBadRequest, "invalid_input")

	moneyStatus(t, request(http.MethodPost, "/api/v1/accounts/"+ownerCard.ID+"/archive", "", ownerCookie, ownerCSRF), http.StatusNoContent)
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
	moneyStatus(t, request(http.MethodPost, "/api/v1/events/"+expense.ID+"/void", `{"reason":"Duplicate"}`, ownerCookie, ownerCSRF), http.StatusNoContent)
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
			foundVoid = event.Status == "voided" && event.VoidedAt != nil && len(event.Movements) == 1 && len(event.Allocations) == 1
		}
	}
	if !foundVoid {
		t.Fatal("void removed history")
	}
	testerAccounts := moneyDecode[struct {
		Items []accountResponse `json:"items"`
	}](t, request(http.MethodGet, "/api/v1/accounts", "", testerCookie, ""), http.StatusOK)
	if len(testerAccounts.Items) != 1 || testerAccounts.Items[0].ID != testerBank.ID {
		t.Fatalf("tester account leak=%+v", testerAccounts.Items)
	}
	moneyStatus(t, request(http.MethodPost, "/api/v1/auth/logout", "", ownerCookie, ownerCSRF), http.StatusNoContent)
	moneyError(t, request(http.MethodGet, "/api/v1/accounts", "", ownerCookie, ""), http.StatusUnauthorized, "authentication_required")
}
