package finance

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/go-chi/chi/v5"
)

const maxMoneyRequestBytes = 16 << 10

var minorUnitPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

type moneyHandler struct {
	service *Service
	auth    *identity.Auth
}

// RegisterRoutes adds only authenticated Finance routes to the shared /api/v1 router.
func RegisterRoutes(router chi.Router, auth *identity.Auth, service *Service) {
	handler := moneyHandler{service: service, auth: auth}
	router.MethodFunc(http.MethodGet, "/accounts", auth.RequireRead(handler.listAccounts))
	router.MethodFunc(http.MethodPost, "/accounts", handler.financialMutation(handler.createAccount))
	router.MethodFunc(http.MethodPost, "/accounts/{accountID}/opening-balance/preview", auth.RequireMutation(handler.previewOpeningBalance))
	router.MethodFunc(http.MethodPost, "/accounts/{accountID}/opening-balance/confirm", handler.financialMutation(handler.confirmOpeningBalance))
	router.MethodFunc(http.MethodGet, "/accounts/{accountID}/balance", auth.RequireRead(handler.balance))
	router.MethodFunc(http.MethodPost, "/accounts/{accountID}/archive", auth.RequireMutation(handler.archiveAccount))
	router.MethodFunc(http.MethodPost, "/accounts/{accountID}/restore", auth.RequireMutation(handler.restoreAccount))
	router.MethodFunc(http.MethodGet, "/categories", auth.RequireRead(handler.listCategories))
	router.MethodFunc(http.MethodPost, "/categories", auth.RequireMutation(handler.createCategory))
	router.MethodFunc(http.MethodPost, "/categories/{categoryID}/rename", auth.RequireMutation(handler.renameCategory))
	router.MethodFunc(http.MethodPost, "/categories/{categoryID}/archive", auth.RequireMutation(handler.archiveCategory))
	router.MethodFunc(http.MethodPost, "/categories/{categoryID}/restore", auth.RequireMutation(handler.restoreCategory))
	router.MethodFunc(http.MethodGet, "/categories/{categoryID}/spending", auth.RequireRead(handler.categorySpending))
	router.MethodFunc(http.MethodGet, "/events/{eventID}", auth.RequireRead(handler.getEvent))
	router.MethodFunc(http.MethodPost, "/events/{eventID}/edit", handler.financialMutation(handler.editEvent))
	router.MethodFunc(http.MethodGet, "/events", auth.RequireRead(handler.history))
	router.MethodFunc(http.MethodPost, "/events", handler.financialMutation(handler.postEvent))
	router.MethodFunc(http.MethodPost, "/transfers", handler.financialMutation(handler.transfer))
	router.MethodFunc(http.MethodPost, "/events/{eventID}/void", handler.financialMutation(handler.voidEvent))
	router.MethodFunc(http.MethodPost, "/events/{eventID}/delete", handler.financialMutation(handler.deleteEvent))
}

type accountRequest struct {
	Name                string     `json:"name"`
	Type                string     `json:"type"`
	Currency            string     `json:"currency"`
	OpeningBalanceMinor string     `json:"opening_balance_minor"`
	OpeningBalanceAt    *time.Time `json:"opening_balance_at"`
}

type accountResponse struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Type                string     `json:"type"`
	Currency            string     `json:"currency"`
	OpeningBalanceMinor string     `json:"opening_balance_minor"`
	ArchivedAt          *time.Time `json:"archived_at,omitempty"`
	OpeningBalanceAt    *time.Time `json:"opening_balance_at"`
	OpeningConfirmedAt  *time.Time `json:"opening_balance_confirmed_at"`
	OpeningBasis        string     `json:"opening_balance_basis"`
	BalanceRevision     string     `json:"balance_revision"`
}

type categoryRequest struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type categoryRenameRequest struct {
	Name string `json:"name"`
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
	Kind        string               `json:"kind"`
	AccountID   string               `json:"account_id"`
	AmountMinor string               `json:"amount_minor"`
	Currency    string               `json:"currency"`
	OccurredAt  time.Time            `json:"occurred_at"`
	Note        string               `json:"note"`
	Allocations *[]allocationRequest `json:"allocations"`
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
	EventRevision string               `json:"event_revision"`
	ID            string               `json:"id"`
	Kind          string               `json:"kind"`
	AmountMinor   string               `json:"amount_minor"`
	Currency      string               `json:"currency"`
	OccurredAt    time.Time            `json:"occurred_at"`
	Note          string               `json:"note"`
	Status        string               `json:"status"`
	VoidedAt      *time.Time           `json:"voided_at,omitempty"`
	VoidReason    string               `json:"void_reason,omitempty"`
	Movements     []movementResponse   `json:"movements"`
	Allocations   []allocationResponse `json:"allocations"`
}

func accountDTO(account Account) accountResponse {
	return accountResponse{ID: account.ID, Name: account.Name, Type: account.Type, Currency: account.Currency,
		OpeningBalanceMinor: strconv.FormatInt(account.OpeningBalanceMinor, 10), ArchivedAt: account.ArchivedAt, OpeningBalanceAt: utcOpeningInstant(account.OpeningBalanceAt), OpeningConfirmedAt: utcOpeningInstant(account.OpeningConfirmedAt), BalanceRevision: strconv.FormatInt(account.BalanceRevision, 10), OpeningBasis: account.OpeningBasis()}
}

func utcOpeningInstant(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	utc := at.UTC()
	return &utc
}

func categoryDTO(category Category) categoryResponse {
	return categoryResponse{ID: category.ID, Name: category.Name, Kind: category.Kind, ArchivedAt: category.ArchivedAt}
}

