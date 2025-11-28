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

func TestLoggingMiddleware_Success(t *testing.T) {
	buf, logger := setupTestLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	wrappedHandler := Middleware(logger)(handler)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	logEntry := parseLogEntry(buf)

	if logEntry["msg"] != RequestCompleted {
		t.Errorf("expected msg to be '%s', got %v", RequestCompleted, logEntry["msg"])
	}
	if logEntry["level"] != "INFO" {
		t.Errorf("expected level to be 'INFO', got %v", logEntry["level"])
	}
	if logEntry["http_method"] != "GET" {
		t.Errorf("expected http_method to be 'GET', got %v", logEntry["http_method"])
	}
	if logEntry["http_path"] != "/test" {
		t.Errorf("expected http_path to be '/test', got %v", logEntry["http_path"])
	}
	if logEntry["http_status"] != float64(200) {
		t.Errorf("expected http_status to be 200, got %v", logEntry["http_status"])
	}
	if _, exists := logEntry["duration_ms"]; !exists {
		t.Error("expected duration_ms to be present")
	}
}

func TestLoggingMiddleware_ClientError(t *testing.T) {
	buf, logger := setupTestLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	wrappedHandler := Middleware(logger)(handler)
	req := httptest.NewRequest("GET", "/notfound", nil)
	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	logEntry := parseLogEntry(buf)

	if logEntry["level"] != "WARN" {
		t.Errorf("expected level to be 'WARN', got %v", logEntry["level"])
	}
	if logEntry["http_status"] != float64(404) {
		t.Errorf("expected http_status to be 404, got %v", logEntry["http_status"])
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
}

func TestLoggingMiddleware_ImplicitStatusOK(t *testing.T) {
	buf, logger := setupTestLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	wrappedHandler := Middleware(logger)(handler)
	req := httptest.NewRequest("GET", "/implicit", nil)
	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	logEntry := parseLogEntry(buf)

	if logEntry["http_status"] != float64(200) {
		t.Errorf("expected http_status to be 200, got %v", logEntry["http_status"])
	}
	if logEntry["level"] != "INFO" {
		t.Errorf("expected level to be 'INFO', got %v", logEntry["level"])
	}
}
