package finance

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/go-chi/chi/v5"
)

const maxMoneyRequestBytes = 16 << 10

var minorUnitPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

type moneyHandler struct{ service *Service }

// RegisterRoutes adds only authenticated Finance routes to the shared /api/v1 router.
func RegisterRoutes(router chi.Router, auth *identity.Auth, service *Service) {
	handler := moneyHandler{service: service}
	router.MethodFunc(http.MethodGet, "/accounts", auth.RequireRead(handler.listAccounts))
	router.MethodFunc(http.MethodPost, "/accounts", auth.RequireMutation(handler.createAccount))
	router.MethodFunc(http.MethodGet, "/accounts/{accountID}/balance", auth.RequireRead(handler.balance))
	router.MethodFunc(http.MethodPost, "/accounts/{accountID}/archive", auth.RequireMutation(handler.archiveAccount))
	router.MethodFunc(http.MethodGet, "/categories", auth.RequireRead(handler.listCategories))
	router.MethodFunc(http.MethodPost, "/categories", auth.RequireMutation(handler.createCategory))
	router.MethodFunc(http.MethodGet, "/categories/{categoryID}/spending", auth.RequireRead(handler.categorySpending))
	router.MethodFunc(http.MethodGet, "/events", auth.RequireRead(handler.history))
	router.MethodFunc(http.MethodPost, "/events", auth.RequireMutation(handler.postEvent))
	router.MethodFunc(http.MethodPost, "/transfers", auth.RequireMutation(handler.transfer))
	router.MethodFunc(http.MethodPost, "/events/{eventID}/void", auth.RequireMutation(handler.voidEvent))
}

type accountRequest struct {
	Name                string `json:"name"`
	Type                string `json:"type"`
	Currency            string `json:"currency"`
	OpeningBalanceMinor string `json:"opening_balance_minor"`
}

type accountResponse struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Type                string     `json:"type"`
	Currency            string     `json:"currency"`
	OpeningBalanceMinor string     `json:"opening_balance_minor"`
	ArchivedAt          *time.Time `json:"archived_at,omitempty"`
}

type categoryRequest struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type categoryResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
}

type allocationRequest struct {
	CategoryID  string `json:"category_id"`
	AmountMinor string `json:"amount_minor"`
}

type eventRequest struct {
	Kind        string              `json:"kind"`
	AccountID   string              `json:"account_id"`
	AmountMinor string              `json:"amount_minor"`
	Currency    string              `json:"currency"`
	OccurredAt  time.Time           `json:"occurred_at"`
	Note        string              `json:"note"`
	Allocations []allocationRequest `json:"allocations"`
}

type transferRequest struct {
	FromAccountID string    `json:"from_account_id"`
	ToAccountID   string    `json:"to_account_id"`
	AmountMinor   string    `json:"amount_minor"`
	Currency      string    `json:"currency"`
	OccurredAt    time.Time `json:"occurred_at"`
	Note          string    `json:"note"`
}

type voidRequest struct {
	Reason string `json:"reason"`
}

type movementResponse struct {
	ID          string `json:"id"`
	AccountID   string `json:"account_id"`
	AmountMinor string `json:"amount_minor"`
	Currency    string `json:"currency"`
}

type allocationResponse struct {
	ID          string `json:"id"`
	CategoryID  string `json:"category_id"`
	AmountMinor string `json:"amount_minor"`
	Currency    string `json:"currency"`
}

type eventResponse struct {
	ID          string               `json:"id"`
	Kind        string               `json:"kind"`
	AmountMinor string               `json:"amount_minor"`
	Currency    string               `json:"currency"`
	OccurredAt  time.Time            `json:"occurred_at"`
	Note        string               `json:"note"`
	Status      string               `json:"status"`
	VoidedAt    *time.Time           `json:"voided_at,omitempty"`
	VoidReason  string               `json:"void_reason,omitempty"`
	Movements   []movementResponse   `json:"movements"`
	Allocations []allocationResponse `json:"allocations"`
}

func accountDTO(account Account) accountResponse {
	return accountResponse{ID: account.ID, Name: account.Name, Type: account.Type, Currency: account.Currency,
		OpeningBalanceMinor: strconv.FormatInt(account.OpeningBalanceMinor, 10), ArchivedAt: account.ArchivedAt}
}

func categoryDTO(category Category) categoryResponse {
	return categoryResponse{ID: category.ID, Name: category.Name, Kind: category.Kind, ArchivedAt: category.ArchivedAt}
}

