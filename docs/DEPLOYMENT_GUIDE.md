# Deployment Guide

Production deployment guide for the Literature Review Service, covering Docker images, Docker Compose, Helm charts, and operational best practices.

## Table of Contents

- [Docker Build](#docker-build)
- [Container Entrypoints](#container-entrypoints)
- [Ports](#ports)
- [Docker Compose](#docker-compose)
- [Helm Chart](#helm-chart)
- [Environment Variables](#environment-variables)
- [Health Checks](#health-checks)
- [Scaling](#scaling)
- [Observability](#observability)
- [Security](#security)
- [Production Checklist](#production-checklist)

---

## Docker Build

The Docker image packages all three binaries (server, worker, migrate), migration files, and default configuration into a single image.

### Build the image

The Dockerfile must be built from the **monorepo root** because `go.mod` has `replace` directives pointing to sibling modules (`grpcauth`, `outbox`, `ingestion_service`, `llm`):

```bash
# From the literature_service/ directory:
make docker-build
```

This is equivalent to:

```bash
cd .. && docker build \
  -f literature_service/Dockerfile \
  --build-arg VERSION=$(git describe --tags --always) \
  --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  --build-arg COMMIT_SHA=$(git rev-parse --short HEAD) \
  -t ghcr.io/helixir/literature-review-service:$(git describe --tags --always) .
```

### Multi-stage build

| Stage   | Base Image              | Purpose                                              |
|---------|-------------------------|------------------------------------------------------|
| Builder | `golang:1.25-alpine`    | Compiles all three binaries with static linking      |
| Runtime | `alpine:3.19`           | Minimal runtime with ca-certificates and tzdata      |

The builder stage copies sibling modules (`grpcauth/`, `outbox/`, `ingestion_service/`) into the build context to resolve `replace` directives.

### Build arguments

| Argument      | Description                          | Default |
|---------------|--------------------------------------|---------|
| `VERSION`     | Version string injected via ldflags  | `dev`   |
| `BUILD_TIME`  | UTC build timestamp                  | --      |
| `COMMIT_SHA`  | Short git commit hash                | --      |

### Push to registry

```bash
make docker-push
```

Pushes both the version tag and `latest` to `ghcr.io/helixir/literature-review-service`.

---

## Container Entrypoints

All three binaries are available in the image at `/app/`:

| Binary                              | Path                                | Purpose                    |
|-------------------------------------|-------------------------------------|----------------------------|
| `literature-review-server`          | `/app/literature-review-server`     | gRPC + HTTP server         |
| `literature-review-worker`          | `/app/literature-review-worker`     | Temporal worker            |
| `literature-review-migrate`         | `/app/literature-review-migrate`    | Database migration CLI     |

The default entrypoint is the server. Override with `--entrypoint` for other binaries:

```bash
# Run the server (default)
docker run ghcr.io/helixir/literature-review-service:latest

# Run the worker
docker run --entrypoint /app/literature-review-worker \
  ghcr.io/helixir/literature-review-service:latest

# Run migrations
docker run --entrypoint /app/literature-review-migrate \
  ghcr.io/helixir/literature-review-service:latest -up
```

The container runs as non-root user `appuser` (UID 1000, GID 1000).

---

## Ports

| Port | Protocol | Description               |
|------|----------|---------------------------|
| 8080 | HTTP     | REST API and health checks |
| 9090 | gRPC     | gRPC API                   |
| 9091 | HTTP     | Prometheus metrics         |

All ports are configurable via environment variables or `config.yaml`:

```bash
LITREVIEW_SERVER_HTTP_PORT=8080
LITREVIEW_SERVER_GRPC_PORT=9090
LITREVIEW_SERVER_METRICS_PORT=9091
```

---

## Docker Compose

The `docker-compose.yml` provides a complete local development stack.

### Infrastructure only

Start PostgreSQL, Temporal, Temporal UI, and Kafka:

```bash
make compose-up
# Equivalent to: POSTGRES_PASSWORD=devpassword docker compose up -d
```

### Full stack

Start infrastructure plus the server and worker containers:

```bash
make compose-up-full
# Equivalent to: POSTGRES_PASSWORD=devpassword docker compose --profile full up -d --build
```

### Other commands

```bash
make compose-down       # Stop all services (including full profile)
make compose-logs       # Tail logs for all services
```

### Service details

| Service      | Container Name          | Ports          | Profile  |
|------------- |-------------------------|----------------|----------|
| PostgreSQL   | litreview-postgres      | 5432           | default  |
| Temporal     | litreview-temporal      | 7233           | default  |
| Temporal UI  | litreview-temporal-ui   | 8088           | default  |
| Kafka        | litreview-kafka         | 9092           | default  |
| Server       | litreview-server        | 8080,9090,9091 | full     |
| Worker       | litreview-worker        | --             | full     |

### Test infrastructure

A separate compose file is available for integration tests with offset ports:

```bash
docker compose -f docker-compose.test.yml up -d --wait
docker compose -f docker-compose.test.yml down
```

---

## Helm Chart

A production-ready Helm chart is provided at `deploy/helm/literature-review-service/`.

### Install

```bash
helm install literature-review deploy/helm/literature-review-service \
  -f deploy/helm/literature-review-service/values-production.yaml \
  --set secrets.database.password=<DB_PASSWORD> \
  --set secrets.llm.openaiApiKey=<OPENAI_KEY> \
  --set secrets.database.host=<DB_HOST> \
  --set secrets.temporal.hostPort=<TEMPORAL_HOST>
```

### Chart components

The chart creates the following Kubernetes resources:

| Resource            | Description                                             |
|---------------------|---------------------------------------------------------|
| Server Deployment   | gRPC + HTTP server pods                                  |
| Worker Deployment   | Temporal worker pods                                     |
| Service             | ClusterIP exposing HTTP (8080), gRPC (9090), metrics (9091) |
| ConfigMap           | Non-secret application configuration                     |
| Secret              | Database password, API keys, TLS certificates            |
| ServiceAccount      | Pod identity                                             |
| HPA (server)        | Auto-scales server 2-12 replicas (CPU/memory targets)    |
| HPA (worker)        | Auto-scales worker 2-20 replicas (CPU/memory targets)    |
| PDB (server)        | Ensures at least 1 (staging) or 2 (prod) pods available  |
| PDB (worker)        | Ensures at least 1 (staging) or 2 (prod) pods available  |
| ServiceMonitor      | Prometheus scrape configuration                          |
| NetworkPolicy       | Restricts ingress/egress traffic                         |
| ExternalSecret      | Syncs secrets from AWS Secrets Manager (optional)        |
| Kafka Topics Job    | Creates Kafka topics on install                          |
| Ingress             | External access with TLS (production)                    |

### Values files

| File                      | Purpose                              |
|---------------------------|--------------------------------------|
| `values.yaml`             | Base defaults                        |
| `values-staging.yaml`     | Staging overrides                    |
| `values-production.yaml`  | Production overrides                 |

### Production values highlights

```yaml
server:
  replicaCount: 3
  resources:
    requests: { memory: "512Mi", cpu: "500m" }
    limits:   { memory: "2Gi",   cpu: "2000m" }

worker:
  replicaCount: 4
  resources:
    requests: { memory: "1Gi",   cpu: "1000m" }
    limits:   { memory: "4Gi",   cpu: "4000m" }

database:
  maxConns: 100
  minConns: 25
  sslMode: "require"

postgresql:
  architecture: replication
  readReplicas.replicaCount: 2
  primary.persistence.size: 200Gi
```

---

## Environment Variables

All configuration values can be set via environment variables with the `LITREVIEW_` prefix. Secrets must always be provided via environment variables (never in `config.yaml`).

### Required variables

| Variable                            | Description                          |
|-------------------------------------|--------------------------------------|
| `LITREVIEW_DATABASE_PASSWORD`       | PostgreSQL password                  |
| `LITREVIEW_DATABASE_HOST`           | PostgreSQL hostname                  |
| `LITREVIEW_TEMPORAL_HOST_PORT`      | Temporal server address              |
| `LITREVIEW_LLM_OPENAI_API_KEY`     | OpenAI API key (if using OpenAI)     |

### Commonly overridden variables

| Variable                               | Default                            | Description                         |
|----------------------------------------|------------------------------------|-------------------------------------|
| `LITREVIEW_DATABASE_SSL_MODE`          | `require`                          | `disable` for local dev             |
| `LITREVIEW_DATABASE_PORT`              | `5432`                             | PostgreSQL port                     |
| `LITREVIEW_DATABASE_NAME`              | `literature_review_service`        | Database name                       |
| `LITREVIEW_DATABASE_USER`              | `litreview`                        | Database user                       |
| `LITREVIEW_DATABASE_MAX_CONNS`         | `50`                               | Connection pool maximum             |
| `LITREVIEW_DATABASE_MIN_CONNS`         | `10`                               | Connection pool minimum             |
| `LITREVIEW_SERVER_HTTP_PORT`           | `8080`                             | HTTP API port                       |
| `LITREVIEW_SERVER_GRPC_PORT`           | `9090`                             | gRPC API port                       |
| `LITREVIEW_SERVER_METRICS_PORT`        | `9091`                             | Prometheus metrics port             |
| `LITREVIEW_TEMPORAL_NAMESPACE`         | `literature-review`                | Temporal namespace                  |
| `LITREVIEW_TEMPORAL_TASK_QUEUE`        | `literature-review-tasks`          | Temporal task queue name            |
| `LITREVIEW_LLM_PROVIDER`              | `openai`                           | LLM provider name                  |
| `LITREVIEW_KAFKA_BROKERS`             | `localhost:9092`                   | Kafka broker addresses              |
| `LITREVIEW_KAFKA_ENABLED`             | `false`                            | Enable Kafka publishing             |
| `LITREVIEW_LOGGING_LEVEL`             | `info`                             | Log level                           |
| `LITREVIEW_LOGGING_FORMAT`            | `json`                             | Log format (`json` or `console`)    |

### Paper source API keys (all optional)

| Variable                                           | Effect                              |
|----------------------------------------------------|-------------------------------------|
| `LITREVIEW_PAPER_SOURCES_SEMANTIC_SCHOLAR_API_KEY` | Increases Semantic Scholar rate limit |
| `LITREVIEW_PAPER_SOURCES_OPENALEX_API_KEY`         | Enables polite pool access           |
| `LITREVIEW_PAPER_SOURCES_PUBMED_API_KEY`           | Increases PubMed rate limit (3 to 10 req/s) |
| `LITREVIEW_PAPER_SOURCES_SCOPUS_API_KEY`           | Required to enable Scopus            |

---

## Health Checks

### HTTP endpoints

| Endpoint   | Method | Auth Required | Description                              |
|------------|--------|---------------|------------------------------------------|
| `/healthz` | GET    | No            | Liveness -- checks database connectivity |
| `/readyz`  | GET    | No            | Readiness -- checks database and overall readiness |

### Healthy response

```json
{"status": "ok", "database": "healthy"}
```

### Unhealthy response

```json
{"status": "unhealthy", "database": "unreachable", "error": "connection refused"}
```

### Docker HEALTHCHECK

The Dockerfile includes a built-in health check:

```dockerfile
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1
```

### Kubernetes probes

The Helm chart configures three probes for the server pods:

| Probe     | Path      | Initial Delay | Period | Failure Threshold |
|-----------|-----------|---------------|--------|-------------------|
| Liveness  | `/health` | 15s           | 20s    | 3                 |
| Readiness | `/health` | 5s            | 10s    | 3                 |
| Startup   | `/health` | 10s           | 10s    | 30                |

### gRPC health check

The gRPC server registers the standard `grpc.health.v1.Health` service. Use `grpc-health-probe` or any gRPC health check tool:

```bash
grpc-health-probe -addr=localhost:9090 \
  -service=literaturereview.v1.LiteratureReviewService
```

---

## Scaling

### Server

The server is **stateless** and can be scaled horizontally behind a load balancer. It handles HTTP REST API requests, gRPC requests, and proxies workflow operations to Temporal.

**Scaling levers:**
- Kubernetes HPA scales on CPU (target 65-70%) and memory (target 75-80%)
- Production range: 3 to 12 replicas
- Connection pool `max_conns` should be sized relative to replica count
- Total connections across all replicas must not exceed PostgreSQL `max_connections`

**Formula:**
```
max_conns_per_replica = floor(pg_max_connections * 0.8 / server_replicas)
```

### Worker

Workers are **stateless** and can be scaled by adding more instances. Temporal automatically distributes workflow tasks across all workers polling the same task queue (`literature-review-tasks`).

**Scaling levers:**
- Kubernetes HPA scales on CPU (target 70-75%) and memory (target 80-85%)
- Production range: 4 to 20 replicas
- Workers are more resource-intensive than servers due to LLM calls and paper source API calls
- Each worker needs its own database connections for status updates and paper storage

**Scaling considerations:**
- LLM API rate limits are shared across all workers. Enable `llm.resilience.enabled: true` and configure the rate limiter to stay within provider limits.
- Paper source APIs have rate limits. The per-source `rate_limit` configuration applies per worker instance. With N workers, effective aggregate rate is N times the configured limit.

### Database

**Connection pool sizing:**

| Setting             | Default | Staging | Production |
|---------------------|---------|---------|------------|
| `max_conns`         | 50      | 50      | 100        |
| `min_conns`         | 10      | 10      | 25         |
| `max_conn_lifetime` | 1h      | 30m     | 30m        |
| `max_conn_idle_time`| 30m     | 5m      | 5m         |

For production, consider PostgreSQL read replicas for query-heavy read traffic.

---

## Observability

### Metrics (Prometheus)

Prometheus metrics are exposed on port 9091 at `/metrics`. The service registers custom metrics under the `literature_review_` namespace.

Kubernetes pod annotations enable automatic scraping:

```yaml
prometheus.io/scrape: "true"
prometheus.io/port: "9091"
prometheus.io/path: "/metrics"
```

The Helm chart includes a `ServiceMonitor` resource for Prometheus Operator.

### Logging

All log output is structured JSON (zerolog) to stdout by default. Recommended log aggregation: ship container stdout to your log platform (Fluentd, Loki, CloudWatch, etc.).

Key log fields:
- `component`: `server`, `worker`, or `migrate`
- `org_id`, `project_id`: tenant context
- `workflow_id`, `request_id`: correlation identifiers

### Tracing (OpenTelemetry)

Distributed tracing is available via OpenTelemetry. Enable in configuration:

```yaml
tracing:
  enabled: true
  endpoint: "otel-collector:4317"
  service_name: "literature-review-service"
  sample_rate: 0.1
```

### Events (Kafka)

The outbox pattern publishes domain events to Kafka topic `events.outbox.literature_review_service`:

- `review.started` -- workflow started with extracted keywords
- `review.completed` -- workflow finished with final counts
- `review.failed` -- workflow encountered an unrecoverable error

---

## Security

### Container security

The runtime container runs as non-root (UID/GID 1000) and the Helm chart enforces:

```yaml
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 1000
  seccompProfile:
    type: RuntimeDefault

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: [ALL]
```

### Authentication

- gRPC endpoints are protected by the shared `grpcauth` interceptor (JWT validation).
- HTTP endpoints use auth middleware injected at server creation.
- Health check endpoints (`/healthz`, `/readyz`) and gRPC reflection are excluded from authentication.

### Network policies

The Helm chart supports Kubernetes `NetworkPolicy` to restrict traffic:

```yaml
networkPolicy:
  enabled: true
  allowedNamespaces:
    - default
    - ingress-nginx
    - monitoring
```

### Secret management

- Secrets are never stored in `config.yaml` or committed to version control.
- `.env` files are in `.gitignore`.
- Production uses ExternalSecrets with AWS Secrets Manager (configurable in Helm values).

### Database security

- SSL mode defaults to `require` for all non-local environments.
- All SQL queries use parameterized statements (no string interpolation).
- LIKE patterns escape special characters to prevent pattern injection.

---

## Production Checklist

### Before first deployment

- [ ] Run all database migrations: `./bin/literature-review-migrate -up`
- [ ] Set `LITREVIEW_DATABASE_SSL_MODE=require`
- [ ] Set `LITREVIEW_DATABASE_PASSWORD` via secret management (not plaintext)
- [ ] Set required LLM API key for the configured provider
- [ ] Configure `LITREVIEW_TEMPORAL_HOST_PORT` and `LITREVIEW_TEMPORAL_NAMESPACE`
- [ ] Create the Temporal namespace if it does not exist
- [ ] Configure Kafka brokers and enable publishing (`LITREVIEW_KAFKA_ENABLED=true`)
- [ ] Create Kafka topic: `events.outbox.literature_review_service`

### Infrastructure

- [ ] PostgreSQL running with replication (recommended for production)
- [ ] Temporal Server running with production storage backend
- [ ] Kafka cluster running with appropriate replication factor
- [ ] Network connectivity verified between all components

### Compute and scaling

- [ ] Server replicas: minimum 3 for high availability
- [ ] Worker replicas: minimum 4 (adjust based on workflow volume)
- [ ] HPA configured for both server and worker deployments
- [ ] PodDisruptionBudget set (`minAvailable: 2` for production)
- [ ] Connection pool `max_conns` sized for replica count vs. PostgreSQL `max_connections`
- [ ] Anti-affinity rules spread pods across nodes and zones

### Observability

- [ ] Prometheus scraping configured (ServiceMonitor or pod annotations)
- [ ] Log aggregation pipeline connected to container stdout
- [ ] OpenTelemetry tracing enabled and collector reachable (optional)
- [ ] Alerts configured for error rates, latency, and workflow failures

### Security

- [ ] Container runs as non-root (UID 1000) -- enforced by Dockerfile
- [ ] Read-only root filesystem enabled in Kubernetes
- [ ] Network policies restrict ingress/egress
- [ ] External secrets synced from secret manager
- [ ] TLS configured for ingress (cert-manager or manual)
- [ ] gRPC authentication interceptor configured with valid JWKS endpoint

### Operational readiness

- [ ] Graceful shutdown timeout appropriate (30-60 seconds)
- [ ] Health and readiness probes configured and tested
- [ ] Migration process documented and tested (run before deploy)
- [ ] Rollback procedure tested
- [ ] Load testing completed with expected traffic patterns
- [ ] On-call runbook prepared for common failure scenarios
