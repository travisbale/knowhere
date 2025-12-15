package identity

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type contextKey string

// Context keys for identity information.
// Exported keys can be passed to logging frameworks to automatically extract identity information.
const (
	ActorIDKey   contextKey = "actor_id"
	TenantIDKey  contextKey = "tenant_id"
	IPAddressKey contextKey = "ip_address"
	UserAgentKey contextKey = "user_agent"
	RequestIDKey contextKey = "request_id"
)

var ErrNoActorInContext = errors.New("no actor ID found in context")
var ErrNoTenantInContext = errors.New("no tenant ID found in context")

// WithActor adds both actor ID and tenant ID to the context
// Used by JWT middleware to propagate authenticated identity to handlers and database layer
// The actor is the authenticated user performing the action
func WithActor(ctx context.Context, tenantID, actorID uuid.UUID) context.Context {
	ctx = context.WithValue(ctx, ActorIDKey, actorID)
	ctx = context.WithValue(ctx, TenantIDKey, tenantID)
	return ctx
}

// WithTenant adds only tenant ID to the context
// Used for pre-authentication flows (e.g., OAuth callbacks) where tenant is known but user isn't authenticated yet
func WithTenant(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

// GetActor retrieves the actor ID from the context
// The actor is the authenticated user performing the action
func GetActor(ctx context.Context) (uuid.UUID, error) {
	actorID, ok := ctx.Value(ActorIDKey).(uuid.UUID)
	if !ok || actorID == uuid.Nil {
		return uuid.Nil, ErrNoActorInContext
	}
	return actorID, nil
}

// GetTenant retrieves the tenant ID from the context
func GetTenant(ctx context.Context) (uuid.UUID, error) {
	tenantID, ok := ctx.Value(TenantIDKey).(uuid.UUID)
	if !ok || tenantID == uuid.Nil {
		return uuid.Nil, ErrNoTenantInContext
	}
	return tenantID, nil
}

// GetTenantAndActor retrieves both tenant ID and actor ID from the context
func GetTenantAndActor(ctx context.Context) (tenantID, actorID uuid.UUID, err error) {
	tenantID, err = GetTenant(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	actorID, err = GetActor(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return tenantID, actorID, nil
}

// WithIPAddress adds the client IP address to the context
// Used by HTTP middleware to propagate the client IP for logging and security
func WithIPAddress(ctx context.Context, ipAddress string) context.Context {
	return context.WithValue(ctx, IPAddressKey, ipAddress)
}

// GetIPAddress retrieves the client IP address from the context
// Returns empty string if no IP address is set
func GetIPAddress(ctx context.Context) string {
	ipAddress, ok := ctx.Value(IPAddressKey).(string)
	if !ok {
		return ""
	}
	return ipAddress
}

// WithUserAgent adds the User-Agent header value to the context
// Used by HTTP middleware for session tracking
func WithUserAgent(ctx context.Context, userAgent string) context.Context {
	return context.WithValue(ctx, UserAgentKey, userAgent)
}

// GetUserAgent retrieves the User-Agent from the context
// Returns empty string if no User-Agent is set
func GetUserAgent(ctx context.Context) string {
	userAgent, ok := ctx.Value(UserAgentKey).(string)
	if !ok {
		return ""
	}
	return userAgent
}

// WithRequestID adds the request ID to the context
// Used by HTTP and gRPC middleware for request correlation
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetRequestID retrieves the request ID from the context
// Returns empty string if no request ID is set
func GetRequestID(ctx context.Context) string {
	requestID, ok := ctx.Value(RequestIDKey).(string)
	if !ok {
		return ""
	}
	return requestID
}