func eventDTO(event Event) eventResponse {
	result := eventResponse{ID: event.ID, Kind: event.Kind, AmountMinor: strconv.FormatInt(event.AmountMinor, 10),
		Currency: event.Currency, OccurredAt: event.OccurredAt, Note: event.Note, Status: event.Status,
		VoidedAt: event.VoidedAt, VoidReason: event.VoidReason,
		Movements:   make([]movementResponse, 0, len(event.Movements)),
		Allocations: make([]allocationResponse, 0, len(event.Allocations))}
	for _, movement := range event.Movements {
		result.Movements = append(result.Movements, movementResponse{ID: movement.ID, AccountID: movement.AccountID,
			AmountMinor: strconv.FormatInt(movement.AmountMinor, 10), Currency: movement.Currency})
	}
	for _, allocation := range event.Allocations {
		result.Allocations = append(result.Allocations, allocationResponse{ID: allocation.ID, CategoryID: allocation.CategoryID,
			AmountMinor: strconv.FormatInt(allocation.AmountMinor, 10), Currency: allocation.Currency})
	}
	return result
}

func (h moneyHandler) createAccount(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	var input accountRequest
	if !decodeMoneyJSON(w, r, &input) {
		return
	}
	opening, err := parseMinorUnit(input.OpeningBalanceMinor)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	account, err := h.service.CreateAccount(r.Context(), owner, AccountInput{
		Name: input.Name, Type: input.Type, Currency: input.Currency, OpeningBalanceMinor: opening})
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	writeMoneyJSON(w, http.StatusCreated, accountDTO(account))
}

func (h moneyHandler) listAccounts(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	accounts, err := h.service.ListAccounts(r.Context(), owner)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	items := make([]accountResponse, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, accountDTO(account))
	}
	writeMoneyJSON(w, http.StatusOK, struct {
		Items []accountResponse `json:"items"`
	}{Items: items})
}

func (h moneyHandler) balance(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	balance, err := h.service.Balance(r.Context(), owner, chi.URLParam(r, "accountID"))
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	writeMoneyJSON(w, http.StatusOK, struct {
		AccountID   string `json:"account_id"`
		Currency    string `json:"currency"`
		AmountMinor string `json:"amount_minor"`
	}{balance.AccountID, balance.Currency, strconv.FormatInt(balance.AmountMinor, 10)})
}

