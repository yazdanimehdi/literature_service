# Quick Start

Get the Literature Review Service running locally in under 5 minutes.

## Prerequisites

- **Go 1.25+** (check with `go version`)
- **Docker** and **Docker Compose** (recommended for infrastructure)
- An **LLM API key** (OpenAI, Anthropic, or another supported provider)

For manual setup without Docker, you also need:

- PostgreSQL 16+
- Temporal Server 1.25+
- Kafka (optional, for event publishing)

---

## Option 1: Docker Compose (Recommended)

### 1. Clone the monorepo

```bash
git clone --recurse-submodules git@bitbucket.org:deepbio/helixir.git
cd helixir/literature_service
```

### 2. Start infrastructure

Docker Compose brings up PostgreSQL, Temporal, Temporal UI, and Kafka:

```bash
make compose-up
```

This starts:

| Service       | Port  | Description                          |
|---------------|-------|--------------------------------------|
| PostgreSQL    | 5432  | Database                             |
| Temporal      | 7233  | Workflow orchestration               |
| Temporal UI   | 8088  | Workflow visibility dashboard        |
| Kafka         | 9092  | Event streaming (outbox pattern)     |

Wait until all containers are healthy before proceeding:

```bash
docker compose ps
```

### 3. Configure environment variables

```bash
cp .env.example .env
```

Edit `.env` and set at minimum:

```bash
# Database password (matches docker-compose default)
LITREVIEW_DATABASE_PASSWORD=devpassword

# At least one LLM provider key
LITREVIEW_LLM_OPENAI_API_KEY=sk-your-key-here
```

Override the database SSL mode for local development:

```bash
export LITREVIEW_DATABASE_SSL_MODE=disable
```

### 4. Run database migrations

```bash
make build-migrate
./bin/literature-review-migrate -up
```

### 5. Start the server

```bash
make run-server
```

You should see log output confirming all listeners are ready:

```
literature-review-service is ready
  grpc_address=0.0.0.0:9090
  http_address=0.0.0.0:8080
  metrics_address=0.0.0.0:9091
```

### 6. Start the worker (separate terminal)

```bash
make run-worker
```

### 7. Verify

```bash
curl http://localhost:8080/healthz
```

Expected response:

```json
{"status":"ok","database":"healthy"}
```

---

## Option 2: Manual Setup

If you prefer to run infrastructure natively instead of Docker.

### PostgreSQL

```bash
# Create the database and user
createuser litreview
createdb -O litreview litreview

# Or via psql
psql -c "CREATE USER litreview WITH PASSWORD 'devpassword';"
psql -c "CREATE DATABASE litreview OWNER litreview;"
```

### Temporal Server

Follow the [Temporal Server quickstart](https://docs.temporal.io/self-hosted-guide/install) or use the development server:

```bash
temporal server start-dev --namespace literature-review
```

### Kafka (optional)

Kafka is only required if you enable outbox event publishing (`kafka.enabled: true` in `config/config.yaml`). For initial development, you can leave it disabled.

### Build and run

```bash
# Install development tools
make dev-setup

# Build all binaries
make build

# Set environment variables
export LITREVIEW_DATABASE_HOST=localhost
export LITREVIEW_DATABASE_PASSWORD=devpassword
export LITREVIEW_DATABASE_SSL_MODE=disable
export LITREVIEW_LLM_OPENAI_API_KEY=sk-your-key-here

# Run migrations
./bin/literature-review-migrate -up

# Start server and worker
./bin/literature-review-server   # terminal 1
./bin/literature-review-worker   # terminal 2
```

---

## Option 3: Full Docker Stack

Run everything in Docker, including the server and worker:

```bash
POSTGRES_PASSWORD=devpassword docker compose --profile full up -d --build
```

This builds the Docker image from the monorepo root and starts all services together.

---

## Your First Literature Review

Start a review by sending a natural language query. The service extracts keywords using an LLM, searches academic databases, and optionally expands the search by analyzing discovered papers.

### Start a review

```bash
curl -X POST \
  http://localhost:8080/api/v1/orgs/my-org/projects/my-project/literature-reviews \
  -H "Content-Type: application/json" \
  -d '{
    "query": "CRISPR gene editing applications in cancer therapy",
    "config": {
      "max_papers": 50,
      "max_expansion_depth": 1,
      "sources": ["semantic_scholar", "openalex", "pubmed"]
    }
  }'
```

Response:

```json
{
  "review_id": "a1b2c3d4-...",
  "workflow_id": "litreview-a1b2c3d4-...",
  "status": "pending",
  "created_at": "2026-02-05T10:00:00Z",
  "message": "Literature review started"
}
```

### Check status

```bash
curl http://localhost:8080/api/v1/orgs/my-org/projects/my-project/literature-reviews/{reviewID}
```

Response:

```json
{
  "review_id": "a1b2c3d4-...",
  "status": "searching",
  "progress": {
    "initial_keywords_count": 10,
    "total_keywords_processed": 8,
    "papers_found": 42,
    "papers_new": 42,
    "papers_ingested": 0,
    "papers_failed": 0,
    "max_expansion_depth": 1
  },
  "created_at": "2026-02-05T10:00:00Z",
  "started_at": "2026-02-05T10:00:01Z",
  "duration": "12.5s"
}
```

### Stream real-time progress (SSE)

```bash
curl -N http://localhost:8080/api/v1/orgs/my-org/projects/my-project/literature-reviews/{reviewID}/progress
```

This endpoint uses Server-Sent Events to push progress updates as the workflow runs.

### List discovered papers

```bash
curl "http://localhost:8080/api/v1/orgs/my-org/projects/my-project/literature-reviews/{reviewID}/papers?limit=10"
```

### List extracted keywords

```bash
curl "http://localhost:8080/api/v1/orgs/my-org/projects/my-project/literature-reviews/{reviewID}/keywords"
```

### List all reviews

```bash
curl "http://localhost:8080/api/v1/orgs/my-org/projects/my-project/literature-reviews?limit=20"
```

### Cancel a running review

```bash
curl -X DELETE \
  http://localhost:8080/api/v1/orgs/my-org/projects/my-project/literature-reviews/{reviewID}
```

---

## Temporal UI

Open [http://localhost:8088](http://localhost:8088) to view running workflows, inspect workflow history, and debug failures.

---

## Review Lifecycle

A literature review progresses through these statuses:

```
pending -> extracting_keywords -> searching -> expanding -> ingesting -> completed
                                                                     -> partial
                                                          -> failed
                                               -> cancelled
```

| Status                 | Description                                                  |
|------------------------|--------------------------------------------------------------|
| `pending`              | Review created, workflow not yet started                      |
| `extracting_keywords`  | LLM is extracting search keywords from your query            |
| `searching`            | Querying academic databases with extracted keywords           |
| `expanding`            | Extracting new keywords from discovered paper abstracts       |
| `ingesting`            | Submitting papers with PDFs to the ingestion service          |
| `completed`            | Finished successfully                                        |
| `partial`              | Finished but some operations failed or were skipped           |
| `failed`               | Unrecoverable error                                          |
| `cancelled`            | Cancelled by user or system                                  |

---

## Next Steps

- Read the [Developer Guide](DEVELOPER_GUIDE.md) for building, testing, and extending the service.
- Read the [Deployment Guide](DEPLOYMENT_GUIDE.md) for Docker, Helm, and production configuration.
- View `config/config.yaml` for the full configuration reference.
- View `.env.example` for all secret environment variables.
