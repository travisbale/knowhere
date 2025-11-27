# Knowhere

Shared Go libraries for the microservices platform.

## Packages

### identity

Context helpers for propagating tenant and actor identity through request contexts.

```go
import "github.com/travisbale/knowhere/identity"

// Set identity in context
ctx = identity.WithActor(ctx, actorID, tenantID)

// Retrieve identity from context
tenantID, err := identity.GetTenant(ctx)
actorID, err := identity.GetActor(ctx)
```

### jwt

JWT token issuance, validation, and HTTP middleware for authentication.

```go
import "github.com/travisbale/knowhere/jwt"

// Create JWT service
config := &jwt.Config{
    Issuer:                 "my-service",
    PrivateKeyPath:         "/path/to/private.pem",
    PublicKeyPath:          "/path/to/public.pem",
    AccessTokenExpiration:  15 * time.Minute,
    RefreshTokenExpiration: 24 * time.Hour,
}
service, err := jwt.NewService(config)

// Issue tokens
token, expiration, err := service.IssueAccessToken(tenantID, userID, scopes)

// Validate tokens
claims, err := service.ValidateToken(token)

// HTTP middleware
middleware := jwt.NewHTTPMiddleware(service)
router.Use(middleware.Authenticate)
router.With(middleware.RequireScope(jwt.ScopeUserRead)).Get("/users", handler)
```

## Installation

```bash
go get github.com/travisbale/knowhere
```
