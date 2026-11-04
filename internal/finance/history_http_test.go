package finance

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPrivateHistoryReadBoundary(t *testing.T) {
	for _, path := range []string{"/api/v1/events?view=running_balances", "/api/v1/events?limit=5&offset=0"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest("GET", path, nil)
			response := httptest.NewRecorder()
			privateHistoryRead(func(w http.ResponseWriter, r *http.Request) {
				deadline, ok := r.Context().Deadline()
				if ok != r.URL.Query().Has("view") {
					t.Fatal("unexpected deadline contract")
				}
				if ok && (!deadline.After(time.Now()) || deadline.After(time.Now().Add(5*time.Second))) {
					t.Fatal("unbounded opt-in read")
				}
				// Even implicit success/early guard writes receive privacy headers.
				_, _ = w.Write([]byte(`{"items":[]}`))
			})(response, request)
			if response.Code != 200 || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("privacy boundary", response.Result())
			}
		})
	}
}
