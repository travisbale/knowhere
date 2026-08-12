# Tests that need a database read KNOWHERE_TEST_DATABASE_URL and skip without it, so
# `go test ./...` stays useful on a machine with no Docker.
TEST_DATABASE_URL ?= postgres://knowhere:knowhere@localhost:5434/knowhere_test?sslmode=disable

.PHONY: fmt lint test test-db test-setup test-teardown

fmt:
	@gofmt -w .

lint:
	@echo "Linting code..."
	@docker run --rm -v $(PWD):/app -w /app golangci/golangci-lint:latest golangci-lint run --timeout 5m

# Unit tests only. The database tests skip.
test:
	@go test -race ./...

# Everything, including the transaction tests.
test-db:
	@KNOWHERE_TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -race -count=1 ./...

test-setup:
	@docker compose up -d --wait
	@echo "Test database ready on 5434"

test-teardown:
	@docker compose down -v
