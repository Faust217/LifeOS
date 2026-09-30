package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Faust217/LifeOS/internal/platform/health"
)

type fakeReadiness struct {
	result health.Result
}

func (readiness fakeReadiness) Check(context.Context) health.Result {
	return readiness.result
}

func TestHealthHandler(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	NewRouter(discardLogger(), fakeReadiness{}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Header().Get("X-Request-Id") == "" {
		t.Fatal("response did not include a request identifier")
	}
	assertResponseStatus(t, response.Body.Bytes(), "ok", "")
}

func TestReadinessHealthy(t *testing.T) {
	result := health.Result{
		Ready: true, DatabaseStatus: health.StatusReady, MigrationStatus: health.StatusReady,
		MigrationVersion: 1, HasVersion: true, ExpectedVersion: 1,
	}
	response := performReadyRequest(result)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	assertResponseStatus(t, response.Body.Bytes(), "ready", "")
}

func TestReadinessDatabaseUnavailable(t *testing.T) {
	result := health.Result{
		DatabaseStatus: health.StatusUnavailable, MigrationStatus: health.StatusUnknown,
		ExpectedVersion: 1, ErrorCode: health.ErrorDatabaseUnavailable,
	}
	response := performReadyRequest(result)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	assertResponseStatus(t, response.Body.Bytes(), "not_ready", health.ErrorDatabaseUnavailable)
}

func TestReadinessMigrationNotReady(t *testing.T) {
	result := health.Result{
		DatabaseStatus: health.StatusReady, MigrationStatus: health.StatusBehind,
		MigrationVersion: 0, HasVersion: true, ExpectedVersion: 1, ErrorCode: health.ErrorMigrationNotReady,
	}
	response := performReadyRequest(result)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	assertResponseStatus(t, response.Body.Bytes(), "not_ready", health.ErrorMigrationNotReady)
}

func performReadyRequest(result health.Result) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	NewRouter(discardLogger(), fakeReadiness{result: result}).ServeHTTP(response, request)
	return response
}

func assertResponseStatus(t *testing.T, body []byte, wantStatus, wantErrorCode string) {
	t.Helper()
	var response operationalResponse
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&response); err != nil && err != io.EOF {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != wantStatus {
		t.Errorf("status = %q, want %q", response.Status, wantStatus)
	}
	if wantErrorCode == "" {
		if response.Error != nil {
			t.Errorf("unexpected error response: %+v", response.Error)
		}
		return
	}
	if response.Error == nil || response.Error.Code != wantErrorCode {
		t.Errorf("error = %+v, want code %q", response.Error, wantErrorCode)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}
