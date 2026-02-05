# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# literature_service multi-stage Dockerfile
#
# Build context: monorepo root (to resolve replace directives for sibling
# modules grpcauth, outbox, ingestion_service).
#
#   docker build -f literature_service/Dockerfile \
#     --build-arg VERSION=$(git describe --tags --always) \
#     --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
#     --build-arg COMMIT_SHA=$(git rev-parse --short HEAD) \
#     -t literature-review-service .
# ---------------------------------------------------------------------------

# ==========================================================================
# Stage 1: Builder
# ==========================================================================
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# -- Copy sibling modules required by replace directives -------------------
COPY grpcauth/ /build/grpcauth/
COPY outbox/ /build/outbox/
COPY ingestion_service/ /build/ingestion_service/

# -- Copy literature_service module ----------------------------------------
COPY literature_service/go.mod literature_service/go.sum /build/literature_service/
WORKDIR /build/literature_service
RUN go mod download

# Copy full source tree for the service
COPY literature_service/ /build/literature_service/

# Build arguments injected via --build-arg
ARG VERSION=dev
ARG BUILD_TIME
ARG COMMIT_SHA

# Build all three binaries with static linking and version metadata
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w \
      -X main.Version=${VERSION} \
      -X main.BuildTime=${BUILD_TIME} \
      -X main.CommitSHA=${COMMIT_SHA}" \
    -o /build/bin/literature-review-server ./cmd/server

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w \
      -X main.Version=${VERSION} \
      -X main.BuildTime=${BUILD_TIME} \
      -X main.CommitSHA=${COMMIT_SHA}" \
    -o /build/bin/literature-review-worker ./cmd/worker

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w \
      -X main.Version=${VERSION} \
      -X main.BuildTime=${BUILD_TIME} \
      -X main.CommitSHA=${COMMIT_SHA}" \
    -o /build/bin/literature-review-migrate ./cmd/migrate

# ==========================================================================
# Stage 2: Runtime
# ==========================================================================
FROM alpine:3.19

# Runtime dependencies
RUN apk add --no-cache ca-certificates tzdata wget

# Non-root user (UID/GID 1000)
RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

WORKDIR /app

# Copy binaries
COPY --from=builder /build/bin/literature-review-server  /app/literature-review-server
COPY --from=builder /build/bin/literature-review-worker  /app/literature-review-worker
COPY --from=builder /build/bin/literature-review-migrate /app/literature-review-migrate

# Copy migrations and config
COPY --from=builder /build/literature_service/migrations /app/migrations
COPY --from=builder /build/literature_service/config     /app/config

# Ownership
RUN chown -R appuser:appgroup /app

USER appuser

# Ports: HTTP (8080), gRPC (9090), metrics (9091)
EXPOSE 8080 9090 9091

# Health check against the HTTP health endpoint
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Default entrypoint is the gRPC+HTTP server.
# Override with --entrypoint for worker or migrate:
#   docker run --entrypoint /app/literature-review-worker ...
#   docker run --entrypoint /app/literature-review-migrate ...
ENTRYPOINT ["/app/literature-review-server"]