func eventDTO(event Event) eventResponse {
	result := eventResponse{EventRevision: strconv.FormatInt(event.Revision, 10), ID: event.ID, Kind: event.Kind, AmountMinor: strconv.FormatInt(event.AmountMinor, 10),
		Currency: event.Currency, OccurredAt: event.OccurredAt.UTC(), Note: event.Note, Status: event.Status,
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
		Name: input.Name, Type: input.Type, Currency: input.Currency, OpeningBalanceMinor: opening, OpeningBalanceAt: input.OpeningBalanceAt}, r.Header.Get("Idempotency-Key"))
	h.finishFinancialWrite(w, r, owner, http.StatusCreated, accountDTO(account), err)
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

func (h moneyHandler) restoreAccount(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	if !emptyMoneyBody(w, r) {
		return
	}
	if err := h.service.RestoreAccount(r.Context(), owner, chi.URLParam(r, "accountID")); err != nil {
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

func (h moneyHandler) renameCategory(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	var input categoryRenameRequest
	if !decodeMoneyJSON(w, r, &input) {
		return
	}
	category, err := h.service.RenameCategory(r.Context(), owner, chi.URLParam(r, "categoryID"), input.Name)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	writeMoneyJSON(w, http.StatusOK, categoryDTO(category))
}

func (h moneyHandler) archiveCategory(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	if !emptyMoneyBody(w, r) {
		return
	}
	if err := h.service.ArchiveCategory(r.Context(), owner, chi.URLParam(r, "categoryID")); err != nil {
		writeMoneyError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h moneyHandler) restoreCategory(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	if !emptyMoneyBody(w, r) {
		return
	}
	if err := h.service.RestoreCategory(r.Context(), owner, chi.URLParam(r, "categoryID")); err != nil {
		writeMoneyError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	if input.Allocations == nil {
		writeMoneyError(w, ErrInvalidInput)
		return
	}
	amount, err := parseMinorUnit(input.AmountMinor)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	allocations := make([]AllocationInput, 0, len(*input.Allocations))
	for _, allocation := range *input.Allocations {
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
		event, err = h.service.PostExpense(r.Context(), owner, eventInput, r.Header.Get("Idempotency-Key"))
	case "income":
		event, err = h.service.PostIncome(r.Context(), owner, eventInput, r.Header.Get("Idempotency-Key"))
	default:
		writeMoneyError(w, ErrInvalidInput)
		return
	}
	h.finishFinancialWrite(w, r, owner, http.StatusCreated, eventDTO(event), err)
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
		OccurredAt: input.OccurredAt, Note: input.Note}, r.Header.Get("Idempotency-Key"))
	h.finishFinancialWrite(w, r, owner, http.StatusCreated, eventDTO(event), err)
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
	revision, err := eventPrecondition(r)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	err = h.service.VoidEvent(r.Context(), owner, chi.URLParam(r, "eventID"), input.Reason, r.Header.Get("Idempotency-Key"), revision)
	h.finishFinancialWrite(w, r, owner, http.StatusNoContent, nil, err)
}

func (h moneyHandler) deleteEvent(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	if !emptyMoneyBody(w, r) {
		return
	}
	revision, err := eventPrecondition(r)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	err = h.service.DeleteEvent(r.Context(), owner, chi.URLParam(r, "eventID"), r.Header.Get("Idempotency-Key"), revision)
	h.finishFinancialWrite(w, r, owner, http.StatusNoContent, nil, err)
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
	var source io.Reader = http.MaxBytesReader(w, r.Body, maxMoneyRequestBytes)
	if r.Context().Value(financialWriteContextKey{}) != nil {
		data, err := io.ReadAll(source)
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeMoneyErrorStatus(w, 413, "request_too_large", "Request body is too large")
			return false
		}
		if err != nil || !utf8.Valid(data) || !uniqueJSONObject(data) {
			writeMoneyErrorStatus(w, 400, "invalid_json", "Invalid JSON request body")
			return false
		}
		source = bytes.NewReader(data)
	}
	decoder := json.NewDecoder(source)
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
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeMoneyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRevisionRequired):
		writeMoneyErrorStatus(w, 428, "event_revision_required", "Read the transaction before changing it")
	case errors.Is(err, ErrRevisionConflict):
		writeMoneyErrorStatus(w, 412, "event_revision_conflict", "This transaction changed. Review the latest record; your draft is kept")
	case errors.Is(err, ErrNotEditable):
		writeMoneyErrorStatus(w, 409, "event_not_editable", "Only posted transactions can be edited")
	case errors.Is(err, ErrKeyRequired):
		writeMoneyErrorStatus(w, 400, "idempotency_key_required", "A request key is required")
	case errors.Is(err, ErrInvalidKey):
		writeMoneyErrorStatus(w, 400, "invalid_idempotency_key", "Invalid request key")
	case errors.Is(err, ErrKeyConflict):
		writeMoneyErrorStatus(w, 409, "idempotency_key_conflict", "That Save key belongs to a different command; do not resubmit with a new key")
	case errors.Is(err, ErrKeyCapacity):
		writeMoneyErrorStatus(w, 409, "idempotency_capacity_exceeded", "Financial write capacity reached; existing Saves can still be retried")
	case errors.Is(err, ErrWriteBusy):
		w.Header().Set("Retry-After", "1")
		writeMoneyErrorStatus(w, 503, "financial_write_busy", "LifeOS is busy; retry the same Save")
	case errors.Is(err, ErrWriteUnknown):
		writeMoneyErrorStatus(w, 503, "financial_write_outcome_unknown", "Save may have completed; retry the same Save")
	case errors.Is(err, ErrFingerprintVersion):
		writeMoneyErrorStatus(w, 503, "idempotency_version_unsupported", "Save compatibility unavailable; retain this request key")
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
