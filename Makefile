# Literature Review Service Makefile

.PHONY: all build test lint clean proto migrate \
	test-race test-integration test-e2e test-chaos test-security test-fuzz test-load test-all

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOCLEAN=$(GOCMD) clean
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Binary names
BINARY_SERVER=literature-review-server
BINARY_WORKER=literature-review-worker
BINARY_MIGRATE=literature-review-migrate

# Build directories
BUILD_DIR=bin

# Default target
all: lint test build

# Build binaries
build: build-server build-worker build-migrate

build-server:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_SERVER) ./cmd/server

build-worker:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_WORKER) ./cmd/worker

build-migrate:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_MIGRATE) ./cmd/migrate

# Run tests
test:
	$(GOTEST) -v -race -cover ./...

# Run tests with coverage report
test-coverage:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Run tests with race detector (multiple passes for intermittent races)
test-race:
	$(GOTEST) -race -count=3 ./...

# Run integration tests (requires docker-compose.test.yml services running)
test-integration:
	docker compose -f docker-compose.test.yml up -d --wait
	$(GOTEST) -tags integration -v -count=1 ./tests/integration/... || (docker compose -f docker-compose.test.yml down && exit 1)
	docker compose -f docker-compose.test.yml down

# Run E2E tests (requires full stack running)
test-e2e:
	$(GOTEST) -tags e2e -v -count=1 ./tests/e2e/...

# Run chaos tests
test-chaos:
	$(GOTEST) -race -v -count=1 ./tests/chaos/...

# Run security tests
test-security:
	$(GOTEST) -v -count=1 ./internal/server/http/... -run "TestSQL|TestResponse|TestMaxQuery|TestXSS|TestWriteDomain|TestLIKE"
	$(GOTEST) -v -count=1 ./internal/repository/... -run "TestPgKeywordRepository_LIKEPatternInjection"
	$(GOTEST) -v -count=1 ./tests/security/...

# Run fuzz tests (30 second fuzz time)
test-fuzz:
	$(GOTEST) -fuzz FuzzStartReviewQuery -fuzztime 30s ./tests/security/...

# Run load tests (requires k6 and running server)
test-load:
	k6 run tests/loadtest/review_lifecycle.js
	k6 run tests/loadtest/list_reviews.js

# Run all test suites (unit + race + chaos + security)
test-all: test test-race test-chaos test-security

# Lint
lint:
	golangci-lint run ./...

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Generate protobuf
proto:
	buf generate

# Lint protobuf
proto-lint:
	buf lint

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Run migrations up
migrate-up:
	$(BUILD_DIR)/$(BINARY_MIGRATE) up

# Run migrations down
migrate-down:
	$(BUILD_DIR)/$(BINARY_MIGRATE) down

# Create new migration
migrate-create:
	@read -p "Migration name: " name; \
	migrate create -ext sql -dir migrations -seq $$name

# Run server locally
run-server: build-server
	./$(BUILD_DIR)/$(BINARY_SERVER)

# Run worker locally
run-worker: build-worker
	./$(BUILD_DIR)/$(BINARY_WORKER)

# Development helpers
dev-setup:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/bufbuild/buf/cmd/buf@latest
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

.DEFAULT_GOAL := all
