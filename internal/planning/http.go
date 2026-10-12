package planning

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/go-chi/chi/v5"
)

const maxGoalRequestBytes = 16 << 10

type goalHandler struct{ service *Service }
type titleRequest struct {
	Title string `json:"title"`
}
type completionRequest struct {
	Completed *bool `json:"completed"`
}

type itemResponse struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type goalResponse struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	CompletedAt    *time.Time     `json:"completed_at,omitempty"`
	ArchivedAt     *time.Time     `json:"archived_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	CompletedCount int            `json:"completed_count"`
	TotalCount     int            `json:"total_count"`
	Items          []itemResponse `json:"items"`
}

func goalDTO(goal Goal) goalResponse {
	result := goalResponse{ID: goal.ID, Title: goal.Title, CompletedAt: goal.CompletedAt,
		ArchivedAt: goal.ArchivedAt, CreatedAt: goal.CreatedAt, UpdatedAt: goal.UpdatedAt,
		CompletedCount: goal.CompletedCount, TotalCount: goal.TotalCount, Items: make([]itemResponse, 0, len(goal.Items))}
	for _, item := range goal.Items {
		result.Items = append(result.Items, itemResponse{ID: item.ID, Title: item.Title,
			CompletedAt: item.CompletedAt, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt})
	}
	return result
}

// RegisterRoutes uses the shared session and CSRF guards, never client owner IDs.
func RegisterRoutes(router chi.Router, auth *identity.Auth, service *Service) {
	h := goalHandler{service: service}
	router.MethodFunc(http.MethodGet, "/goals", auth.RequireRead(h.listGoals))
	router.MethodFunc(http.MethodPost, "/goals", auth.RequireMutation(h.createGoal))
	router.MethodFunc(http.MethodGet, "/goals/{goalID}", auth.RequireRead(h.getGoal))
	router.MethodFunc(http.MethodPost, "/goals/{goalID}/rename", auth.RequireMutation(h.renameGoal))
	router.MethodFunc(http.MethodPost, "/goals/{goalID}/completion", auth.RequireMutation(h.completeGoal))
	router.MethodFunc(http.MethodPost, "/goals/{goalID}/archive", auth.RequireMutation(h.archiveGoal))
	router.MethodFunc(http.MethodPost, "/goals/{goalID}/restore", auth.RequireMutation(h.restoreGoal))
	router.MethodFunc(http.MethodPost, "/goals/{goalID}/items", auth.RequireMutation(h.addItem))
	router.MethodFunc(http.MethodPost, "/goals/{goalID}/items/{itemID}/rename", auth.RequireMutation(h.renameItem))
	router.MethodFunc(http.MethodPost, "/goals/{goalID}/items/{itemID}/completion", auth.RequireMutation(h.completeItem))
	router.MethodFunc(http.MethodDelete, "/goals/{goalID}/items/{itemID}", auth.RequireMutation(h.removeItem))
}

func (h goalHandler) listGoals(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if !validQuery(r) {
		writeGoalError(w, ErrInvalidInput)
		return
	}
	goals, err := h.service.ListGoals(r.Context(), owner)
	if err != nil {
		writeGoalError(w, err)
		return
	}
	items := make([]goalResponse, 0, len(goals))
	for _, goal := range goals {
		items = append(items, goalDTO(goal))
	}
	writeGoalJSON(w, http.StatusOK, struct {
		Items []goalResponse `json:"items"`
	}{items})
}

func (h goalHandler) getGoal(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if !validQuery(r) {
		writeGoalError(w, ErrInvalidInput)
		return
	}
	goal, err := h.service.GetGoal(r.Context(), owner, chi.URLParam(r, "goalID"))
	if err != nil {
		writeGoalError(w, err)
		return
	}
	writeGoalJSON(w, http.StatusOK, goalDTO(goal))
}

func (h goalHandler) createGoal(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if !validQuery(r) {
		writeGoalError(w, ErrInvalidInput)
		return
	}
	var input titleRequest
	if !decodeGoalJSON(w, r, &input) {
		return
	}
	goal, err := h.service.CreateGoal(r.Context(), owner, input.Title)
	if err != nil {
		writeGoalError(w, err)
		return
	}
	writeGoalJSON(w, http.StatusCreated, goalDTO(goal))
}

func (h goalHandler) renameGoal(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if !validQuery(r) {
		writeGoalError(w, ErrInvalidInput)
		return
	}
	var input titleRequest
	if !decodeGoalJSON(w, r, &input) {
		return
	}
	goal, err := h.service.RenameGoal(r.Context(), owner, chi.URLParam(r, "goalID"), input.Title)
	if err != nil {
		writeGoalError(w, err)
		return
	}
	writeGoalJSON(w, http.StatusOK, goalDTO(goal))
}

func (h goalHandler) completeGoal(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if !validQuery(r) {
		writeGoalError(w, ErrInvalidInput)
		return
	}
	var input completionRequest
	if !decodeGoalJSON(w, r, &input) {
		return
	}
	if input.Completed == nil {
		writeGoalError(w, ErrInvalidInput)
		return
	}
	goal, err := h.service.SetGoalCompleted(r.Context(), owner, chi.URLParam(r, "goalID"), *input.Completed)
	if err != nil {
		writeGoalError(w, err)
		return
	}
	writeGoalJSON(w, http.StatusOK, goalDTO(goal))
}

func (h goalHandler) archiveGoal(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if !validQuery(r) {
		writeGoalError(w, ErrInvalidInput)
		return
	}
	if !emptyGoalBody(w, r) {
		return
	}
	goal, err := h.service.ArchiveGoal(r.Context(), owner, chi.URLParam(r, "goalID"))
	if err != nil {
		writeGoalError(w, err)
		return
	}
	writeGoalJSON(w, http.StatusOK, goalDTO(goal))
}

func (h goalHandler) restoreGoal(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if !validQuery(r) {
		writeGoalError(w, ErrInvalidInput)
		return
	}
	if !emptyGoalBody(w, r) {
		return
	}
	goal, err := h.service.RestoreGoal(r.Context(), owner, chi.URLParam(r, "goalID"))
	if err != nil {
		writeGoalError(w, err)
		return
	}
	writeGoalJSON(w, http.StatusOK, goalDTO(goal))
}

func (h goalHandler) addItem(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if !validQuery(r) {
		writeGoalError(w, ErrInvalidInput)
		return
	}
	var input titleRequest
	if !decodeGoalJSON(w, r, &input) {
		return
	}
	goal, err := h.service.AddItem(r.Context(), owner, chi.URLParam(r, "goalID"), input.Title)
	if err != nil {
		writeGoalError(w, err)
		return
	}
	writeGoalJSON(w, http.StatusCreated, goalDTO(goal))
}

func (h goalHandler) renameItem(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if !validQuery(r) {
		writeGoalError(w, ErrInvalidInput)
		return
	}
	var input titleRequest
	if !decodeGoalJSON(w, r, &input) {
		return
	}
	goal, err := h.service.RenameItem(r.Context(), owner, chi.URLParam(r, "goalID"), chi.URLParam(r, "itemID"), input.Title)
	if err != nil {
		writeGoalError(w, err)
		return
	}
	writeGoalJSON(w, http.StatusOK, goalDTO(goal))
}

func (h goalHandler) completeItem(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if !validQuery(r) {
		writeGoalError(w, ErrInvalidInput)
		return
	}
	var input completionRequest
	if !decodeGoalJSON(w, r, &input) {
		return
	}
	if input.Completed == nil {
		writeGoalError(w, ErrInvalidInput)
		return
	}
	goal, err := h.service.SetItemCompleted(r.Context(), owner, chi.URLParam(r, "goalID"), chi.URLParam(r, "itemID"), *input.Completed)
	if err != nil {
		writeGoalError(w, err)
		return
	}
	writeGoalJSON(w, http.StatusOK, goalDTO(goal))
}

func (h goalHandler) removeItem(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if !validQuery(r) {
		writeGoalError(w, ErrInvalidInput)
		return
	}
	if !emptyGoalBody(w, r) {
		return
	}
	goal, err := h.service.RemoveItem(r.Context(), owner, chi.URLParam(r, "goalID"), chi.URLParam(r, "itemID"))
	if err != nil {
		writeGoalError(w, err)
		return
	}
	writeGoalJSON(w, http.StatusOK, goalDTO(goal))
}

func validQuery(r *http.Request) bool {
	values, err := url.ParseQuery(r.URL.RawQuery)
	return err == nil && len(values) == 0
}

func decodeGoalJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeGoalErrorStatus(w, 415, "unsupported_media_type", "Content-Type must be application/json")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxGoalRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeDecodeError(w, err)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeDecodeError(w, err)
		return false
	}
	return true
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var large *http.MaxBytesError
	if errors.As(err, &large) {
		writeGoalErrorStatus(w, 413, "request_too_large", "Request body is too large")
		return
	}
	writeGoalErrorStatus(w, 400, "invalid_json", "Invalid JSON request body")
}

func emptyGoalBody(w http.ResponseWriter, r *http.Request) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	if err != nil || len(body) != 0 {
		writeGoalError(w, ErrInvalidInput)
		return false
	}
	return true
}

func writeGoalJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeGoalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		writeGoalErrorStatus(w, 400, "invalid_input", "Invalid Goal input")
	case errors.Is(err, ErrNotFound):
		writeGoalErrorStatus(w, 404, "not_found", "Goal record not found")
	case errors.Is(err, ErrConflict):
		writeGoalErrorStatus(w, 409, "conflict", "Goal state does not permit this change")
	case errors.Is(err, ErrUnauthorized):
		writeGoalErrorStatus(w, 401, "authentication_required", "Authentication required")
	default:
		writeGoalErrorStatus(w, 500, "internal_error", "Goal operation unavailable")
	}
}

func writeGoalErrorStatus(w http.ResponseWriter, status int, code, message string) {
	writeGoalJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
