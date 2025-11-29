package clog

import (
	"context"
	"net/http"
	"time"
)

// logger interface for HTTP request logging (matches *slog.Logger)
type logger interface {
	ErrorContext(ctx context.Context, msg string, args ...any)
}

// HTTP event constants for request logging
const (
	RequestFailed = "request_failed"
)

// Middleware returns middleware that logs HTTP requests with structured fields and context enrichment
// Automatically includes request_id, user_id, tenant_id, ip_address from context
func Middleware(logger logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Process request
			next.ServeHTTP(ww, r)

			// Calculate duration
			duration := time.Since(start).Milliseconds()
			statusCode := ww.statusCode

			// Build log fields
			fields := []any{
				"http_method", r.Method,
				"http_path", r.URL.Path,
				"http_status", statusCode,
				"duration_ms", duration,
			}

			// Only log server errors - proxy handles access logging
			if statusCode >= 500 {
				logger.ErrorContext(r.Context(), RequestFailed, fields...)
			}
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

// WriteHeader captures the status code before delegating to the underlying ResponseWriter
func (rw *responseWriter) WriteHeader(statusCode int) {
	if !rw.written {
		rw.statusCode = statusCode
		rw.written = true
		rw.ResponseWriter.WriteHeader(statusCode)
	}
}

// Write ensures WriteHeader is called if it hasn't been already
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}
