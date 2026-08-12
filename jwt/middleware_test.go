package jwt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	ScopeUserRead   Scope = "user:read"
	ScopeUserUpdate Scope = "user:update"
)

type mockValidator struct {
	claims *Claims
	err    error
}

func (m *mockValidator) ValidateToken(token string) (*Claims, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.claims, nil
}

func newValidClaims(userID, tenantID uuid.UUID, scopes ...Scope) *Claims {
	return &Claims{
		UserID:   userID,
		TenantID: tenantID,
		Scopes:   scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userID.String(),
		},
	}
}

func newMockValidator(claims *Claims) *mockValidator {
	return &mockValidator{claims: claims}
}

func newMockValidatorWithError(err error) *mockValidator {
	return &mockValidator{err: err}
}

func testRequest(handler http.HandlerFunc, authToken string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	if authToken != "" {
		req.Header.Set("Authorization", authToken)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func testHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"success"}`))
	}
}

func TestAuthenticate_Success(t *testing.T) {
	claims := newValidClaims(uuid.New(), uuid.New(), ScopeUserRead)
	m := NewHTTPMiddleware(newMockValidator(claims))

	rec := testRequest(m.Authenticate(testHandler()), "Bearer valid-token")
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestAuthenticate_MissingAuthHeader(t *testing.T) {
	m := NewHTTPMiddleware(newMockValidator(nil))

	rec := testRequest(m.Authenticate(testHandler()), "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthenticate_InvalidFormat(t *testing.T) {
	m := NewHTTPMiddleware(newMockValidator(nil))

	for _, header := range []string{"InvalidFormat", "Bearer", "Basic token", "Bearer token with spaces"} {
		rec := testRequest(m.Authenticate(testHandler()), header)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("header %q: expected 401, got %d", header, rec.Code)
		}
	}
}

func TestAuthenticate_InvalidToken(t *testing.T) {
	m := NewHTTPMiddleware(newMockValidatorWithError(ErrInvalidToken))

	rec := testRequest(m.Authenticate(testHandler()), "Bearer invalid")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireScope_Success(t *testing.T) {
	claims := newValidClaims(uuid.New(), uuid.New(), ScopeUserRead, ScopeUserUpdate)
	m := NewHTTPMiddleware(newMockValidator(claims))

	rec := testRequest(m.RequireScope(ScopeUserRead, testHandler()), "Bearer valid-token")
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequireScope_MissingScope(t *testing.T) {
	claims := newValidClaims(uuid.New(), uuid.New(), ScopeUserRead)
	m := NewHTTPMiddleware(newMockValidator(claims))

	rec := testRequest(m.RequireScope(ScopeUserUpdate, testHandler()), "Bearer valid-token")
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRequireScope_NoScopes(t *testing.T) {
	claims := newValidClaims(uuid.New(), uuid.New())
	m := NewHTTPMiddleware(newMockValidator(claims))

	rec := testRequest(m.RequireScope(ScopeUserRead, testHandler()), "Bearer valid-token")
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRequireScope_MissingAuthHeader(t *testing.T) {
	claims := newValidClaims(uuid.New(), uuid.New(), ScopeUserRead)
	m := NewHTTPMiddleware(newMockValidator(claims))

	rec := testRequest(m.RequireScope(ScopeUserRead, testHandler()), "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireScope_InvalidToken(t *testing.T) {
	m := NewHTTPMiddleware(newMockValidatorWithError(ErrInvalidToken))

	rec := testRequest(m.RequireScope(ScopeUserRead, testHandler()), "Bearer invalid")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestGetJWTClaims_Success(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	claims := newValidClaims(userID, tenantID, ScopeUserRead, ScopeUserUpdate)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), claimsContextKey, claims)
	req = req.WithContext(ctx)

	retrieved, err := GetJWTClaims(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if retrieved.UserID != userID {
		t.Errorf("expected user ID %s, got %s", userID, retrieved.UserID)
	}
	if retrieved.TenantID != tenantID {
		t.Errorf("expected tenant ID %s, got %s", tenantID, retrieved.TenantID)
	}
	if len(retrieved.Scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(retrieved.Scopes))
	}
}

func TestGetJWTClaims_NoContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	_, err := GetJWTClaims(req)
	if err == nil {
		t.Error("expected error when no claims in context")
	}
}
