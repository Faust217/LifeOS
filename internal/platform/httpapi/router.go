// Package httpapi contains the Foundation HTTP transport adapters.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/Faust217/LifeOS/internal/platform/health"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// ReadinessChecker supplies transport-neutral operational readiness.
type ReadinessChecker interface {
	Check(context.Context) health.Result
}

// NewRouter creates the HTTP router. The optional API handler owns /api/v1.
func NewRouter(logger *slog.Logger, readiness ReadinessChecker, apiHandlers ...http.Handler) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(requestLogger(logger))
	router.Get("/healthz", healthHandler)
	router.Get("/readyz", readinessHandler(readiness))
	if len(apiHandlers) > 0 && apiHandlers[0] != nil {
		router.Mount("/api/v1", apiHandlers[0])
	}
	return router
}

type componentStatus struct {
	Status string `json:"status"`
}

type migrationStatus struct {
	Status          string `json:"status"`
	Version         *uint  `json:"version,omitempty"`
	ExpectedVersion uint   `json:"expected_version"`
	Dirty           bool   `json:"dirty"`
}

type errorStatus struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type operationalResponse struct {
	Status    string           `json:"status"`
	Database  *componentStatus `json:"database,omitempty"`
	Migration *migrationStatus `json:"migration,omitempty"`
	Error     *errorStatus     `json:"error,omitempty"`
}

func healthHandler(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, operationalResponse{Status: "ok"})
}

func readinessHandler(readiness ReadinessChecker) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		result := readiness.Check(request.Context())
		migration := &migrationStatus{
			Status:          result.MigrationStatus,
			ExpectedVersion: result.ExpectedVersion,
			Dirty:           result.Dirty,
		}
		if result.HasVersion {
			version := result.MigrationVersion
			migration.Version = &version
		}

		body := operationalResponse{
			Status:    "ready",
			Database:  &componentStatus{Status: result.DatabaseStatus},
			Migration: migration,
		}
		statusCode := http.StatusOK
		if !result.Ready {
			statusCode = http.StatusServiceUnavailable
			body.Status = "not_ready"
			body.Error = &errorStatus{
				Code:    result.ErrorCode,
				Message: readinessMessage(result.ErrorCode),
			}
		}
		writeJSON(response, statusCode, body)
	}
}

func readinessMessage(code string) string {
	switch code {
	case health.ErrorDatabaseUnavailable:
		return "PostgreSQL is unavailable"
	case health.ErrorMigrationStatusUnavailable:
		return "Migration status is unavailable"
	case health.ErrorMigrationDirty:
		return "Database migration state requires manual recovery"
	default:
		return "Database migrations are not ready"
	}
}

func writeJSON(response http.ResponseWriter, statusCode int, body operationalResponse) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(statusCode)
	_ = json.NewEncoder(response).Encode(body)
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			requestID := middleware.GetReqID(request.Context())
			response.Header().Set("X-Request-Id", requestID)
			wrapped := middleware.NewWrapResponseWriter(response, request.ProtoMajor)
			started := time.Now()
			next.ServeHTTP(wrapped, request)
			logger.InfoContext(request.Context(), "http request",
				"request_id", requestID,
				"method", request.Method,
				"path", request.URL.Path,
				"status", wrapped.Status(),
				"duration_ms", time.Since(started).Milliseconds(),
			)
		})
	}
}