func (h moneyHandler) archiveAccount(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	if !emptyMoneyBody(w, r) {
		return
	}
	if err := h.service.ArchiveAccount(r.Context(), owner, chi.URLParam(r, "accountID")); err != nil {
		writeMoneyError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h moneyHandler) createCategory(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	var input categoryRequest
	if !decodeMoneyJSON(w, r, &input) {
		return
	}
	category, err := h.service.CreateCategory(r.Context(), owner, CategoryInput{Name: input.Name, Kind: input.Kind})
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	writeMoneyJSON(w, http.StatusCreated, categoryDTO(category))
}

func (h moneyHandler) listCategories(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	categories, err := h.service.ListCategories(r.Context(), owner)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	items := make([]categoryResponse, 0, len(categories))
	for _, category := range categories {
		items = append(items, categoryDTO(category))
	}
	writeMoneyJSON(w, http.StatusOK, struct {
		Items []categoryResponse `json:"items"`
	}{Items: items})
}

func (h moneyHandler) categorySpending(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	query, err := moneyQuery(r, "currency")
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	currency := query.Get("currency")
	amount, err := h.service.CategorySpending(r.Context(), owner, chi.URLParam(r, "categoryID"), currency)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	writeMoneyJSON(w, http.StatusOK, struct {
		CategoryID  string `json:"category_id"`
		Currency    string `json:"currency"`
		AmountMinor string `json:"amount_minor"`
	}{chi.URLParam(r, "categoryID"), currency, strconv.FormatInt(amount, 10)})
}

func (h moneyHandler) postEvent(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	var input eventRequest
	if !decodeMoneyJSON(w, r, &input) {
		return
	}
	amount, err := parseMinorUnit(input.AmountMinor)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	allocations := make([]AllocationInput, 0, len(input.Allocations))
	for _, allocation := range input.Allocations {
		part, err := parseMinorUnit(allocation.AmountMinor)
		if err != nil {
			writeMoneyError(w, err)
			return
		}
		allocations = append(allocations, AllocationInput{CategoryID: allocation.CategoryID, AmountMinor: part})
	}
	eventInput := EventInput{AccountID: input.AccountID, AmountMinor: amount, Currency: input.Currency,
		OccurredAt: input.OccurredAt, Note: input.Note, Allocations: allocations}
	var event Event
	switch input.Kind {
	case "expense":
		event, err = h.service.PostExpense(r.Context(), owner, eventInput)
	case "income":
		event, err = h.service.PostIncome(r.Context(), owner, eventInput)
	default:
		writeMoneyError(w, ErrInvalidInput)
		return
	}
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	writeMoneyJSON(w, http.StatusCreated, eventDTO(event))
}

func (h moneyHandler) transfer(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	var input transferRequest
	if !decodeMoneyJSON(w, r, &input) {
		return
	}
	amount, err := parseMinorUnit(input.AmountMinor)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	event, err := h.service.Transfer(r.Context(), owner, TransferInput{FromAccountID: input.FromAccountID,
		ToAccountID: input.ToAccountID, AmountMinor: amount, Currency: input.Currency,
		OccurredAt: input.OccurredAt, Note: input.Note})
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	writeMoneyJSON(w, http.StatusCreated, eventDTO(event))
}

func (h moneyHandler) history(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	query, err := moneyQuery(r, "limit", "offset")
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	limit, offset := 20, 0
	if query.Has("limit") {
		limit, err = strconv.Atoi(query.Get("limit"))
		if err != nil {
			writeMoneyError(w, ErrInvalidInput)
			return
		}
	}
	if query.Has("offset") {
		offset, err = strconv.Atoi(query.Get("offset"))
		if err != nil {
			writeMoneyError(w, ErrInvalidInput)
			return
		}
	}
	events, err := h.service.History(r.Context(), owner, limit, offset)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	items := make([]eventResponse, 0, len(events))
	for _, event := range events {
		items = append(items, eventDTO(event))
	}
	writeMoneyJSON(w, http.StatusOK, struct {
		Items  []eventResponse `json:"items"`
		Limit  int             `json:"limit"`
		Offset int             `json:"offset"`
	}{items, limit, offset})
}

func (h moneyHandler) voidEvent(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	var input voidRequest
	if !decodeMoneyJSON(w, r, &input) {
		return
	}
	if err := h.service.VoidEvent(r.Context(), owner, chi.URLParam(r, "eventID"), input.Reason); err != nil {
		writeMoneyError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func moneyQuery(r *http.Request, allowed ...string) (url.Values, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, ErrInvalidInput
	}
	names := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		names[name] = true
	}
	for name, entries := range values {
		if !names[name] || len(entries) != 1 {
			return nil, ErrInvalidInput
		}
	}
	return values, nil
}

func parseMinorUnit(text string) (int64, error) {
	if !minorUnitPattern.MatchString(text) {
		return 0, ErrInvalidInput
	}
	amount, err := strconv.ParseInt(text, 10, 64)
	if errors.Is(err, strconv.ErrRange) {
		return 0, ErrOverflow
	}
	if err != nil {
		return 0, ErrInvalidInput
	}
	return amount, nil
}

func decodeMoneyJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeMoneyErrorStatus(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMoneyRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeMoneyErrorStatus(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request body is too large")
		} else {
			writeMoneyErrorStatus(w, http.StatusBadRequest, "invalid_json", "Invalid JSON request body")
		}
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeMoneyErrorStatus(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request body is too large")
		} else {
			writeMoneyErrorStatus(w, http.StatusBadRequest, "invalid_json", "Invalid JSON request body")
		}
		return false
	}
	return true
}

func emptyMoneyBody(w http.ResponseWriter, r *http.Request) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	if err != nil || len(body) != 0 {
		writeMoneyError(w, ErrInvalidInput)
		return false
	}
	return true
}

func writeMoneyJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeMoneyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		writeMoneyErrorStatus(w, http.StatusBadRequest, "invalid_input", "Invalid Money input")
	case errors.Is(err, ErrCurrencyMismatch):
		writeMoneyErrorStatus(w, http.StatusBadRequest, "currency_mismatch", "Account currency does not match")
	case errors.Is(err, ErrNotFound):
		writeMoneyErrorStatus(w, http.StatusNotFound, "not_found", "Money record not found")
	case errors.Is(err, ErrArchived):
		writeMoneyErrorStatus(w, http.StatusConflict, "archived_record", "Account or category is archived")
	case errors.Is(err, ErrConflict):
		writeMoneyErrorStatus(w, http.StatusConflict, "conflict", "Money event cannot be changed")
	case errors.Is(err, ErrOverflow):
		writeMoneyErrorStatus(w, http.StatusUnprocessableEntity, "amount_overflow", "Money amount is out of range")
	case errors.Is(err, ErrUnauthorized):
		writeMoneyErrorStatus(w, http.StatusUnauthorized, "authentication_required", "Authentication required")
	default:
		writeMoneyErrorStatus(w, http.StatusInternalServerError, "internal_error", "Money operation unavailable")
	}
}

func writeMoneyErrorStatus(w http.ResponseWriter, status int, code, message string) {
	writeMoneyJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
