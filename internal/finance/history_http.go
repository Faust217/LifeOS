package finance

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
)

type runningHistoryEventDTO struct {
	eventResponse
	Positions []HistoryPosition `json:"account_balances"`
}
type runningHistoryResponse struct {
	Projection string                   `json:"projection"`
	Revision   string                   `json:"history_revision"`
	TimeZone   *string                  `json:"time_zone"`
	Limit      int                      `json:"limit"`
	HasMore    bool                     `json:"has_more"`
	NextCursor *string                  `json:"next_cursor"`
	Items      []runningHistoryEventDTO `json:"items"`
	Accounts   []HistoryAccount         `json:"accounts"`
	Categories []HistoryCategory        `json:"categories"`
}

func (h moneyHandler) runningHistory(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	query, err := moneyQuery(r, "view", "limit", "history_revision", "cursor")
	if err != nil || query.Get("view") != "running_balances" || r.ContentLength != 0 {
		writeMoneyError(w, ErrInvalidInput)
		return
	}
	input := HistoryQuery{Limit: 10, Revision: query.Get("history_revision"), Cursor: query.Get("cursor")}
	if query.Has("history_revision") && input.Revision == "" || query.Has("cursor") && input.Cursor == "" {
		writeMoneyError(w, ErrInvalidInput)
		return
	}
	if query.Has("limit") {
		input.Limit, err = strconv.Atoi(query.Get("limit"))
		if err != nil {
			writeMoneyError(w, ErrInvalidInput)
			return
		}
	}
	page, err := h.service.HistoryPage(r.Context(), owner, input)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	response := runningHistoryResponse{"lifeos-running-history-v1", page.Revision, page.TimeZone, page.Limit, page.HasMore, page.NextCursor, []runningHistoryEventDTO{}, page.Accounts, page.Categories}
	for _, item := range page.Items {
		response.Items = append(response.Items, runningHistoryEventDTO{eventDTO(item.Event), item.Positions})
	}
	writeMoneyJSON(w, 200, response)
}

// Reapply privacy headers at WriteHeader so even early shared-guard denials retain them.
type historyPrivateWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *historyPrivateWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *historyPrivateWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.ResponseWriter.WriteHeader(status)
}
func (w *historyPrivateWriter) Write(data []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(data)
}
func privateHistoryRead(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("view") {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
		}
		next(&historyPrivateWriter{ResponseWriter: w}, r)
	}
}
