# Literature Review Service Makefile

.PHONY: all build test lint clean proto migrate \
	test-race test-integration test-e2e test-chaos test-security test-fuzz test-load test-all \
	docker-build docker-push docker-run-server docker-run-worker \
	compose-up compose-up-full compose-down compose-logs

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

# ---------------------------------------------------------------------------
# Docker settings
# ---------------------------------------------------------------------------
DOCKER_REGISTRY ?= ghcr.io/helixir
DOCKER_IMAGE := $(DOCKER_REGISTRY)/literature-review-service
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DOCKER_TAG ?= $(VERSION)

## docker-build: Build Docker image (runs from monorepo root)
docker-build:
	cd .. && docker build \
		-f literature_service/Dockerfile \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--build-arg COMMIT_SHA=$(COMMIT_SHA) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_IMAGE):latest

## docker-push: Push Docker image to registry
docker-push:
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_IMAGE):latest

## docker-run-server: Run server container locally
docker-run-server:
	docker run --rm -p 8080:8080 -p 9090:9090 -p 9091:9091 \
		-e LITREVIEW_DATABASE_HOST=host.docker.internal \
		-e LITREVIEW_DATABASE_PASSWORD=devpassword \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

## docker-run-worker: Run worker container locally
docker-run-worker:
	docker run --rm \
		--entrypoint /app/literature-review-worker \
		-e LITREVIEW_DATABASE_HOST=host.docker.internal \
		-e LITREVIEW_DATABASE_PASSWORD=devpassword \
		-e LITREVIEW_TEMPORAL_HOST_PORT=host.docker.internal:7233 \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

## compose-up: Start infrastructure (postgres, temporal, kafka)
compose-up:
	POSTGRES_PASSWORD=devpassword docker compose up -d

## compose-up-full: Start all services including server and worker
compose-up-full:
	POSTGRES_PASSWORD=devpassword docker compose --profile full up -d --build

## compose-down: Stop all services
compose-down:
	docker compose --profile full down

## compose-logs: Tail logs for all services
compose-logs:
	docker compose --profile full logs -f

.DEFAULT_GOAL := all
