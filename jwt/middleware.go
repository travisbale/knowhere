package jwt

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/travisbale/knowhere/identity"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	claimsContextKey contextKey = "jwt_claims"
)

type validator interface {
	ValidateToken(token string) (*Claims, error)
}

// HTTPMiddleware provides JWT authentication and authorization middleware
type HTTPMiddleware struct {
	validator validator
}

// NewHTTPMiddleware creates a new JWT HTTP middleware
func NewHTTPMiddleware(validator validator) *HTTPMiddleware {
	return &HTTPMiddleware{
		validator: validator,
	}
}

// Authenticate wraps a handler with JWT authentication.
// Extracts user ID and tenant ID from token and adds to request context.
func (m *HTTPMiddleware) Authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		// Expected format: "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, `{"error":"invalid authorization header format"}`, http.StatusUnauthorized)
			return
		}

		claims, err := m.validator.ValidateToken(parts[1])
		if err != nil {
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		// Add identity context for downstream RLS enforcement
		ctx := context.WithValue(r.Context(), claimsContextKey, claims)
		ctx = identity.WithActor(ctx, claims.TenantID, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// Scope represents a system permission identifier used in JWT tokens and authorization
type Scope string

// RequireScope wraps a handler with JWT authentication and scope verification.
func (m *HTTPMiddleware) RequireScope(scope Scope, next http.HandlerFunc) http.HandlerFunc {
	return m.Authenticate(func(w http.ResponseWriter, r *http.Request) {
		claims, err := getClaimsFromContext(r.Context())
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		for _, s := range claims.Scopes {
			if s == scope {
				next.ServeHTTP(w, r)
				return
			}
		}

		http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
	})
}

// GetJWTClaims extracts the full JWT claims from the request context
func GetJWTClaims(r *http.Request) (*Claims, error) {
	return getClaimsFromContext(r.Context())
}

// getClaimsFromContext retrieves JWT claims from the request context
func getClaimsFromContext(ctx context.Context) (*Claims, error) {
	claims, ok := ctx.Value(claimsContextKey).(*Claims)
	if !ok || claims == nil {
		return nil, errors.New("no claims found in context")
	}

	return claims, nil
}
