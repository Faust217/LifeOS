package exporting

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type handler struct {
	service *Service
	auth    *identity.Auth
}

func RegisterRoutes(router chi.Router, auth *identity.Auth, service *Service) {
	h := handler{service, auth}
	protected := auth.RequireMutation(h.download)
	router.HandleFunc("/me/export", func(w http.ResponseWriter, r *http.Request) {
		w = &privateWriter{ResponseWriter: w}
		privateHeaders(w)
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			exportError(w, 405, "method_not_allowed", "Use POST to request an export.")
			return
		}
		protected(w, r)
	})
}

type privateWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *privateWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *privateWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	privateHeaders(w.ResponseWriter)
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}
func (w *privateWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}

func connectionUnavailable(err error) bool {
	var connect *pgconn.ConnectError
	var network net.Error
	var database *pgconn.PgError
	return errors.As(err, &connect) || errors.As(err, &network) || (errors.As(err, &database) && (strings.HasPrefix(database.Code, "08") || strings.HasPrefix(database.Code, "53") || database.Code == "57P01"))
}

func privateHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
func exportError(w http.ResponseWriter, status int, code, message string) {
	privateHeaders(w)
	w.Header().Del("Content-Disposition")
	w.Header().Del("Content-Length")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func (h handler) download(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		exportError(w, 415, "invalid_content_type", "Use application/json.")
		return
	}
	var input struct {
		Format string `json:"format"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	d.DisallowUnknownFields()
	if r.URL.RawQuery != "" || d.Decode(&input) != nil || d.Decode(&struct{}{}) != io.EOF || (input.Format != "json" && input.Format != "finance_csv") {
		exportError(w, 400, "invalid_export_request", "Choose JSON or Finance CSV without filters or owner overrides.")
		return
	}
	artifact, err := h.service.Generate(r.Context(), owner, input.Format)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		switch {
		case errors.Is(err, ErrBusy):
			w.Header().Set("Retry-After", "5")
			exportError(w, 429, "export_busy", "An export is already running. Retry shortly.")
		case errors.Is(err, ErrLimit):
			exportError(w, 422, "export_limit_exceeded", "This export exceeds the supported size. No partial file was generated.")
		case errors.Is(err, context.DeadlineExceeded):
			exportError(w, 503, "export_timeout", "Export generation timed out. Please retry.")
		case errors.Is(err, identity.ErrNotAuthorized) || errors.Is(err, pgx.ErrNoRows):
			exportError(w, 401, "authentication_required", "Please sign in again.")
		case errors.Is(err, pgx.ErrTxClosed):
			exportError(w, 503, "export_unavailable", "Export is unavailable. Please retry.")
		default:
			// Keep database/serialization details out of both public errors and logs.
			if connectionUnavailable(err) {
				exportError(w, 503, "export_unavailable", "Export is unavailable. Please retry.")
			} else {
				exportError(w, 500, "export_failed", "Export could not be completed. Please retry.")
			}
		}
		return
	}
	defer artifact.Close()
	if r.Context().Err() != nil {
		return
	}
	revalidateCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err = h.auth.RevalidateOwner(r.WithContext(revalidateCtx), owner); err != nil {
		if errors.Is(err, identity.ErrNotAuthorized) || errors.Is(err, pgx.ErrNoRows) {
			exportError(w, 401, "authentication_required", "Please sign in again.")
		} else {
			exportError(w, 503, "session_unavailable", "Session validation unavailable.")
		}
		return
	}
	controller := http.NewResponseController(w)
	if err = controller.SetWriteDeadline(time.Now().Add(h.service.limits.write)); err != nil {
		exportError(w, 503, "export_unavailable", "Secure download is unavailable.")
		return
	}
	defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
	privateHeaders(w)
	w.Header().Set("Content-Type", artifact.ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+artifact.Filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(artifact.Data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(artifact.Data) // A failed network write cannot be replaced with a JSON file.
}
