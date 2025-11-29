# Knowhere

Shared Go libraries for the microservices platform.

Named after [Knowhere](https://en.wikipedia.org/wiki/Knowhere), home of the Collector’s archive of rare artifacts. This repository stores the shared packages and domain primitives used across all services.

## Packages

### identity

Context helpers for propagating tenant and actor identity through request contexts.

```go
import "github.com/travisbale/knowhere/identity"

// Set identity in context
ctx = identity.WithActor(ctx, tenantID, actorID)

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

// Define your own scopes
const ScopeUserRead jwt.Scope = "user:read"
router.With(middleware.RequireScope(ScopeUserRead)).Get("/users", handler)
```

### clog

HTTP middleware for request logging.

```go
import "github.com/travisbale/knowhere/clog"

// Add logging middleware to router
router.Use(clog.Middleware(slog.Default()))
```

### crypto/argon2

Argon2id password hashing with configurable parameters.

```go
import "github.com/travisbale/knowhere/crypto/argon2"

hasher := argon2.NewHasher(&argon2.Config{
    Memory:      64 * 1024, // 64 MB
    Iterations:  1,
    SaltLength:  16,
    KeyLength:   32,
    Parallelism: 4,
})

hash, err := hasher.Hash("mysecretpassword")
err = hasher.Verify("mysecretpassword", hash)
```

### crypto/aes

AES-256-GCM encryption for sensitive data.

```go
import "github.com/travisbale/knowhere/crypto/aes"

key, err := aes.GenerateKey()
cipher, err := aes.NewCipher(key)

ciphertext, err := cipher.Encrypt("sensitive data")
plaintext, err := cipher.Decrypt(ciphertext)
```

### crypto/token

Cryptographically secure token generation and hashing.

```go
import "github.com/travisbale/knowhere/crypto/token"

// Generate URL-safe base64 token
tok, err := token.Generate(32)

// Hash token for secure storage
hashed := token.Hash(tok)
```

### crypto/password

Password validation with breach checking (Have I Been Pwned).

```go
import "github.com/travisbale/knowhere/crypto/password"

validator := password.NewValidator()
err := validator.Validate(ctx, "mypassword123")
```

### db

Database migration helpers using golang-migrate with embedded SQL files.

```go
import "github.com/travisbale/knowhere/db"

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Apply all pending migrations
err := db.MigrateUp(migrationsFS, "migrations", databaseURL)

// Rollback last migration
err := db.MigrateDown(migrationsFS, "migrations", databaseURL)

// Get current migration version
version, dirty, err := db.MigrateVersion(migrationsFS, "migrations", databaseURL)
```

### db/postgres

Generic PostgreSQL connection pool wrapper with tenant context support for Row-Level Security.

```go
import (
    "github.com/travisbale/knowhere/db/postgres"
    "myapp/internal/db/postgres/internal/sqlc"
)

// Define app-specific DB type alias
type DB = postgres.DB[*sqlc.Queries]

// Create connection pool
db, err := postgres.NewDB(ctx, databaseURL, func(d any) *sqlc.Queries {
    return sqlc.New(d.(sqlc.DBTX))
}, nil)

// Execute queries with tenant context (sets app.current_tenant_id for RLS)
err := db.WithTenantContext(ctx, func(q *sqlc.Queries) error {
    return q.CreateUser(ctx, params)
})

// Execute queries without tenant context
err := db.WithTransaction(ctx, func(q *sqlc.Queries) error {
    return q.GetVerificationToken(ctx, token)
})

// Execute with explicit tenant ID
err := db.WithExplicitTenant(ctx, tenantID, func(q *sqlc.Queries) error {
    return q.UpdateUser(ctx, params)
})
```

## Installation

```bash
go get github.com/travisbale/knowhere
```
