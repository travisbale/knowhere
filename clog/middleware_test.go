package clog

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// setupTestLogger initializes a JSON logger for testing
func setupTestLogger() (*bytes.Buffer, logger) {
	var buf bytes.Buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{})
	return &buf, slog.New(jsonHandler)
}

// parseLogEntry parses the JSON log output from the buffer
func parseLogEntry(buf *bytes.Buffer) map[string]any {
	var logEntry map[string]any
	json.Unmarshal(buf.Bytes(), &logEntry)
	return logEntry
}

func TestLoggingMiddleware_Success_NoLog(t *testing.T) {
	buf, logger := setupTestLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	wrappedHandler := Middleware(logger)(handler)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	// Should not log successful requests (proxy handles this)
	if buf.Len() > 0 {
		t.Error("expected no log output for successful request")
	}
}

func TestLoggingMiddleware_ClientError_NoLog(t *testing.T) {
	buf, logger := setupTestLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	wrappedHandler := Middleware(logger)(handler)
	req := httptest.NewRequest("GET", "/notfound", nil)
	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	// Should not log client errors (proxy handles this)
	if buf.Len() > 0 {
		t.Error("expected no log output for client error")
	}
}

func TestLoggingMiddleware_ServerError(t *testing.T) {
	buf, logger := setupTestLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	wrappedHandler := Middleware(logger)(handler)
	req := httptest.NewRequest("POST", "/error", nil)
	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	logEntry := parseLogEntry(buf)

	if logEntry["level"] != "ERROR" {
		t.Errorf("expected level to be 'ERROR', got %v", logEntry["level"])
	}
	if logEntry["msg"] != RequestFailed {
		t.Errorf("expected msg to be '%s', got %v", RequestFailed, logEntry["msg"])
	}
	if logEntry["http_status"] != float64(500) {
		t.Errorf("expected http_status to be 500, got %v", logEntry["http_status"])
	}
	if logEntry["http_method"] != "POST" {
		t.Errorf("expected http_method to be 'POST', got %v", logEntry["http_method"])
	}
	if logEntry["http_path"] != "/error" {
		t.Errorf("expected http_path to be '/error', got %v", logEntry["http_path"])
	}
	if _, exists := logEntry["duration_ms"]; !exists {
		t.Error("expected duration_ms to be present")
	}
}

func TestLoggingMiddleware_ImplicitStatusOK_NoLog(t *testing.T) {
	buf, logger := setupTestLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	wrappedHandler := Middleware(logger)(handler)
	req := httptest.NewRequest("GET", "/implicit", nil)
	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	// Should not log implicit 200 OK (proxy handles this)
	if buf.Len() > 0 {
		t.Error("expected no log output for implicit 200 OK")
	}
}

func TestLoggingMiddleware_ServiceUnavailable(t *testing.T) {
	buf, logger := setupTestLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	wrappedHandler := Middleware(logger)(handler)
	req := httptest.NewRequest("GET", "/unavailable", nil)
	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	logEntry := parseLogEntry(buf)

	if logEntry["level"] != "ERROR" {
		t.Errorf("expected level to be 'ERROR', got %v", logEntry["level"])
	}
	if logEntry["http_status"] != float64(503) {
		t.Errorf("expected http_status to be 503, got %v", logEntry["http_status"])
	}
}
