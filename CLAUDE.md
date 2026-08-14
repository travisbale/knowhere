# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Knowhere is a library of shared Go packages — identity propagation, JWT issuance and
validation, a generic Postgres pool wrapper, cryptographic primitives, and migration helpers.

It is **a library, not a service**. It has no `main`, no deployment, and no opinion about who
imports it. Do not name consuming services in this repo — not in the README, not in the
compose file, not in comments. A library that knows its callers has a dependency pointing the
wrong way, and the list goes stale the moment someone adds a fourth service.

## Common Commands

```sh
make fmt            # gofmt -w .
make lint           # golangci-lint via Docker
make test           # unit tests; the database tests skip
make test-setup     # start the test Postgres on :5434
make test-db        # everything, including the transaction tests
make test-teardown  # stop it and drop the volume
```

CI runs four jobs: format, lint, build + vet, and test. Two details worth keeping:

- The format job runs `make fmt` and fails on a diff. Formatting here is **gofmt only** —
  unlike the service repos, which use goimports and so also enforce import grouping.
- The test job runs `make test-db`, not `make test`. The transaction tests skip without
  `KNOWHERE_TEST_DATABASE_URL`, and a suite that silently skips its most important cases in
  CI is worse than no suite.

## Packages

| Package | Holds |
| --- | --- |
| `identity` | Context helpers for tenant, actor, request ID, client IP, user agent; the HTTP middleware that populates them; `RequireProxySecret` |
| `jwt` | RSA issuer, validator, claims, and the HTTP `Authenticate` / `RequireScope` middleware |
| `db` | golang-migrate helpers over an embedded FS |
| `db/postgres` | `DB[Q]` — a pgxpool wrapper generic over a sqlc `Queries` type |
| `crypto/argon2` | Argon2id hashing |
| `crypto/aes` | AES-256-GCM |
| `crypto/token` | Secure token generation and hashing |
| `clog` | HTTP request-logging middleware |
| `api` | Generic JSON response, decode and validation helpers |

`api` and `clog` are not currently imported by either service in this workspace; the rest are.

### What Belongs Here

Primitives, not policy. A package earns its place if a second service would use it unchanged.

`crypto/password` was removed for failing that test: password length, common-word lists and
breach checks are one application's rules about its own users, and keeping them here meant a
library making product decisions and two repos needing a library release to change a minimum
length. It now lives in `heimdall/internal/password`. Do not reintroduce it, or anything
shaped like it — validation with a policy baked in, defaults that encode a business rule.

### `DB[Q]` and Transactions

The wrapper is generic over the queries type because a pgx pool and a pgx transaction expose
the same `Exec`/`Query` surface, so sqlc-generated code cannot tell them apart. That is the
whole premise: `WithTransaction` and `WithTenantContext` hand the closure a `Q` built over a
transaction, and the caller never learns which.

`WithTenantContext` begins a transaction, issues `SET LOCAL app.current_tenant_id`, and runs
the closure. **One call is one transaction** — that is the entire transaction API, and it is
deliberate.

Two attempts to extend it were built and dropped:

- `WithinTenantTx`, which took an existing transaction.
- A `DB` handle bound to one transaction, so repository methods could be composed inside it.

Both moved transaction control above the data layer, which made an ordinary repository call
behave differently depending on invisible context. A consumer that needs several writes to be
atomic composes them **inside one closure in its own repository layer**, extracting a helper
that takes the queries type where two paths need the same writes. The tests and CI added
alongside those attempts were kept; the API was not.

## Testing

Everything except `db/postgres` is a pure unit test. The transaction tests need real commit
and rollback semantics, so they need a database: they read `KNOWHERE_TEST_DATABASE_URL` and
**skip** without it, so `go test ./...` stays useful on a machine with no Docker. `make
test-db` supplies it; `make test-setup` starts the container on 5434, off the default port
that a local install or another project usually holds.

Each test creates its own uniquely named table, so the suite is parallel-safe and needs no
migrations.

## Releasing

A Go module is published by **tagging it** — the module proxy serves the tag straight from
VCS. There is no build or upload step, which is why `release.yml` only generates release notes
so a consumer deciding whether to bump has something to read.

**Never move a published tag.** `sum.golang.org` records the hash it first saw, permanently;
a moved tag becomes a checksum mismatch for everyone who already fetched it. Cut a new patch
instead.

The `go` directive is a language version and a minimum floor, not a pin. Bumping it raises the
floor for every consumer, so it is a breaking change for anyone on an older toolchain even
though the API did not move.

## Development Guidelines

- Keep comments minimal and focused on *why*. The code should speak for itself; do not
  narrate what a function does. One line where at all possible.
- No `Co-Authored-By` trailer and no generated-with footer, in commits or PR descriptions.
  The repo squash-merges with the PR body as the message, so anything in it lands in the
  log — write PR descriptions as prose for that reason.
- Exported API is a contract with every consumer at once. Removing or resigning a function
  means a coordinated PR in each repo that imports it, so prefer adding over changing.
- Commit subjects are lowercase and scoped by package where one applies (`feat(db):`,
  `identity:`), describing the change in the library's terms.
