# Literature Review Service -- Operations Runbook

## Table of Contents

- [Service Overview](#service-overview)
- [Architecture](#architecture)
- [Health Checks](#health-checks)
- [Checking Service Status](#checking-service-status)
- [Common Failure Modes and Resolutions](#common-failure-modes-and-resolutions)
- [Scaling](#scaling)
- [Rollback Procedures](#rollback-procedures)
- [Running Migrations Manually](#running-migrations-manually)
- [Useful Queries](#useful-queries)
- [Alert Response Procedures](#alert-response-procedures)
- [Key Metrics to Watch](#key-metrics-to-watch)

---

## Service Overview

The Literature Review Service automates systematic literature reviews. It searches academic paper sources, retrieves and processes papers, and uses LLM providers to analyze and synthesize findings.

### Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 8080 | HTTP | REST API, health checks |
| 9090 | gRPC | Service-to-service communication |
| 9091 | HTTP | Prometheus metrics endpoint |

### Dependencies

| Dependency | Purpose | Critical? |
|------------|---------|-----------|
| PostgreSQL | Primary data store | Yes |
| Temporal | Workflow orchestration | Yes |
| Kafka | Event publishing (outbox pattern) | Yes |
| Semantic Scholar API | Paper search and retrieval | No (degraded mode) |
| OpenAlex API | Paper search and retrieval | No (degraded mode) |
| PubMed API | Paper search and retrieval | No (degraded mode) |
| OpenAI API | LLM-powered analysis | No (degraded mode) |
| Anthropic API | LLM-powered analysis | No (degraded mode) |

### Binaries

| Binary | Purpose |
|--------|---------|
| `server` | gRPC + HTTP API server |
| `worker` | Temporal workflow worker |
| `migrate` | Database migration CLI |

### Namespaces

| Environment | Kubernetes Namespace |
|-------------|---------------------|
| Staging | `literature-review-staging` |
| Production | `literature-review-production` |

---

## Architecture

```
                    +------------------+
                    |   gRPC Clients   |
                    +--------+---------+
                             |
                    +--------v---------+
                    |  Server (gRPC)   |  :9090
                    |  Server (HTTP)   |  :8080
                    +--------+---------+
                             |
              +--------------+--------------+
              |                             |
     +--------v---------+       +----------v----------+
     |   PostgreSQL DB   |       |   Temporal Server    |
     +-------------------+       +----------+----------+
                                            |
                                 +----------v----------+
                                 |  Worker (Temporal)   |
                                 +----------+----------+
                                            |
                         +------------------+------------------+
                         |                  |                  |
                +--------v------+  +--------v------+  +-------v-------+
                | Paper Sources |  | LLM Providers |  |    Kafka      |
                | (S2, OA, PM)  |  | (OAI, Anthr.) |  |  (Outbox)     |
                +---------------+  +---------------+  +---------------+
```

---

## Health Checks

### HTTP Health Endpoint

```bash
curl -sf http://<service-host>:8080/health
```

Expected response: HTTP 200 with JSON body indicating component status.

### Kubernetes Probes

The Helm chart configures:
- **Liveness probe**: `GET /health` on port 8080 (checks if process is alive)
- **Readiness probe**: `GET /health` on port 8080 (checks if ready to serve traffic)

### Quick Health Check from Cluster

```bash
# Port-forward and check
kubectl port-forward svc/literature-review-service-server 8080:8080 -n <namespace> &
curl -sf http://localhost:8080/health
```

---

## Checking Service Status

### Pod Status

```bash
# List all pods
kubectl get pods -n <namespace> -l app.kubernetes.io/name=literature-review-service

# Get detailed pod info
kubectl describe pod <pod-name> -n <namespace>

# Check recent events
kubectl get events -n <namespace> --sort-by='.lastTimestamp' | tail -20
```

### Logs

```bash
# Server logs
kubectl logs -l app.kubernetes.io/component=server -n <namespace> --tail=100 -f

# Worker logs
kubectl logs -l app.kubernetes.io/component=worker -n <namespace> --tail=100 -f

# Logs from a specific pod
kubectl logs <pod-name> -n <namespace> --tail=200

# Previous container logs (after a crash)
kubectl logs <pod-name> -n <namespace> --previous
```

### Deployment Status

```bash
# Check rollout status
kubectl rollout status deployment/literature-review-service-server -n <namespace>
kubectl rollout status deployment/literature-review-service-worker -n <namespace>

# View deployment details
kubectl get deployments -n <namespace>

# Check replica sets
kubectl get rs -n <namespace>
```

### Helm Release Status

```bash
helm status literature-review-service -n <namespace>
helm history literature-review-service -n <namespace>
```

---

## Common Failure Modes and Resolutions

### 1. Database Connection Failures

**Symptoms**:
- Pods in `CrashLoopBackOff`
- Logs show `failed to connect to database` or `connection refused`
- Health endpoint returns unhealthy

**Diagnosis**:
```bash
# Check if PostgreSQL is reachable from within the cluster
kubectl run pg-check --rm -it --image=postgres:16-alpine -n <namespace> -- \
  pg_isready -h <db-host> -p 5432 -U <db-user>

# Check the database secret is mounted correctly
kubectl get secret literature-review-db-credentials -n <namespace> -o yaml

# Check connection pool metrics
curl -s http://localhost:9091/metrics | grep "literature_review_db_pool"
```

**Resolution**:
1. Verify the database host, port, and credentials in the Kubernetes secret.
2. Check if the PostgreSQL instance is running and accepting connections.
3. If the database was recently restored, run migrations (see [Running Migrations Manually](#running-migrations-manually)).
4. Check if the connection pool is exhausted -- this may indicate a connection leak or insufficient pool size.
5. Verify SSL mode configuration matches the database server settings.

### 2. Temporal Connection Failures

**Symptoms**:
- Worker pods in `CrashLoopBackOff`
- Logs show `failed to connect to Temporal` or `namespace not found`
- Workflows stuck in `Running` state

**Diagnosis**:
```bash
# Check Temporal namespace exists
kubectl exec -it <temporal-pod> -n temporal -- \
  tctl namespace describe literature-review

# Check worker registration
kubectl logs -l app.kubernetes.io/component=worker -n <namespace> | grep "registered"

# Check Temporal UI for stuck workflows
# (Access via port-forward to Temporal Web UI)
```

**Resolution**:
1. Verify the Temporal server address in service configuration.
2. Ensure the `literature-review` namespace exists in Temporal. Create it if missing:
   ```bash
   tctl namespace register --namespace literature-review \
     --retention 30 --description "Literature Review Service"
   ```
3. Check that the worker has the correct task queue name configured.
4. If workflows are stuck, check the Temporal event history for the specific workflow execution.
5. Restart the worker pods if they lost connection:
   ```bash
   kubectl rollout restart deployment/literature-review-service-worker -n <namespace>
   ```

### 3. Kafka Connection Failures

**Symptoms**:
- Outbox table growing (events not being published)
- Logs show `kafka: broker connection failed` or `topic not found`
- `literature_review_outbox_pending_total` metric increasing

**Diagnosis**:
```bash
# Check Kafka broker connectivity from pod
kubectl exec -it <pod-name> -n <namespace> -- \
  nc -zv <kafka-broker-host> 9092

# Check outbox backlog
kubectl exec -it <pod-name> -n <namespace> -- \
  psql -h <db-host> -U <db-user> -d <db-name> -c \
  "SELECT COUNT(*) FROM outbox WHERE published_at IS NULL;"

# Check Kafka topic existence
kubectl exec -it <kafka-pod> -n kafka -- \
  kafka-topics.sh --list --bootstrap-server localhost:9092 | grep literature
```

**Resolution**:
1. Verify Kafka broker addresses in the service configuration.
2. Ensure required topics exist. Create them if missing:
   ```bash
   kafka-topics.sh --create --topic literature-review-events \
     --bootstrap-server <broker>:9092 --partitions 6 --replication-factor 3
   ```
3. Check if Kafka authentication credentials (if SASL is enabled) are correct.
4. If the outbox backlog is large, monitor the `literature_review_outbox_published_total` metric after fixing connectivity to confirm events are draining.
5. As a last resort, manually mark stale outbox entries if they are no longer relevant.

### 4. LLM API Failures

**Symptoms**:
- Literature review workflows failing at the analysis stage
- Logs show `LLM API error`, `rate limit exceeded`, or `authentication failed`
- `literature_review_llm_errors_total` metric increasing

**Diagnosis**:
```bash
# Check LLM error metrics
curl -s http://localhost:9091/metrics | grep "literature_review_llm"

# Check API key validity (do NOT log the actual key)
kubectl get secret literature-review-llm-credentials -n <namespace> -o jsonpath='{.data}' | \
  xargs -I {} echo "Secret keys present: {}" | sed 's/[a-zA-Z0-9+/=]*/.../g'

# Check logs for specific error details
kubectl logs -l app.kubernetes.io/component=worker -n <namespace> | grep -i "llm\|openai\|anthropic"
```

**Resolution**:
1. Check the LLM provider status page (OpenAI: https://status.openai.com, Anthropic: https://status.anthropic.com).
2. Verify API keys are valid and have not been rotated without updating the Kubernetes secret.
3. If rate limited, check current usage against plan limits. The service has built-in rate limiting; adjust `llm.rate_limit_rps` in the config if needed.
4. If one provider is down, the service uses a factory pattern -- verify the fallback provider is configured.
5. Retry failed workflows after the provider issue is resolved:
   ```bash
   # Use Temporal CLI to retry failed workflows
   tctl workflow list --namespace literature-review --query "ExecutionStatus='Failed'" | \
     awk '{print $1}' | xargs -I {} tctl workflow retry --workflow_id {}
   ```

### 5. Paper Source API Rate Limiting

**Symptoms**:
- Paper search workflows taking longer than expected
- Logs show `rate limit exceeded` or HTTP 429 responses
- `literature_review_paper_source_rate_limited_total` metric increasing

**Diagnosis**:
```bash
# Check rate limiter metrics
curl -s http://localhost:9091/metrics | grep "literature_review_paper_source"

# Check which source is being rate limited
kubectl logs -l app.kubernetes.io/component=worker -n <namespace> | \
  grep -i "rate.limit\|429" | tail -20
```

**Resolution**:
1. Semantic Scholar: Free tier allows 100 requests/5 min. If consistently hitting limits, request an API key from Semantic Scholar.
2. OpenAlex: Polite pool allows higher limits if you configure an email in the `User-Agent` header.
3. PubMed: Register for an NCBI API key for higher rate limits (10 req/sec vs 3 req/sec).
4. Adjust backoff configuration in the service config:
   ```yaml
   paper_sources:
     semantic_scholar:
       rate_limit_rps: 0.3  # Reduce to stay under limits
       backoff_max_seconds: 120
   ```
5. If a source is severely rate-limited, temporarily disable it and rely on other sources.

### 6. Out of Memory (OOM) Kills

**Symptoms**:
- Pods restarting with `OOMKilled` status
- `kubectl describe pod` shows `Last State: Terminated, Reason: OOMKilled`

**Diagnosis**:
```bash
kubectl describe pod <pod-name> -n <namespace> | grep -A5 "Last State"
kubectl top pods -n <namespace>
```

**Resolution**:
1. Check if the memory limit is appropriate for the workload. Worker pods processing large papers may need more memory.
2. Increase memory limits in the Helm values:
   ```yaml
   worker:
     resources:
       limits:
         memory: 1Gi  # Increase from default
   ```
3. Investigate if there is a memory leak by checking memory usage trends in Grafana.

---

## Scaling

### Horizontal Pod Autoscaler (HPA)

The HPA is configured in the Helm chart with the following defaults:

| Component | Min Replicas | Max Replicas | CPU Target | Memory Target |
|-----------|-------------|-------------|------------|---------------|
| Server | 2 | 10 | 70% | 80% |
| Worker | 2 | 8 | 70% | 80% |

```bash
# Check HPA status
kubectl get hpa -n <namespace>

# View HPA details
kubectl describe hpa literature-review-service-server -n <namespace>
kubectl describe hpa literature-review-service-worker -n <namespace>
```

### Manual Scaling

Use manual scaling when you need immediate capacity that HPA cannot respond to quickly enough:

```bash
# Scale server pods
kubectl scale deployment/literature-review-service-server --replicas=5 -n <namespace>

# Scale worker pods
kubectl scale deployment/literature-review-service-worker --replicas=6 -n <namespace>
```

**When to scale manually vs rely on HPA**:
- **Use HPA**: Normal traffic fluctuations, gradual load increases.
- **Scale manually**: Known upcoming load spikes (e.g., batch review submissions), HPA is not responding fast enough, or during incident response when you need immediate capacity.

**Important**: Manual scaling overrides HPA temporarily. HPA will eventually adjust replicas back to its calculated target. To make manual scaling stick, update the HPA min replicas:
```bash
kubectl patch hpa literature-review-service-worker -n <namespace> \
  --patch '{"spec":{"minReplicas":4}}'
```

### Scaling Considerations

- **Server pods**: Scale based on incoming request rate. Watch `literature_review_grpc_requests_total`.
- **Worker pods**: Scale based on Temporal task queue depth. Watch `literature_review_temporal_task_queue_depth`.
- **Database connections**: Each pod opens a connection pool. Monitor total connections against PostgreSQL `max_connections`. Default pool size per pod is 10.
- **Kafka consumers**: Worker pods share Kafka consumer group. Adding workers improves event processing throughput proportionally up to the number of partitions.

---

## Rollback Procedures

### Helm Rollback

```bash
# View release history
helm history literature-review-service -n <namespace>

# Rollback to the previous release
helm rollback literature-review-service -n <namespace>

# Rollback to a specific revision
helm rollback literature-review-service <revision-number> -n <namespace>

# Verify the rollback
kubectl rollout status deployment/literature-review-service-server -n <namespace>
kubectl rollout status deployment/literature-review-service-worker -n <namespace>
```

### Verifying a Rollback Succeeded

1. Check that pods are running the expected image version:
   ```bash
   kubectl get pods -n <namespace> -o jsonpath='{.items[*].spec.containers[*].image}'
   ```
2. Verify the health endpoint returns healthy:
   ```bash
   kubectl port-forward svc/literature-review-service-server 8080:8080 -n <namespace> &
   curl -sf http://localhost:8080/health
   ```
3. Check that the rollout is complete with no pending replicas:
   ```bash
   kubectl get deployments -n <namespace>
   ```
4. Monitor error rates in Grafana for 10-15 minutes after rollback.

### Database Migration Rollback

If a release included a database migration that needs to be reverted:

```bash
# Check current migration version
kubectl exec -it <pod-name> -n <namespace> -- /app/migrate -action version

# Roll back one migration
kubectl exec -it <pod-name> -n <namespace> -- /app/migrate -action down -steps 1
```

**Warning**: Not all migrations are reversible. Check the migration files before rolling back. Data-destructive migrations (dropping columns, tables) cannot be undone.

---

## Running Migrations Manually

The `migrate` binary is included in the Docker image at `/app/migrate`.

### From a Running Pod

```bash
# Check current migration version
kubectl exec -it <pod-name> -n <namespace> -- /app/migrate -action version

# Run all pending migrations
kubectl exec -it <pod-name> -n <namespace> -- /app/migrate -action up

# Run a specific number of migrations
kubectl exec -it <pod-name> -n <namespace> -- /app/migrate -action up -steps 2

# Roll back one migration
kubectl exec -it <pod-name> -n <namespace> -- /app/migrate -action down -steps 1
```

### Using a One-Off Job

For production environments, prefer running migrations as a Kubernetes Job:

```bash
kubectl run literature-review-migrate \
  --image=ghcr.io/<org>/literature-review-service:<version> \
  --rm -it --restart=Never \
  -n <namespace> \
  --env="LITREVIEW_DATABASE_HOST=<db-host>" \
  --env="LITREVIEW_DATABASE_PORT=5432" \
  --env="LITREVIEW_DATABASE_USER=<db-user>" \
  --env="LITREVIEW_DATABASE_PASSWORD=<db-pass>" \
  --env="LITREVIEW_DATABASE_NAME=<db-name>" \
  --env="LITREVIEW_DATABASE_SSL_MODE=require" \
  --command -- /app/migrate -action up
```

---

## Useful Queries

These PostgreSQL queries help diagnose issues. Connect to the database:

```bash
kubectl port-forward svc/<postgres-service> 5432:5432 -n <namespace> &
psql -h localhost -U <db-user> -d <db-name>
```

### Active Literature Reviews

```sql
-- All active reviews with their current status
SELECT id, org_id, project_id, title, status, created_at, updated_at
FROM literature_reviews
WHERE status NOT IN ('completed', 'failed', 'cancelled')
ORDER BY created_at DESC
LIMIT 20;
```

### Failed Workflows

```sql
-- Reviews that failed in the last 24 hours
SELECT id, org_id, project_id, title, status, error_message, updated_at
FROM literature_reviews
WHERE status = 'failed'
  AND updated_at > NOW() - INTERVAL '24 hours'
ORDER BY updated_at DESC;
```

### Outbox Backlog

```sql
-- Unpublished outbox events (should be near zero in healthy state)
SELECT COUNT(*) AS pending_count,
       MIN(created_at) AS oldest_pending,
       AGE(NOW(), MIN(created_at)) AS oldest_age
FROM outbox
WHERE published_at IS NULL;

-- Outbox events by type
SELECT event_type, COUNT(*) AS count
FROM outbox
WHERE published_at IS NULL
GROUP BY event_type
ORDER BY count DESC;
```

### Paper Source Statistics

```sql
-- Papers retrieved by source in the last 7 days
SELECT source, COUNT(*) AS paper_count
FROM papers
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY source
ORDER BY paper_count DESC;
```

### Review Progress

```sql
-- Detailed progress of a specific review
SELECT lr.id, lr.title, lr.status,
       COUNT(DISTINCT p.id) AS total_papers,
       COUNT(DISTINCT CASE WHEN p.analysis_status = 'completed' THEN p.id END) AS analyzed,
       COUNT(DISTINCT CASE WHEN p.analysis_status = 'failed' THEN p.id END) AS failed
FROM literature_reviews lr
LEFT JOIN review_papers rp ON rp.review_id = lr.id
LEFT JOIN papers p ON p.id = rp.paper_id
WHERE lr.id = '<review-id>'
GROUP BY lr.id, lr.title, lr.status;
```

### Database Connection Status

```sql
-- Active connections (run against the PostgreSQL admin database)
SELECT count(*) AS total_connections,
       count(*) FILTER (WHERE state = 'active') AS active,
       count(*) FILTER (WHERE state = 'idle') AS idle,
       count(*) FILTER (WHERE state = 'idle in transaction') AS idle_in_transaction
FROM pg_stat_activity
WHERE datname = '<db-name>';
```

---

## Alert Response Procedures

### LiteratureReviewHighErrorRate

**Description**: The service is returning errors at a rate above the configured threshold.

**Steps**:
1. Check the error rate metric: `rate(literature_review_grpc_errors_total[5m])`.
2. Look at server logs for the specific errors:
   ```bash
   kubectl logs -l app.kubernetes.io/component=server -n <namespace> --tail=200 | grep "error"
   ```
3. Identify the failing endpoint from metrics: `literature_review_grpc_errors_total{method="..."}`.
4. Check if the issue is with a downstream dependency (database, Temporal, external API).
5. If the error is transient (e.g., a brief network blip), monitor for resolution.
6. If persistent, check the relevant failure mode section above for the specific dependency.

### LiteratureReviewHighLatency

**Description**: Request latency (p99) is above the configured threshold.

**Steps**:
1. Check the latency metric: `histogram_quantile(0.99, literature_review_grpc_request_duration_seconds_bucket)`.
2. Identify the slow endpoint.
3. Check database query latency: `literature_review_db_query_duration_seconds`.
4. Check if the database is under load:
   ```sql
   SELECT query, state, age(now(), query_start) AS duration
   FROM pg_stat_activity
   WHERE state = 'active' AND datname = '<db-name>'
   ORDER BY duration DESC;
   ```
5. Check pod CPU and memory usage: `kubectl top pods -n <namespace>`.
6. If database-related, check for missing indexes or long-running queries.
7. If load-related, consider scaling (see [Scaling](#scaling)).

### LiteratureReviewWorkflowsStuck

**Description**: Temporal workflows are not progressing.

**Steps**:
1. Check if worker pods are running:
   ```bash
   kubectl get pods -l app.kubernetes.io/component=worker -n <namespace>
   ```
2. Check worker logs for errors or panics.
3. Check Temporal task queue depth: `literature_review_temporal_task_queue_depth`.
4. Verify Temporal connectivity from the worker pod:
   ```bash
   kubectl exec -it <worker-pod> -n <namespace> -- nc -zv <temporal-host> 7233
   ```
5. Check Temporal UI for workflow execution details and error messages.
6. If workflows are stuck due to a bug, fix the bug, deploy, and then retry failed workflows.

### LiteratureReviewOutboxBacklog

**Description**: The outbox table has a growing number of unpublished events.

**Steps**:
1. Check the outbox pending metric: `literature_review_outbox_pending_total`.
2. Check Kafka connectivity (see [Kafka Connection Failures](#3-kafka-connection-failures)).
3. Check the outbox publisher logs:
   ```bash
   kubectl logs -l app.kubernetes.io/component=server -n <namespace> | grep "outbox"
   ```
4. Run the [Outbox Backlog](#outbox-backlog) SQL query to see the oldest pending event.
5. If Kafka is healthy and events are still not publishing, restart the server pods.

### LiteratureReviewPodCrashLooping

**Description**: One or more pods are in CrashLoopBackOff.

**Steps**:
1. Identify the crashing pod:
   ```bash
   kubectl get pods -n <namespace> | grep CrashLoopBackOff
   ```
2. Check the previous container logs:
   ```bash
   kubectl logs <pod-name> -n <namespace> --previous
   ```
3. Check pod events:
   ```bash
   kubectl describe pod <pod-name> -n <namespace>
   ```
4. Common causes: OOMKilled (increase memory), configuration errors (check secrets/configmaps), failed health checks (check dependencies).
5. If caused by a bad deployment, perform a [rollback](#rollback-procedures).

### LiteratureReviewDiskPressure

**Description**: Pod or node is running low on disk space.

**Steps**:
1. Check node disk usage:
   ```bash
   kubectl describe node <node-name> | grep -A5 "Conditions"
   ```
2. Check if large temporary files are being generated by paper processing.
3. Clean up completed Temporal workflow histories if they are consuming storage.
4. Check PostgreSQL disk usage and consider vacuuming:
   ```sql
   SELECT pg_size_pretty(pg_database_size('<db-name>'));
   ```

---

## Key Metrics to Watch

All service metrics use the `literature_review_` prefix and are exposed on port 9091 at `/metrics`.

### Request Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `literature_review_grpc_requests_total` | Counter | Total gRPC requests by method and status |
| `literature_review_grpc_errors_total` | Counter | Total gRPC errors by method and error code |
| `literature_review_grpc_request_duration_seconds` | Histogram | gRPC request latency by method |
| `literature_review_http_requests_total` | Counter | Total HTTP requests (health, metrics) |

### Database Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `literature_review_db_query_duration_seconds` | Histogram | Database query latency |
| `literature_review_db_pool_open_connections` | Gauge | Current open database connections |
| `literature_review_db_pool_idle_connections` | Gauge | Current idle database connections |
| `literature_review_db_pool_wait_count_total` | Counter | Total times a connection was waited for |

### Temporal Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `literature_review_temporal_workflow_started_total` | Counter | Workflows started by type |
| `literature_review_temporal_workflow_completed_total` | Counter | Workflows completed by type and status |
| `literature_review_temporal_workflow_duration_seconds` | Histogram | Workflow execution duration |
| `literature_review_temporal_task_queue_depth` | Gauge | Pending tasks in the task queue |
| `literature_review_temporal_activity_duration_seconds` | Histogram | Activity execution duration |

### External API Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `literature_review_paper_source_requests_total` | Counter | Paper source API requests by source and status |
| `literature_review_paper_source_rate_limited_total` | Counter | Rate limit hits by source |
| `literature_review_paper_source_duration_seconds` | Histogram | Paper source API latency |
| `literature_review_llm_requests_total` | Counter | LLM API requests by provider and status |
| `literature_review_llm_errors_total` | Counter | LLM API errors by provider and error type |
| `literature_review_llm_duration_seconds` | Histogram | LLM API request latency |
| `literature_review_llm_tokens_total` | Counter | LLM tokens consumed by provider and direction (input/output) |

### Outbox Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `literature_review_outbox_pending_total` | Gauge | Unpublished outbox events |
| `literature_review_outbox_published_total` | Counter | Successfully published outbox events |
| `literature_review_outbox_publish_errors_total` | Counter | Failed outbox publish attempts |
| `literature_review_outbox_publish_duration_seconds` | Histogram | Time to publish an outbox event |

### Business Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `literature_review_reviews_created_total` | Counter | Total literature reviews created |
| `literature_review_reviews_completed_total` | Counter | Total literature reviews completed |
| `literature_review_papers_processed_total` | Counter | Total papers processed |
| `literature_review_papers_analyzed_total` | Counter | Total papers analyzed by LLM |

### Recommended Grafana Dashboard Panels

1. **Request Rate**: `rate(literature_review_grpc_requests_total[5m])` grouped by method
2. **Error Rate**: `rate(literature_review_grpc_errors_total[5m]) / rate(literature_review_grpc_requests_total[5m]) * 100`
3. **P99 Latency**: `histogram_quantile(0.99, rate(literature_review_grpc_request_duration_seconds_bucket[5m]))`
4. **Active Workflows**: `literature_review_temporal_workflow_started_total - literature_review_temporal_workflow_completed_total`
5. **Outbox Backlog**: `literature_review_outbox_pending_total`
6. **DB Connection Pool**: `literature_review_db_pool_open_connections` vs `literature_review_db_pool_idle_connections`
7. **LLM Token Usage**: `rate(literature_review_llm_tokens_total[1h])` grouped by provider
8. **Paper Source Availability**: `rate(literature_review_paper_source_requests_total{status="success"}[5m]) / rate(literature_review_paper_source_requests_total[5m]) * 100`
