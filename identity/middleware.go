package identity

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
)

var (
	// prefix is a unique identifier for this process instance (hostname + random)
	prefix string
	// reqID is an atomic counter for generating unique request IDs
	reqID uint64
)

func init() {
	hostname, err := os.Hostname()
	if hostname == "" || err != nil {
		hostname = "localhost"
	}

	buf := make([]byte, 5)
	_, _ = rand.Read(buf)
	prefix = fmt.Sprintf("%s/%s", hostname, base64.RawURLEncoding.EncodeToString(buf))
}

// RequestID generates a unique request ID and adds it to the request context.
// If the X-Request-Id header is present, that value is used instead.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-Id")
		if requestID == "" {
			id := atomic.AddUint64(&reqID, 1)
			requestID = fmt.Sprintf("%s-%06d", prefix, id)
		}

		ctx := WithRequestID(r.Context(), requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ClientIP extracts the client IP address and adds it to the request context
// This should run before logging middleware so IP is available for log enrichment
func ClientIP(trustedProxyMode bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIPAddress(r, trustedProxyMode)
			ctx := WithIPAddress(r.Context(), ip)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractIPAddress extracts the client IP address from the request with security validation
func extractIPAddress(r *http.Request, trustedProxyMode bool) string {
	// Behind trusted reverse proxy - extract from proxy headers
	if trustedProxyMode {
		var ip string

		// Take the rightmost IP (last entry added by our trusted proxy)
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				ip = strings.TrimSpace(ips[len(ips)-1])
			}
		}

		// Fallback to X-Real-IP if X-Forwarded-For not present
		if ip == "" {
			if xri := r.Header.Get("X-Real-IP"); xri != "" {
				ip = strings.TrimSpace(xri)
			}
		}

		// Last resort: use RemoteAddr
		if ip != "" {
			return ip
		}
	}

	// Direct connection or untrusted proxy - use RemoteAddr
	// RemoteAddr format is "IP:port", extract just the IP
	if host, _, ok := strings.Cut(r.RemoteAddr, ":"); ok {
		return host
	}

	return r.RemoteAddr
}

// UserAgent extracts the User-Agent header and adds it to the request context
// Used for session tracking to identify device/browser
func UserAgent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		ctx := WithUserAgent(r.Context(), ua)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
