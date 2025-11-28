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

hash, err := hasher.HashPassword("mysecretpassword")
err = hasher.VerifyPassword("mysecretpassword", hash)
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

### db/postgres

PostgreSQL connection pool with multi-tenant transaction support and migration utilities.

```go
import "github.com/travisbale/knowhere/db/postgres"

// Create connection pool
db, err := postgres.NewDB(ctx, databaseURL)
defer db.Close()

// Or with custom configuration
cfg := postgres.DefaultConfig()
cfg.MaxConns = 50
db, err := postgres.NewDBWithConfig(ctx, databaseURL, cfg)

// Execute within tenant-scoped transaction (sets RLS context)
err = db.WithTenantContext(ctx, func(tx pgx.Tx) error {
    _, err := tx.Exec(ctx, "INSERT INTO users ...")
    return err
})

// Execute without tenant context (for pre-auth operations)
err = db.WithTransaction(ctx, func(tx pgx.Tx) error {
    _, err := tx.Exec(ctx, "SELECT * FROM verification_tokens ...")
    return err
})

// Run migrations (pass your embedded migrations FS)
//go:embed migrations/*.sql
var migrationsFS embed.FS

err = postgres.MigrateUp(databaseURL, migrationsFS, "migrations")
```

## Installation

```bash
go get github.com/travisbale/knowhere
```
