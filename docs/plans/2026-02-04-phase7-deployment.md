# Phase 7: Deployment — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create production-ready deployment infrastructure for the literature review service including multi-stage Docker images for server and worker, local development docker-compose, Helm chart with environment overlays, CI/CD pipelines, Grafana dashboards, and an operations runbook.

**Architecture:** Phase 7 covers 10 deliverables (D7.1–D7.10). The service has two binaries (server + worker) that share the same Docker image but use different entrypoints. Kubernetes runs them as separate Deployments. Helm chart follows the world_model_store_service pattern with `values.yaml` (defaults), `values-staging.yaml`, and `values-production.yaml`. CI uses GitHub Actions (lint → test → build → security scan → docker build). CD triggers on version tags, builds multi-arch images, deploys to staging, runs smoke tests, then promotes to production with manual approval. Grafana dashboards cover service overview, Temporal workflows, paper source APIs, and LLM usage. Kafka topics are provisioned via Helm hooks.

**Tech Stack:** Docker (multi-stage alpine), Helm 3, GitHub Actions, Prometheus/Grafana, Kafka (Confluent), Kubernetes 1.28+, External Secrets Operator, cert-manager

---

## Task Overview

| Task | Deliverable | Description |
|------|-------------|-------------|
| 1 | D7.1 | Multi-stage Dockerfile for server + worker binaries |
| 2 | D7.2 | Docker Compose for local development |
| 3 | D7.2 | Makefile docker targets |
| 4 | D7.3 | Helm chart scaffold — Chart.yaml, helpers, base templates |
| 5 | D7.3 | Helm chart — server Deployment + Service + ConfigMap + Secret |
| 6 | D7.3 | Helm chart — worker Deployment |
| 7 | D7.4 | Helm values — staging and production overlays |
| 8 | D7.5 | External Secrets integration |
| 9 | D7.6 | HPA, PDB, and NetworkPolicy |
| 10 | D7.7 | Kafka topic provisioning Job |
| 11 | D7.8 | CI pipeline — GitHub Actions |
| 12 | D7.8 | CD pipeline — GitHub Actions |
| 13 | D7.9 | Operations runbook |
| 14 | D7.10 | Grafana dashboards — service overview |
| 15 | D7.10 | Grafana dashboards — Temporal workflows + paper sources |
| 16 | — | Final verification — build, lint, test |

---

### Task 1: Multi-Stage Dockerfile

**Files:**
- Create: `literature_service/Dockerfile`
- Create: `literature_service/.dockerignore`

**Step 1: Create .dockerignore**

```
# literature_service/.dockerignore
.git
.gitignore
*.md
docs/
tests/
testdata/
coverage.*
bin/
.idea/
.vscode/
*.test
docker-compose*.yml
```

**Step 2: Create the Dockerfile**

```dockerfile
# literature_service/Dockerfile

# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG BUILD_TIME
ARG COMMIT_SHA

# Build server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.CommitSHA=${COMMIT_SHA}" \
    -o /app/literature-review-server ./cmd/server

# Build worker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.CommitSHA=${COMMIT_SHA}" \
    -o /app/literature-review-worker ./cmd/worker

# Build migrate
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w" \
    -o /app/literature-review-migrate ./cmd/migrate

# Final stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/literature-review-server .
COPY --from=builder /app/literature-review-worker .
COPY --from=builder /app/literature-review-migrate .
COPY --from=builder /app/migrations ./migrations
COPY --from=builder /app/config ./config

RUN chown -R appuser:appgroup /app

USER appuser

# Default to server; override with --entrypoint for worker
EXPOSE 8080 9090 9091

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/literature-review-server"]
```

**Step 3: Verify Dockerfile builds locally**

Run: `cd literature_service && docker build -t literature-review-service:test .`
Expected: Build completes, image < 100MB

**Step 4: Verify both entrypoints work**

Run: `docker run --rm literature-review-service:test --help` (server)
Run: `docker run --rm --entrypoint /app/literature-review-worker literature-review-service:test --help` (worker)
Expected: Both print help/usage without crashing

**Step 5: Commit**

```bash
git add Dockerfile .dockerignore
git commit -m "feat(deploy): add multi-stage Dockerfile for server and worker"
```

---

### Task 2: Docker Compose for Local Development

**Files:**
- Create: `literature_service/docker-compose.yml`

**Step 1: Create docker-compose.yml**

```yaml
# literature_service/docker-compose.yml
# WARNING: This file is for LOCAL DEVELOPMENT ONLY.

version: '3.8'

services:
  postgres:
    image: postgres:16-alpine
    container_name: litreview-postgres
    environment:
      POSTGRES_USER: ${POSTGRES_USER:-litreview}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set}
      POSTGRES_DB: ${POSTGRES_DB:-literature_review}
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-litreview} -d ${POSTGRES_DB:-literature_review}"]
      interval: 5s
      timeout: 5s
      retries: 5

  temporal:
    image: temporalio/auto-setup:1.25.2
    container_name: litreview-temporal
    environment:
      - DB=postgres12
      - DB_PORT=5432
      - POSTGRES_USER=${POSTGRES_USER:-litreview}
      - POSTGRES_PWD=${POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set}
      - POSTGRES_SEEDS=postgres
    ports:
      - "7233:7233"
    depends_on:
      postgres:
        condition: service_healthy

  temporal-ui:
    image: temporalio/ui:latest
    container_name: litreview-temporal-ui
    environment:
      - TEMPORAL_ADDRESS=temporal:7233
      - TEMPORAL_CORS_ORIGINS=http://localhost:3000
    ports:
      - "8088:8080"
    depends_on:
      - temporal

  kafka:
    image: confluentinc/cp-kafka:7.6.0
    container_name: litreview-kafka
    environment:
      KAFKA_NODE_ID: 1
      KAFKA_LISTENER_SECURITY_PROTOCOL_MAP: CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://kafka:9092
      KAFKA_LISTENERS: PLAINTEXT://0.0.0.0:9092,CONTROLLER://0.0.0.0:9093
      KAFKA_CONTROLLER_LISTENER_NAMES: CONTROLLER
      KAFKA_CONTROLLER_QUORUM_VOTERS: 1@kafka:9093
      KAFKA_PROCESS_ROLES: broker,controller
      KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
      KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR: 1
      KAFKA_TRANSACTION_STATE_LOG_MIN_ISR: 1
      CLUSTER_ID: MkU3OEVBNTcwNTJENDM2Qk
    ports:
      - "9092:9092"
    healthcheck:
      test: ["CMD", "kafka-topics", "--bootstrap-server", "localhost:9092", "--list"]
      interval: 10s
      timeout: 10s
      retries: 5

  server:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: litreview-server
    environment:
      LITREVIEW_SERVER_HOST: "0.0.0.0"
      LITREVIEW_SERVER_HTTP_PORT: 8080
      LITREVIEW_SERVER_GRPC_PORT: 9090
      LITREVIEW_SERVER_METRICS_PORT: 9091
      LITREVIEW_DATABASE_HOST: postgres
      LITREVIEW_DATABASE_PORT: 5432
      LITREVIEW_DATABASE_USER: ${POSTGRES_USER:-litreview}
      LITREVIEW_DATABASE_PASSWORD: ${POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set}
      LITREVIEW_DATABASE_NAME: ${POSTGRES_DB:-literature_review}
      LITREVIEW_DATABASE_SSL_MODE: disable
      LITREVIEW_TEMPORAL_HOST_PORT: temporal:7233
      LITREVIEW_TEMPORAL_NAMESPACE: literature-review
      LITREVIEW_TEMPORAL_TASK_QUEUE: literature-review-tasks
      LITREVIEW_KAFKA_ENABLED: "true"
      LITREVIEW_KAFKA_BROKERS: kafka:9092
      LITREVIEW_KAFKA_TOPIC: events.outbox.literature_review_service
      LITREVIEW_LOGGING_LEVEL: debug
      LITREVIEW_LOGGING_FORMAT: json
    ports:
      - "8080:8080"
      - "9090:9090"
      - "9091:9091"
    depends_on:
      postgres:
        condition: service_healthy
      temporal:
        condition: service_started
      kafka:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3
    profiles:
      - full

  worker:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: litreview-worker
    entrypoint: ["/app/literature-review-worker"]
    environment:
      LITREVIEW_DATABASE_HOST: postgres
      LITREVIEW_DATABASE_PORT: 5432
      LITREVIEW_DATABASE_USER: ${POSTGRES_USER:-litreview}
      LITREVIEW_DATABASE_PASSWORD: ${POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set}
      LITREVIEW_DATABASE_NAME: ${POSTGRES_DB:-literature_review}
      LITREVIEW_DATABASE_SSL_MODE: disable
      LITREVIEW_TEMPORAL_HOST_PORT: temporal:7233
      LITREVIEW_TEMPORAL_NAMESPACE: literature-review
      LITREVIEW_TEMPORAL_TASK_QUEUE: literature-review-tasks
      LITREVIEW_KAFKA_ENABLED: "true"
      LITREVIEW_KAFKA_BROKERS: kafka:9092
      LITREVIEW_LOGGING_LEVEL: debug
      LITREVIEW_LOGGING_FORMAT: json
    depends_on:
      postgres:
        condition: service_healthy
      temporal:
        condition: service_started
      kafka:
        condition: service_healthy
    profiles:
      - full

volumes:
  postgres_data:

networks:
  default:
    name: litreview-network
```

**Step 2: Verify compose up for infrastructure only (default profile)**

Run: `POSTGRES_PASSWORD=devpassword docker compose up -d`
Expected: postgres, temporal, temporal-ui, kafka start. Server/worker NOT started (they use `full` profile).

**Step 3: Verify full profile starts all services**

Run: `POSTGRES_PASSWORD=devpassword docker compose --profile full up -d`
Expected: All 6 services start.

**Step 4: Commit**

```bash
git add docker-compose.yml
git commit -m "feat(deploy): add docker-compose for local development"
```

---

### Task 3: Makefile Docker Targets

**Files:**
- Modify: `literature_service/Makefile`

**Step 1: Add Docker variables and targets to Makefile**

Append to existing Makefile, after the `dev-setup` target:

```makefile
# Docker settings
DOCKER_REGISTRY ?= ghcr.io/helixir
DOCKER_IMAGE := $(DOCKER_REGISTRY)/literature-review-service
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DOCKER_TAG ?= $(VERSION)

## docker-build: Build Docker image
docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--build-arg COMMIT_SHA=$(COMMIT_SHA) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_IMAGE):latest

## docker-push: Push Docker image to registry
docker-push:
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_IMAGE):latest

## docker-run-server: Run server Docker container locally
docker-run-server:
	docker run --rm -p 8080:8080 -p 9090:9090 -p 9091:9091 \
		-e LITREVIEW_DATABASE_HOST=host.docker.internal \
		-e LITREVIEW_DATABASE_PASSWORD=devpassword \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

## docker-run-worker: Run worker Docker container locally
docker-run-worker:
	docker run --rm \
		--entrypoint /app/literature-review-worker \
		-e LITREVIEW_DATABASE_HOST=host.docker.internal \
		-e LITREVIEW_DATABASE_PASSWORD=devpassword \
		-e LITREVIEW_TEMPORAL_HOST_PORT=host.docker.internal:7233 \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

## compose-up: Start infrastructure services (postgres, temporal, kafka)
compose-up:
	POSTGRES_PASSWORD=devpassword docker compose up -d

## compose-up-full: Start all services including server and worker
compose-up-full:
	POSTGRES_PASSWORD=devpassword docker compose --profile full up -d

## compose-down: Stop all services
compose-down:
	docker compose --profile full down

## compose-logs: View logs
compose-logs:
	docker compose --profile full logs -f
```

**Step 2: Update .PHONY at top of Makefile**

Add: `docker-build docker-push docker-run-server docker-run-worker compose-up compose-up-full compose-down compose-logs`

**Step 3: Verify docker-build works**

Run: `make docker-build`
Expected: Image builds successfully

**Step 4: Commit**

```bash
git add Makefile
git commit -m "feat(deploy): add docker and compose targets to Makefile"
```

---

### Task 4: Helm Chart Scaffold

**Files:**
- Create: `literature_service/deploy/helm/literature-review-service/Chart.yaml`
- Create: `literature_service/deploy/helm/literature-review-service/templates/_helpers.tpl`
- Create: `literature_service/deploy/helm/literature-review-service/templates/NOTES.txt`
- Create: `literature_service/deploy/helm/literature-review-service/.helmignore`

**Step 1: Create Chart.yaml**

```yaml
# deploy/helm/literature-review-service/Chart.yaml
apiVersion: v2
name: literature-review-service
description: A Helm chart for the Literature Review Service — automated literature review with Temporal workflows
type: application
version: 1.0.0
appVersion: "1.0.0"
keywords:
  - literature-review
  - temporal
  - grpc
  - postgresql
  - kafka
home: https://github.com/deepbio/literature_service
maintainers:
  - name: DeepBio Team
    email: team@deepbio.dev
dependencies:
  - name: postgresql
    version: "15.5.0"
    repository: "https://charts.bitnami.com/bitnami"
    condition: postgresql.enabled
annotations:
  category: Science
  licenses: Proprietary
```

**Step 2: Create _helpers.tpl**

```yaml
# deploy/helm/literature-review-service/templates/_helpers.tpl
{{/*
Expand the name of the chart.
*/}}
{{- define "literature-review-service.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "literature-review-service.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "literature-review-service.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "literature-review-service.labels" -}}
helm.sh/chart: {{ include "literature-review-service.chart" . }}
{{ include "literature-review-service.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: helixir
{{- end }}

{{/*
Selector labels
*/}}
{{- define "literature-review-service.selectorLabels" -}}
app.kubernetes.io/name: {{ include "literature-review-service.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Server selector labels
*/}}
{{- define "literature-review-service.serverSelectorLabels" -}}
{{ include "literature-review-service.selectorLabels" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Worker selector labels
*/}}
{{- define "literature-review-service.workerSelectorLabels" -}}
{{ include "literature-review-service.selectorLabels" . }}
app.kubernetes.io/component: worker
{{- end }}

{{/*
Service account name
*/}}
{{- define "literature-review-service.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "literature-review-service.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Image reference
*/}}
{{- define "literature-review-service.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}

{{/*
Secret name
*/}}
{{- define "literature-review-service.secretName" -}}
{{- if .Values.externalSecrets.enabled }}
{{- .Values.externalSecrets.target.name | default (printf "%s-secrets" (include "literature-review-service.fullname" .)) }}
{{- else }}
{{- printf "%s-secrets" (include "literature-review-service.fullname" .) }}
{{- end }}
{{- end }}

{{/*
PostgreSQL host
*/}}
{{- define "literature-review-service.postgresql.host" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "%s-postgresql" (include "literature-review-service.fullname" .) }}
{{- else }}
{{- .Values.secrets.database.host }}
{{- end }}
{{- end }}

{{/*
PostgreSQL port
*/}}
{{- define "literature-review-service.postgresql.port" -}}
{{- if .Values.postgresql.enabled }}
{{- "5432" }}
{{- else }}
{{- .Values.secrets.database.port | default "5432" | toString }}
{{- end }}
{{- end }}

{{/*
PostgreSQL database
*/}}
{{- define "literature-review-service.postgresql.database" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.database }}
{{- else }}
{{- .Values.secrets.database.name }}
{{- end }}
{{- end }}
```

**Step 3: Create .helmignore**

```
# deploy/helm/literature-review-service/.helmignore
.git
.gitignore
*.md
tests/
```

**Step 4: Create NOTES.txt**

```
# deploy/helm/literature-review-service/templates/NOTES.txt
Literature Review Service has been deployed.

Server:
  kubectl port-forward svc/{{ include "literature-review-service.fullname" . }}-server {{ .Values.service.httpPort }}:{{ .Values.service.httpPort }} -n {{ .Release.Namespace }}

gRPC:
  kubectl port-forward svc/{{ include "literature-review-service.fullname" . }}-server {{ .Values.service.grpcPort }}:{{ .Values.service.grpcPort }} -n {{ .Release.Namespace }}

Metrics:
  kubectl port-forward svc/{{ include "literature-review-service.fullname" . }}-server {{ .Values.service.metricsPort }}:{{ .Values.service.metricsPort }} -n {{ .Release.Namespace }}
```

**Step 5: Commit**

```bash
git add deploy/
git commit -m "feat(deploy): scaffold Helm chart with helpers and Chart.yaml"
```

---

### Task 5: Helm Chart — Server Deployment + Service + ConfigMap + Secret

**Files:**
- Create: `literature_service/deploy/helm/literature-review-service/values.yaml`
- Create: `literature_service/deploy/helm/literature-review-service/templates/server-deployment.yaml`
- Create: `literature_service/deploy/helm/literature-review-service/templates/service.yaml`
- Create: `literature_service/deploy/helm/literature-review-service/templates/configmap.yaml`
- Create: `literature_service/deploy/helm/literature-review-service/templates/secret.yaml`
- Create: `literature_service/deploy/helm/literature-review-service/templates/serviceaccount.yaml`

**Step 1: Create values.yaml**

```yaml
# deploy/helm/literature-review-service/values.yaml

# Server deployment
server:
  replicaCount: 2
  resources:
    requests:
      memory: "256Mi"
      cpu: "250m"
    limits:
      memory: "1Gi"
      cpu: "1000m"

# Worker deployment
worker:
  replicaCount: 2
  resources:
    requests:
      memory: "512Mi"
      cpu: "500m"
    limits:
      memory: "2Gi"
      cpu: "2000m"

image:
  repository: ghcr.io/helixir/literature-review-service
  pullPolicy: IfNotPresent
  tag: ""

imagePullSecrets: []
nameOverride: ""
fullnameOverride: ""

serviceAccount:
  create: true
  annotations: {}
  name: ""

podAnnotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "9091"
  prometheus.io/path: "/metrics"

podSecurityContext:
  runAsNonRoot: true
  runAsUser: 1000
  runAsGroup: 1000
  fsGroup: 1000
  seccompProfile:
    type: RuntimeDefault

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  runAsNonRoot: true
  runAsUser: 1000
  capabilities:
    drop:
      - ALL

service:
  type: ClusterIP
  httpPort: 8080
  grpcPort: 9090
  metricsPort: 9091

ingress:
  enabled: false
  className: "nginx"
  annotations:
    nginx.ingress.kubernetes.io/backend-protocol: "GRPC"
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
  hosts:
    - host: literature-review.local
      paths:
        - path: /
          pathType: Prefix
  tls: []

autoscaling:
  server:
    enabled: true
    minReplicas: 2
    maxReplicas: 8
    targetCPUUtilization: 70
    targetMemoryUtilization: 80
    behavior:
      scaleDown:
        stabilizationWindowSeconds: 300
        policies:
          - type: Percent
            value: 10
            periodSeconds: 60
      scaleUp:
        stabilizationWindowSeconds: 0
        policies:
          - type: Percent
            value: 100
            periodSeconds: 15
          - type: Pods
            value: 4
            periodSeconds: 15
        selectPolicy: Max
  worker:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilization: 75
    targetMemoryUtilization: 85

podDisruptionBudget:
  server:
    enabled: true
    minAvailable: 1
  worker:
    enabled: true
    minAvailable: 1

livenessProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 15
  periodSeconds: 20
  timeoutSeconds: 5
  failureThreshold: 3
  successThreshold: 1

readinessProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 5
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3
  successThreshold: 1

startupProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 10
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 30

nodeSelector: {}
tolerations: []
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchExpressions:
              - key: app.kubernetes.io/name
                operator: In
                values:
                  - literature-review-service
          topologyKey: kubernetes.io/hostname

topologySpreadConstraints:
  - maxSkew: 1
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: ScheduleAnyway
    labelSelector:
      matchLabels:
        app.kubernetes.io/name: literature-review-service

terminationGracePeriodSeconds: 30

# Application configuration (stored in ConfigMap)
config:
  server:
    host: "0.0.0.0"
    httpPort: 8080
    grpcPort: 9090
    metricsPort: 9091
    shutdownTimeout: "30s"

  logging:
    level: "info"
    format: "json"

  database:
    maxConns: 50
    minConns: 10
    maxConnLifetime: "30m"
    maxConnIdleTime: "5m"
    sslMode: "require"

  temporal:
    namespace: "literature-review"
    taskQueue: "literature-review-tasks"

  kafka:
    enabled: true
    topic: "events.outbox.literature_review_service"

  outbox:
    pollInterval: "1s"
    batchSize: 100
    workers: 4

  metrics:
    enabled: true

  tracing:
    enabled: false
    endpoint: ""
    samplingRate: 0.1
    serviceName: "literature-review-service"

  paperSources:
    semanticScholar:
      enabled: true
      rps: 10
      burst: 20
    openalex:
      enabled: true
      rps: 10
      burst: 20
    pubmed:
      enabled: true
      rps: 3
      burst: 5

  llm:
    provider: "openai"
    model: "gpt-4-turbo"

# Secrets (stored in Secret)
secrets:
  database:
    host: ""
    port: 5432
    user: "litreview"
    password: ""
    name: "literature_review"
  temporal:
    hostPort: "temporal:7233"
  kafka:
    brokers: "kafka:9092"
  llm:
    openaiApiKey: ""
    anthropicApiKey: ""
  tls:
    enabled: false
    cert: ""
    key: ""
    ca: ""

externalSecrets:
  enabled: false
  secretStoreRef:
    name: ""
    kind: ClusterSecretStore
  refreshInterval: "1h"
  target:
    name: ""
    creationPolicy: Owner

serviceMonitor:
  enabled: true
  namespace: ""
  labels: {}
  interval: "30s"
  scrapeTimeout: "10s"

networkPolicy:
  enabled: false
  allowedNamespaces: []
  allowedPodLabels: {}

postgresql:
  enabled: true
  architecture: standalone
  auth:
    username: litreview
    password: ""
    database: literature_review
  primary:
    persistence:
      enabled: true
      size: 50Gi
    resources:
      requests:
        memory: "512Mi"
        cpu: "250m"
      limits:
        memory: "2Gi"
        cpu: "1000m"
  metrics:
    enabled: true
    serviceMonitor:
      enabled: true

kafka:
  topics:
    - name: "events.outbox.literature_review_service"
      partitions: 6
      replicationFactor: 1
      config:
        retention.ms: "604800000"
        cleanup.policy: "delete"

priorityClassName: ""
```

**Step 2: Create server-deployment.yaml**

```yaml
# deploy/helm/literature-review-service/templates/server-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "literature-review-service.fullname" . }}-server
  labels:
    {{- include "literature-review-service.labels" . | nindent 4 }}
    app.kubernetes.io/component: server
spec:
  {{- if not .Values.autoscaling.server.enabled }}
  replicas: {{ .Values.server.replicaCount }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "literature-review-service.serverSelectorLabels" . | nindent 6 }}
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 0
  template:
    metadata:
      annotations:
        checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
        checksum/secret: {{ include (print $.Template.BasePath "/secret.yaml") . | sha256sum }}
        {{- with .Values.podAnnotations }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      labels:
        {{- include "literature-review-service.labels" . | nindent 8 }}
        app.kubernetes.io/component: server
    spec:
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "literature-review-service.serviceAccountName" . }}
      {{- if .Values.priorityClassName }}
      priorityClassName: {{ .Values.priorityClassName }}
      {{- end }}
      securityContext:
        {{- toYaml .Values.podSecurityContext | nindent 8 }}
      terminationGracePeriodSeconds: {{ .Values.terminationGracePeriodSeconds }}
      containers:
        - name: server
          securityContext:
            {{- toYaml .Values.securityContext | nindent 12 }}
          image: {{ include "literature-review-service.image" . }}
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          command: ["/app/literature-review-server"]
          ports:
            - name: http
              containerPort: {{ .Values.config.server.httpPort }}
              protocol: TCP
            - name: grpc
              containerPort: {{ .Values.config.server.grpcPort }}
              protocol: TCP
            - name: metrics
              containerPort: {{ .Values.config.server.metricsPort }}
              protocol: TCP
          livenessProbe:
            {{- toYaml .Values.livenessProbe | nindent 12 }}
          readinessProbe:
            {{- toYaml .Values.readinessProbe | nindent 12 }}
          startupProbe:
            {{- toYaml .Values.startupProbe | nindent 12 }}
          resources:
            {{- toYaml .Values.server.resources | nindent 12 }}
          envFrom:
            - configMapRef:
                name: {{ include "literature-review-service.fullname" . }}
          env:
            - name: LITREVIEW_DATABASE_HOST
              value: {{ include "literature-review-service.postgresql.host" . | quote }}
            - name: LITREVIEW_DATABASE_PORT
              value: {{ include "literature-review-service.postgresql.port" . | quote }}
            - name: LITREVIEW_DATABASE_NAME
              value: {{ include "literature-review-service.postgresql.database" . | quote }}
            - name: LITREVIEW_DATABASE_USER
              valueFrom:
                secretKeyRef:
                  name: {{ include "literature-review-service.secretName" . }}
                  key: DATABASE_USER
            - name: LITREVIEW_DATABASE_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: {{ include "literature-review-service.secretName" . }}
                  key: DATABASE_PASSWORD
            - name: LITREVIEW_TEMPORAL_HOST_PORT
              valueFrom:
                secretKeyRef:
                  name: {{ include "literature-review-service.secretName" . }}
                  key: TEMPORAL_HOST_PORT
            - name: LITREVIEW_KAFKA_BROKERS
              valueFrom:
                secretKeyRef:
                  name: {{ include "literature-review-service.secretName" . }}
                  key: KAFKA_BROKERS
            - name: LITREVIEW_LLM_OPENAI_API_KEY
              valueFrom:
                secretKeyRef:
                  name: {{ include "literature-review-service.secretName" . }}
                  key: LLM_OPENAI_API_KEY
                  optional: true
            - name: LITREVIEW_LLM_ANTHROPIC_API_KEY
              valueFrom:
                secretKeyRef:
                  name: {{ include "literature-review-service.secretName" . }}
                  key: LLM_ANTHROPIC_API_KEY
                  optional: true
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
          volumeMounts:
            - name: tmp
              mountPath: /tmp
      volumes:
        - name: tmp
          emptyDir: {}
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.topologySpreadConstraints }}
      topologySpreadConstraints:
        {{- toYaml . | nindent 8 }}
      {{- end }}
```

**Step 3: Create service.yaml**

```yaml
# deploy/helm/literature-review-service/templates/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ include "literature-review-service.fullname" . }}-server
  labels:
    {{- include "literature-review-service.labels" . | nindent 4 }}
    app.kubernetes.io/component: server
spec:
  type: {{ .Values.service.type }}
  ports:
    - name: http
      port: {{ .Values.service.httpPort }}
      targetPort: http
      protocol: TCP
    - name: grpc
      port: {{ .Values.service.grpcPort }}
      targetPort: grpc
      protocol: TCP
    - name: metrics
      port: {{ .Values.service.metricsPort }}
      targetPort: metrics
      protocol: TCP
  selector:
    {{- include "literature-review-service.serverSelectorLabels" . | nindent 4 }}
```

**Step 4: Create configmap.yaml**

```yaml
# deploy/helm/literature-review-service/templates/configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "literature-review-service.fullname" . }}
  labels:
    {{- include "literature-review-service.labels" . | nindent 4 }}
data:
  LITREVIEW_SERVER_HOST: {{ .Values.config.server.host | quote }}
  LITREVIEW_SERVER_HTTP_PORT: {{ .Values.config.server.httpPort | quote }}
  LITREVIEW_SERVER_GRPC_PORT: {{ .Values.config.server.grpcPort | quote }}
  LITREVIEW_SERVER_METRICS_PORT: {{ .Values.config.server.metricsPort | quote }}
  LITREVIEW_SERVER_SHUTDOWN_TIMEOUT: {{ .Values.config.server.shutdownTimeout | quote }}
  LITREVIEW_DATABASE_MAX_CONNS: {{ .Values.config.database.maxConns | quote }}
  LITREVIEW_DATABASE_MIN_CONNS: {{ .Values.config.database.minConns | quote }}
  LITREVIEW_DATABASE_MAX_CONN_LIFETIME: {{ .Values.config.database.maxConnLifetime | quote }}
  LITREVIEW_DATABASE_MAX_CONN_IDLE_TIME: {{ .Values.config.database.maxConnIdleTime | quote }}
  LITREVIEW_DATABASE_SSL_MODE: {{ .Values.config.database.sslMode | quote }}
  LITREVIEW_TEMPORAL_NAMESPACE: {{ .Values.config.temporal.namespace | quote }}
  LITREVIEW_TEMPORAL_TASK_QUEUE: {{ .Values.config.temporal.taskQueue | quote }}
  LITREVIEW_KAFKA_ENABLED: {{ .Values.config.kafka.enabled | quote }}
  LITREVIEW_KAFKA_TOPIC: {{ .Values.config.kafka.topic | quote }}
  LITREVIEW_OUTBOX_POLL_INTERVAL: {{ .Values.config.outbox.pollInterval | quote }}
  LITREVIEW_OUTBOX_BATCH_SIZE: {{ .Values.config.outbox.batchSize | quote }}
  LITREVIEW_OUTBOX_WORKERS: {{ .Values.config.outbox.workers | quote }}
  LITREVIEW_LOGGING_LEVEL: {{ .Values.config.logging.level | quote }}
  LITREVIEW_LOGGING_FORMAT: {{ .Values.config.logging.format | quote }}
  LITREVIEW_METRICS_ENABLED: {{ .Values.config.metrics.enabled | quote }}
  LITREVIEW_TRACING_ENABLED: {{ .Values.config.tracing.enabled | quote }}
  LITREVIEW_TRACING_ENDPOINT: {{ .Values.config.tracing.endpoint | quote }}
  LITREVIEW_TRACING_SAMPLING_RATE: {{ .Values.config.tracing.samplingRate | quote }}
  LITREVIEW_TRACING_SERVICE_NAME: {{ .Values.config.tracing.serviceName | quote }}
  LITREVIEW_LLM_PROVIDER: {{ .Values.config.llm.provider | quote }}
  LITREVIEW_LLM_MODEL: {{ .Values.config.llm.model | quote }}
  LITREVIEW_PAPER_SOURCES_SEMANTIC_SCHOLAR_ENABLED: {{ .Values.config.paperSources.semanticScholar.enabled | quote }}
  LITREVIEW_PAPER_SOURCES_OPENALEX_ENABLED: {{ .Values.config.paperSources.openalex.enabled | quote }}
  LITREVIEW_PAPER_SOURCES_PUBMED_ENABLED: {{ .Values.config.paperSources.pubmed.enabled | quote }}
```

**Step 5: Create secret.yaml**

```yaml
# deploy/helm/literature-review-service/templates/secret.yaml
{{- if not .Values.externalSecrets.enabled }}
apiVersion: v1
kind: Secret
metadata:
  name: {{ include "literature-review-service.secretName" . }}
  labels:
    {{- include "literature-review-service.labels" . | nindent 4 }}
type: Opaque
stringData:
  DATABASE_USER: {{ .Values.secrets.database.user | quote }}
  DATABASE_PASSWORD: {{ .Values.secrets.database.password | quote }}
  TEMPORAL_HOST_PORT: {{ .Values.secrets.temporal.hostPort | quote }}
  KAFKA_BROKERS: {{ .Values.secrets.kafka.brokers | quote }}
  {{- if .Values.secrets.llm.openaiApiKey }}
  LLM_OPENAI_API_KEY: {{ .Values.secrets.llm.openaiApiKey | quote }}
  {{- end }}
  {{- if .Values.secrets.llm.anthropicApiKey }}
  LLM_ANTHROPIC_API_KEY: {{ .Values.secrets.llm.anthropicApiKey | quote }}
  {{- end }}
{{- end }}
```

**Step 6: Create serviceaccount.yaml**

```yaml
# deploy/helm/literature-review-service/templates/serviceaccount.yaml
{{- if .Values.serviceAccount.create }}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "literature-review-service.serviceAccountName" . }}
  labels:
    {{- include "literature-review-service.labels" . | nindent 4 }}
  {{- with .Values.serviceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
```

**Step 7: Validate Helm template renders**

Run: `helm template test deploy/helm/literature-review-service/`
Expected: YAML renders without errors

**Step 8: Commit**

```bash
git add deploy/
git commit -m "feat(deploy): add Helm chart server deployment, service, configmap, secret"
```

---

### Task 6: Helm Chart — Worker Deployment

**Files:**
- Create: `literature_service/deploy/helm/literature-review-service/templates/worker-deployment.yaml`

**Step 1: Create worker-deployment.yaml**

```yaml
# deploy/helm/literature-review-service/templates/worker-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "literature-review-service.fullname" . }}-worker
  labels:
    {{- include "literature-review-service.labels" . | nindent 4 }}
    app.kubernetes.io/component: worker
spec:
  {{- if not .Values.autoscaling.worker.enabled }}
  replicas: {{ .Values.worker.replicaCount }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "literature-review-service.workerSelectorLabels" . | nindent 6 }}
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 0
  template:
    metadata:
      annotations:
        checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
        checksum/secret: {{ include (print $.Template.BasePath "/secret.yaml") . | sha256sum }}
        {{- with .Values.podAnnotations }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      labels:
        {{- include "literature-review-service.labels" . | nindent 8 }}
        app.kubernetes.io/component: worker
    spec:
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "literature-review-service.serviceAccountName" . }}
      {{- if .Values.priorityClassName }}
      priorityClassName: {{ .Values.priorityClassName }}
      {{- end }}
      securityContext:
        {{- toYaml .Values.podSecurityContext | nindent 8 }}
      terminationGracePeriodSeconds: {{ .Values.terminationGracePeriodSeconds }}
      containers:
        - name: worker
          securityContext:
            {{- toYaml .Values.securityContext | nindent 12 }}
          image: {{ include "literature-review-service.image" . }}
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          command: ["/app/literature-review-worker"]
          ports:
            - name: metrics
              containerPort: {{ .Values.config.server.metricsPort }}
              protocol: TCP
          resources:
            {{- toYaml .Values.worker.resources | nindent 12 }}
          envFrom:
            - configMapRef:
                name: {{ include "literature-review-service.fullname" . }}
          env:
            - name: LITREVIEW_DATABASE_HOST
              value: {{ include "literature-review-service.postgresql.host" . | quote }}
            - name: LITREVIEW_DATABASE_PORT
              value: {{ include "literature-review-service.postgresql.port" . | quote }}
            - name: LITREVIEW_DATABASE_NAME
              value: {{ include "literature-review-service.postgresql.database" . | quote }}
            - name: LITREVIEW_DATABASE_USER
              valueFrom:
                secretKeyRef:
                  name: {{ include "literature-review-service.secretName" . }}
                  key: DATABASE_USER
            - name: LITREVIEW_DATABASE_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: {{ include "literature-review-service.secretName" . }}
                  key: DATABASE_PASSWORD
            - name: LITREVIEW_TEMPORAL_HOST_PORT
              valueFrom:
                secretKeyRef:
                  name: {{ include "literature-review-service.secretName" . }}
                  key: TEMPORAL_HOST_PORT
            - name: LITREVIEW_KAFKA_BROKERS
              valueFrom:
                secretKeyRef:
                  name: {{ include "literature-review-service.secretName" . }}
                  key: KAFKA_BROKERS
            - name: LITREVIEW_LLM_OPENAI_API_KEY
              valueFrom:
                secretKeyRef:
                  name: {{ include "literature-review-service.secretName" . }}
                  key: LLM_OPENAI_API_KEY
                  optional: true
            - name: LITREVIEW_LLM_ANTHROPIC_API_KEY
              valueFrom:
                secretKeyRef:
                  name: {{ include "literature-review-service.secretName" . }}
                  key: LLM_ANTHROPIC_API_KEY
                  optional: true
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
          volumeMounts:
            - name: tmp
              mountPath: /tmp
      volumes:
        - name: tmp
          emptyDir: {}
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
```

**Step 2: Validate Helm template renders with both deployments**

Run: `helm template test deploy/helm/literature-review-service/ | grep "kind: Deployment" | wc -l`
Expected: `2` (server + worker)

**Step 3: Commit**

```bash
git add deploy/
git commit -m "feat(deploy): add Helm chart worker deployment"
```

---

### Task 7: Helm Values — Staging and Production Overlays

**Files:**
- Create: `literature_service/deploy/helm/literature-review-service/values-staging.yaml`
- Create: `literature_service/deploy/helm/literature-review-service/values-production.yaml`

**Step 1: Create values-staging.yaml**

```yaml
# deploy/helm/literature-review-service/values-staging.yaml

server:
  replicaCount: 2
  resources:
    requests:
      memory: "256Mi"
      cpu: "250m"
    limits:
      memory: "1Gi"
      cpu: "1000m"

worker:
  replicaCount: 2
  resources:
    requests:
      memory: "512Mi"
      cpu: "500m"
    limits:
      memory: "2Gi"
      cpu: "2000m"

image:
  pullPolicy: Always

autoscaling:
  server:
    enabled: true
    minReplicas: 2
    maxReplicas: 6
    targetCPUUtilization: 70
  worker:
    enabled: true
    minReplicas: 2
    maxReplicas: 8

podDisruptionBudget:
  server:
    enabled: true
    minAvailable: 1
  worker:
    enabled: true
    minAvailable: 1

ingress:
  enabled: true
  className: "nginx"
  annotations:
    nginx.ingress.kubernetes.io/backend-protocol: "GRPC"
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    cert-manager.io/cluster-issuer: "letsencrypt-staging"
  hosts:
    - host: literature-review.staging.helixir.dev
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: literature-review-staging-tls
      hosts:
        - literature-review.staging.helixir.dev

config:
  logging:
    level: "debug"

  tracing:
    enabled: true
    samplingRate: 0.5
    environment: "staging"

  database:
    maxConns: 30
    minConns: 5
    sslMode: "require"

externalSecrets:
  enabled: true
  secretStoreRef:
    name: "aws-secretsmanager"
    kind: ClusterSecretStore
  refreshInterval: "30m"
  target:
    name: "literature-review-staging-secrets"
    creationPolicy: Owner

serviceMonitor:
  enabled: true
  interval: "30s"
  labels:
    release: prometheus

networkPolicy:
  enabled: false

postgresql:
  enabled: true
  architecture: standalone
  auth:
    database: literature_review_staging
  primary:
    persistence:
      size: 20Gi
    resources:
      requests:
        memory: "512Mi"
        cpu: "250m"
      limits:
        memory: "2Gi"
        cpu: "1000m"
```

**Step 2: Create values-production.yaml**

```yaml
# deploy/helm/literature-review-service/values-production.yaml

server:
  replicaCount: 3
  resources:
    requests:
      memory: "512Mi"
      cpu: "500m"
    limits:
      memory: "2Gi"
      cpu: "2000m"

worker:
  replicaCount: 4
  resources:
    requests:
      memory: "1Gi"
      cpu: "1000m"
    limits:
      memory: "4Gi"
      cpu: "4000m"

image:
  pullPolicy: IfNotPresent

autoscaling:
  server:
    enabled: true
    minReplicas: 3
    maxReplicas: 12
    targetCPUUtilization: 65
    targetMemoryUtilization: 75
    behavior:
      scaleDown:
        stabilizationWindowSeconds: 600
        policies:
          - type: Percent
            value: 5
            periodSeconds: 120
      scaleUp:
        stabilizationWindowSeconds: 60
        policies:
          - type: Percent
            value: 50
            periodSeconds: 60
          - type: Pods
            value: 4
            periodSeconds: 60
        selectPolicy: Max
  worker:
    enabled: true
    minReplicas: 4
    maxReplicas: 20
    targetCPUUtilization: 70
    targetMemoryUtilization: 80

podDisruptionBudget:
  server:
    enabled: true
    minAvailable: 2
  worker:
    enabled: true
    minAvailable: 2

podAnnotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "9091"
  prometheus.io/path: "/metrics"
  cluster-autoscaler.kubernetes.io/safe-to-evict: "false"

ingress:
  enabled: true
  className: "nginx"
  annotations:
    nginx.ingress.kubernetes.io/backend-protocol: "GRPC"
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
  hosts:
    - host: literature-review.helixir.dev
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: literature-review-tls
      hosts:
        - literature-review.helixir.dev

affinity:
  podAntiAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
            - key: app.kubernetes.io/name
              operator: In
              values:
                - literature-review-service
        topologyKey: kubernetes.io/hostname

topologySpreadConstraints:
  - maxSkew: 1
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: DoNotSchedule
    labelSelector:
      matchLabels:
        app.kubernetes.io/name: literature-review-service

priorityClassName: "high-priority"
terminationGracePeriodSeconds: 60

config:
  logging:
    level: "info"

  database:
    maxConns: 100
    minConns: 25
    sslMode: "require"

  outbox:
    pollInterval: "500ms"
    batchSize: 200
    workers: 8

  tracing:
    enabled: true
    samplingRate: 0.1
    environment: "production"

externalSecrets:
  enabled: true
  secretStoreRef:
    name: "aws-secretsmanager"
    kind: ClusterSecretStore
  refreshInterval: "1h"
  target:
    name: "literature-review-production-secrets"
    creationPolicy: Owner

serviceMonitor:
  enabled: true
  interval: "15s"
  labels:
    release: prometheus

networkPolicy:
  enabled: true
  allowedNamespaces:
    - "default"
    - "ingress-nginx"
    - "monitoring"
  allowedPodLabels:
    app.kubernetes.io/part-of: helixir

postgresql:
  enabled: true
  architecture: replication
  auth:
    database: literature_review
  primary:
    persistence:
      enabled: true
      size: 200Gi
      storageClass: "gp3-encrypted"
    resources:
      requests:
        memory: "2Gi"
        cpu: "1000m"
      limits:
        memory: "8Gi"
        cpu: "4000m"
    configuration: |
      max_connections = 300
      shared_buffers = 2GB
      effective_cache_size = 6GB
      maintenance_work_mem = 512MB
      wal_buffers = 128MB
      random_page_cost = 1.1
  readReplicas:
    replicaCount: 2
    persistence:
      enabled: true
      size: 200Gi
      storageClass: "gp3-encrypted"
  metrics:
    enabled: true
    serviceMonitor:
      enabled: true

kafka:
  topics:
    - name: "events.outbox.literature_review_service"
      partitions: 12
      replicationFactor: 3
      config:
        retention.ms: "604800000"
        cleanup.policy: "delete"
        min.insync.replicas: "2"
```

**Step 3: Validate both overlays render correctly**

Run: `helm template test deploy/helm/literature-review-service/ -f deploy/helm/literature-review-service/values-staging.yaml`
Run: `helm template test deploy/helm/literature-review-service/ -f deploy/helm/literature-review-service/values-production.yaml`
Expected: Both render without errors

**Step 4: Commit**

```bash
git add deploy/
git commit -m "feat(deploy): add staging and production Helm value overlays"
```

---

### Task 8: External Secrets Integration

**Files:**
- Create: `literature_service/deploy/helm/literature-review-service/templates/externalsecret.yaml`

**Step 1: Create externalsecret.yaml**

```yaml
# deploy/helm/literature-review-service/templates/externalsecret.yaml
{{- if .Values.externalSecrets.enabled }}
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: {{ include "literature-review-service.fullname" . }}
  labels:
    {{- include "literature-review-service.labels" . | nindent 4 }}
spec:
  refreshInterval: {{ .Values.externalSecrets.refreshInterval }}
  secretStoreRef:
    name: {{ .Values.externalSecrets.secretStoreRef.name }}
    kind: {{ .Values.externalSecrets.secretStoreRef.kind }}
  target:
    name: {{ .Values.externalSecrets.target.name | default (printf "%s-secrets" (include "literature-review-service.fullname" .)) }}
    creationPolicy: {{ .Values.externalSecrets.target.creationPolicy }}
  data:
    - secretKey: DATABASE_USER
      remoteRef:
        key: helixir/literature-review-service/database
        property: user
    - secretKey: DATABASE_PASSWORD
      remoteRef:
        key: helixir/literature-review-service/database
        property: password
    - secretKey: TEMPORAL_HOST_PORT
      remoteRef:
        key: helixir/literature-review-service/temporal
        property: host_port
    - secretKey: KAFKA_BROKERS
      remoteRef:
        key: helixir/literature-review-service/kafka
        property: brokers
    - secretKey: LLM_OPENAI_API_KEY
      remoteRef:
        key: helixir/literature-review-service/llm
        property: openai_api_key
    - secretKey: LLM_ANTHROPIC_API_KEY
      remoteRef:
        key: helixir/literature-review-service/llm
        property: anthropic_api_key
{{- end }}
```

**Step 2: Commit**

```bash
git add deploy/
git commit -m "feat(deploy): add External Secrets integration for secrets management"
```

---

### Task 9: HPA, PDB, NetworkPolicy, and ServiceMonitor

**Files:**
- Create: `literature_service/deploy/helm/literature-review-service/templates/server-hpa.yaml`
- Create: `literature_service/deploy/helm/literature-review-service/templates/worker-hpa.yaml`
- Create: `literature_service/deploy/helm/literature-review-service/templates/server-pdb.yaml`
- Create: `literature_service/deploy/helm/literature-review-service/templates/worker-pdb.yaml`
- Create: `literature_service/deploy/helm/literature-review-service/templates/networkpolicy.yaml`
- Create: `literature_service/deploy/helm/literature-review-service/templates/servicemonitor.yaml`

**Step 1: Create server-hpa.yaml**

```yaml
{{- if .Values.autoscaling.server.enabled }}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ include "literature-review-service.fullname" . }}-server
  labels:
    {{- include "literature-review-service.labels" . | nindent 4 }}
    app.kubernetes.io/component: server
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ include "literature-review-service.fullname" . }}-server
  minReplicas: {{ .Values.autoscaling.server.minReplicas }}
  maxReplicas: {{ .Values.autoscaling.server.maxReplicas }}
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: {{ .Values.autoscaling.server.targetCPUUtilization }}
    {{- if .Values.autoscaling.server.targetMemoryUtilization }}
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: {{ .Values.autoscaling.server.targetMemoryUtilization }}
    {{- end }}
  {{- with .Values.autoscaling.server.behavior }}
  behavior:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
```

**Step 2: Create worker-hpa.yaml** (same pattern, replace `server` with `worker`)

```yaml
{{- if .Values.autoscaling.worker.enabled }}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ include "literature-review-service.fullname" . }}-worker
  labels:
    {{- include "literature-review-service.labels" . | nindent 4 }}
    app.kubernetes.io/component: worker
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ include "literature-review-service.fullname" . }}-worker
  minReplicas: {{ .Values.autoscaling.worker.minReplicas }}
  maxReplicas: {{ .Values.autoscaling.worker.maxReplicas }}
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: {{ .Values.autoscaling.worker.targetCPUUtilization }}
    {{- if .Values.autoscaling.worker.targetMemoryUtilization }}
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: {{ .Values.autoscaling.worker.targetMemoryUtilization }}
    {{- end }}
{{- end }}
```

**Step 3: Create server-pdb.yaml and worker-pdb.yaml**

```yaml
# server-pdb.yaml
{{- if .Values.podDisruptionBudget.server.enabled }}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "literature-review-service.fullname" . }}-server
  labels:
    {{- include "literature-review-service.labels" . | nindent 4 }}
spec:
  minAvailable: {{ .Values.podDisruptionBudget.server.minAvailable }}
  selector:
    matchLabels:
      {{- include "literature-review-service.serverSelectorLabels" . | nindent 6 }}
{{- end }}
```

```yaml
# worker-pdb.yaml
{{- if .Values.podDisruptionBudget.worker.enabled }}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "literature-review-service.fullname" . }}-worker
  labels:
    {{- include "literature-review-service.labels" . | nindent 4 }}
spec:
  minAvailable: {{ .Values.podDisruptionBudget.worker.minAvailable }}
  selector:
    matchLabels:
      {{- include "literature-review-service.workerSelectorLabels" . | nindent 6 }}
{{- end }}
```

**Step 4: Create networkpolicy.yaml**

```yaml
{{- if .Values.networkPolicy.enabled }}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ include "literature-review-service.fullname" . }}
  labels:
    {{- include "literature-review-service.labels" . | nindent 4 }}
spec:
  podSelector:
    matchLabels:
      {{- include "literature-review-service.selectorLabels" . | nindent 6 }}
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        {{- range .Values.networkPolicy.allowedNamespaces }}
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: {{ . }}
        {{- end }}
        {{- if .Values.networkPolicy.allowedPodLabels }}
        - podSelector:
            matchLabels:
              {{- toYaml .Values.networkPolicy.allowedPodLabels | nindent 14 }}
        {{- end }}
      ports:
        - port: {{ .Values.config.server.httpPort }}
        - port: {{ .Values.config.server.grpcPort }}
        - port: {{ .Values.config.server.metricsPort }}
  egress:
    - {}
{{- end }}
```

**Step 5: Create servicemonitor.yaml**

```yaml
{{- if .Values.serviceMonitor.enabled }}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: {{ include "literature-review-service.fullname" . }}
  {{- if .Values.serviceMonitor.namespace }}
  namespace: {{ .Values.serviceMonitor.namespace }}
  {{- end }}
  labels:
    {{- include "literature-review-service.labels" . | nindent 4 }}
    {{- with .Values.serviceMonitor.labels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
spec:
  selector:
    matchLabels:
      {{- include "literature-review-service.selectorLabels" . | nindent 6 }}
  endpoints:
    - port: metrics
      interval: {{ .Values.serviceMonitor.interval }}
      scrapeTimeout: {{ .Values.serviceMonitor.scrapeTimeout }}
      path: /metrics
{{- end }}
```

**Step 6: Validate all templates render**

Run: `helm template test deploy/helm/literature-review-service/ -f deploy/helm/literature-review-service/values-production.yaml | grep "kind:" | sort | uniq -c`
Expected: ConfigMap, Deployment(2), ExternalSecret, HPA(2), NetworkPolicy, PDB(2), Secret, Service, ServiceAccount, ServiceMonitor

**Step 7: Commit**

```bash
git add deploy/
git commit -m "feat(deploy): add HPA, PDB, NetworkPolicy, ServiceMonitor templates"
```

---

### Task 10: Kafka Topic Provisioning Job

**Files:**
- Create: `literature_service/deploy/helm/literature-review-service/templates/kafka-topics-job.yaml`

**Step 1: Create kafka-topics-job.yaml**

```yaml
# deploy/helm/literature-review-service/templates/kafka-topics-job.yaml
{{- if .Values.config.kafka.enabled }}
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ include "literature-review-service.fullname" . }}-kafka-topics
  labels:
    {{- include "literature-review-service.labels" . | nindent 4 }}
  annotations:
    "helm.sh/hook": post-install,post-upgrade
    "helm.sh/hook-weight": "0"
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
spec:
  backoffLimit: 3
  template:
    metadata:
      labels:
        {{- include "literature-review-service.selectorLabels" . | nindent 8 }}
    spec:
      restartPolicy: Never
      containers:
        - name: create-topics
          image: confluentinc/cp-kafka:7.6.0
          command:
            - /bin/bash
            - -c
            - |
              {{- range .Values.kafka.topics }}
              echo "Creating topic {{ .name }}..."
              kafka-topics --create \
                --bootstrap-server $KAFKA_BROKERS \
                --topic {{ .name }} \
                --partitions {{ .partitions }} \
                --replication-factor {{ .replicationFactor }} \
                {{- range $key, $value := .config }}
                --config {{ $key }}={{ $value }} \
                {{- end }}
                --if-not-exists
              {{- end }}
              echo "All topics created."
          env:
            - name: KAFKA_BROKERS
              valueFrom:
                secretKeyRef:
                  name: {{ include "literature-review-service.secretName" . }}
                  key: KAFKA_BROKERS
{{- end }}
```

**Step 2: Commit**

```bash
git add deploy/
git commit -m "feat(deploy): add Kafka topic provisioning Helm hook"
```

---

### Task 11: CI Pipeline — GitHub Actions

**Files:**
- Create: `literature_service/.github/workflows/ci.yml`

**Step 1: Create ci.yml**

```yaml
# .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

env:
  GO_VERSION: '1.25'
  GOLANGCI_LINT_VERSION: 'v1.62.0'

jobs:
  lint:
    name: Lint
    runs-on: ubuntu-latest
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true
          cache-dependency-path: go.sum
      - run: go mod download
      - uses: golangci/golangci-lint-action@v6
        with:
          version: ${{ env.GOLANGCI_LINT_VERSION }}
          args: --timeout=5m

  test:
    name: Unit Tests
    runs-on: ubuntu-latest
    timeout-minutes: 20
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true
          cache-dependency-path: go.sum
      - run: go mod download
      - name: Run unit tests
        run: go test -v -race -short -coverprofile=coverage.out -covermode=atomic ./...
      - name: Generate coverage report
        run: go tool cover -func=coverage.out
      - uses: codecov/codecov-action@v4
        with:
          token: ${{ secrets.CODECOV_TOKEN }}
          files: ./coverage.out
          flags: unittests
          fail_ci_if_error: false

  integration-test:
    name: Integration Tests
    runs-on: ubuntu-latest
    timeout-minutes: 30
    env:
      TEST_DB_USER: testuser
      TEST_DB_PASS: testpass
      TEST_DB_NAME: testdb
    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_USER: testuser
          POSTGRES_PASSWORD: testpass
          POSTGRES_DB: testdb
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true
      - run: go mod download
      - name: Mask credentials
        run: |
          echo "::add-mask::${{ env.TEST_DB_USER }}"
          echo "::add-mask::${{ env.TEST_DB_PASS }}"
      - name: Run integration tests
        env:
          LITREVIEW_DATABASE_HOST: localhost
          LITREVIEW_DATABASE_PORT: 5432
          LITREVIEW_DATABASE_USER: testuser
          LITREVIEW_DATABASE_PASSWORD: testpass
          LITREVIEW_DATABASE_NAME: testdb
          LITREVIEW_DATABASE_SSL_MODE: disable
        run: |
          go test -v -race -tags=integration -timeout 20m \
            -coverprofile=coverage-integration.out ./tests/integration/...

  security-scan:
    name: Security Scan
    runs-on: ubuntu-latest
    timeout-minutes: 20
    permissions:
      security-events: write
      actions: read
      contents: read
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true
      - run: go mod download
      - uses: securego/gosec@master
        with:
          args: '-no-fail -fmt sarif -out gosec-results.sarif ./...'
      - uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: gosec-results.sarif
        if: always()
      - name: Run govulncheck
        run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./... || true

  build:
    name: Build
    runs-on: ubuntu-latest
    timeout-minutes: 15
    strategy:
      matrix:
        goos: [linux]
        goarch: [amd64, arm64]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true
      - run: go mod download
      - name: Build binaries
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
          CGO_ENABLED: 0
        run: |
          go build -ldflags="-s -w -X main.Version=${{ github.sha }}" \
            -o bin/server-${{ matrix.goos }}-${{ matrix.goarch }} ./cmd/server
          go build -ldflags="-s -w -X main.Version=${{ github.sha }}" \
            -o bin/worker-${{ matrix.goos }}-${{ matrix.goarch }} ./cmd/worker
      - uses: actions/upload-artifact@v4
        with:
          name: binaries-${{ matrix.goos }}-${{ matrix.goarch }}
          path: bin/
          retention-days: 7

  docker-build:
    name: Docker Build
    runs-on: ubuntu-latest
    timeout-minutes: 20
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/build-push-action@v6
        with:
          context: .
          push: false
          tags: literature-review-service:test
          cache-from: type=gha
          cache-to: type=gha,mode=max
      - uses: aquasecurity/trivy-action@master
        with:
          image-ref: 'literature-review-service:test'
          format: 'table'
          exit-code: '0'
          severity: 'CRITICAL,HIGH'

  ci-summary:
    name: CI Summary
    runs-on: ubuntu-latest
    needs: [lint, test, integration-test, security-scan, build, docker-build]
    if: always()
    steps:
      - name: Check results
        run: |
          echo "Lint: ${{ needs.lint.result }}"
          echo "Tests: ${{ needs.test.result }}"
          echo "Integration: ${{ needs.integration-test.result }}"
          echo "Security: ${{ needs.security-scan.result }}"
          echo "Build: ${{ needs.build.result }}"
          echo "Docker: ${{ needs.docker-build.result }}"
      - name: Fail if critical jobs failed
        if: |
          needs.lint.result == 'failure' ||
          needs.test.result == 'failure' ||
          needs.build.result == 'failure' ||
          needs.docker-build.result == 'failure'
        run: exit 1
```

**Step 2: Commit**

```bash
git add .github/
git commit -m "feat(deploy): add CI pipeline with lint, test, build, security scan"
```

---

### Task 12: CD Pipeline — GitHub Actions

**Files:**
- Create: `literature_service/.github/workflows/cd.yml`

**Step 1: Create cd.yml**

```yaml
# .github/workflows/cd.yml
name: CD

on:
  push:
    tags:
      - 'v*'

concurrency:
  group: cd-${{ github.ref }}
  cancel-in-progress: false

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}
  GO_VERSION: '1.25'
  HELM_VERSION: '3.14.0'

jobs:
  validate-tag:
    name: Validate Tag
    runs-on: ubuntu-latest
    outputs:
      version: ${{ steps.get_version.outputs.version }}
      is_prerelease: ${{ steps.check.outputs.is_prerelease }}
    steps:
      - name: Get version
        id: get_version
        run: |
          VERSION=${GITHUB_REF#refs/tags/v}
          echo "version=$VERSION" >> $GITHUB_OUTPUT
      - name: Check prerelease
        id: check
        run: |
          if [[ "${{ github.ref }}" == *"-alpha"* ]] || \
             [[ "${{ github.ref }}" == *"-beta"* ]] || \
             [[ "${{ github.ref }}" == *"-rc"* ]]; then
            echo "is_prerelease=true" >> $GITHUB_OUTPUT
          else
            echo "is_prerelease=false" >> $GITHUB_OUTPUT
          fi

  build-image:
    name: Build & Push Docker Image
    runs-on: ubuntu-latest
    needs: validate-tag
    permissions:
      contents: read
      packages: write
    outputs:
      digest: ${{ steps.build.outputs.digest }}
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=sha,prefix=
            type=raw,value=latest,enable=${{ needs.validate-tag.outputs.is_prerelease == 'false' }}
      - id: build
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          platforms: linux/amd64,linux/arm64
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
          build-args: |
            VERSION=${{ needs.validate-tag.outputs.version }}
            COMMIT_SHA=${{ github.sha }}

  deploy-staging:
    name: Deploy to Staging
    runs-on: ubuntu-latest
    needs: [validate-tag, build-image]
    environment:
      name: staging
      url: https://literature-review.staging.helixir.dev
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4
        with:
          version: ${{ env.HELM_VERSION }}
      - uses: azure/k8s-set-context@v4
        with:
          kubeconfig: ${{ secrets.STAGING_KUBECONFIG }}
      - name: Deploy to staging
        run: |
          helm upgrade --install literature-review-service \
            ./deploy/helm/literature-review-service \
            --namespace literature-review-staging \
            --create-namespace \
            --set image.repository=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }} \
            --set image.tag=${{ needs.validate-tag.outputs.version }} \
            --values ./deploy/helm/literature-review-service/values-staging.yaml \
            --wait --timeout 10m
      - name: Verify deployment
        run: |
          kubectl rollout status deployment/literature-review-service-server -n literature-review-staging --timeout=5m
          kubectl rollout status deployment/literature-review-service-worker -n literature-review-staging --timeout=5m

  smoke-test-staging:
    name: Staging Smoke Test
    runs-on: ubuntu-latest
    needs: [validate-tag, deploy-staging]
    environment: staging
    steps:
      - name: Health check
        run: |
          for i in {1..10}; do
            if curl -sf "https://literature-review.staging.helixir.dev/health"; then
              echo "Health check passed"
              exit 0
            fi
            echo "Attempt $i failed, retrying..."
            sleep 10
          done
          exit 1

  deploy-production:
    name: Deploy to Production
    runs-on: ubuntu-latest
    needs: [validate-tag, build-image, smoke-test-staging]
    if: needs.validate-tag.outputs.is_prerelease == 'false'
    environment:
      name: production
      url: https://literature-review.helixir.dev
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4
        with:
          version: ${{ env.HELM_VERSION }}
      - uses: azure/k8s-set-context@v4
        with:
          kubeconfig: ${{ secrets.PRODUCTION_KUBECONFIG }}
      - name: Deploy to production
        run: |
          helm upgrade --install literature-review-service \
            ./deploy/helm/literature-review-service \
            --namespace literature-review-production \
            --create-namespace \
            --set image.repository=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }} \
            --set image.tag=${{ needs.validate-tag.outputs.version }} \
            --values ./deploy/helm/literature-review-service/values-production.yaml \
            --wait --timeout 15m
      - name: Verify deployment
        run: |
          kubectl rollout status deployment/literature-review-service-server -n literature-review-production --timeout=10m
          kubectl rollout status deployment/literature-review-service-worker -n literature-review-production --timeout=10m

  rollback:
    name: Rollback on Failure
    runs-on: ubuntu-latest
    if: ${{ always() && needs.deploy-production.result == 'failure' }}
    needs: [validate-tag, deploy-production]
    environment: production
    steps:
      - uses: azure/k8s-set-context@v4
        with:
          kubeconfig: ${{ secrets.PRODUCTION_KUBECONFIG }}
      - run: helm rollback literature-review-service -n literature-review-production
```

**Step 2: Commit**

```bash
git add .github/
git commit -m "feat(deploy): add CD pipeline with staging, production, rollback"
```

---

### Task 13: Operations Runbook

**Files:**
- Create: `literature_service/docs/runbook.md`

**Step 1: Create the runbook**

Write a runbook covering:
- Service overview (what it does, ports, dependencies)
- Health check endpoints (`GET /health`)
- How to check service status (`kubectl get pods`, `kubectl logs`)
- Common failure modes and resolutions:
  - Database connection failures → check pg connection, run migrations
  - Temporal connection failures → check temporal namespace exists
  - Kafka connection failures → verify broker connectivity, topic existence
  - LLM API failures → check API key validity, rate limits
  - Paper source API rate limiting → check rate limiter metrics, backoff config
- How to scale (`kubectl scale`, HPA thresholds)
- How to rollback (`helm rollback`)
- How to run migrations manually
- Useful queries (active reviews, failed workflows, outbox backlog)
- Alert response procedures
- Key metrics to watch

**Step 2: Commit**

```bash
git add docs/
git commit -m "docs: add operations runbook for on-call engineers"
```

---

### Task 14: Grafana Dashboard — Service Overview

**Files:**
- Create: `literature_service/deploy/grafana/dashboards/service-overview.json`
- Create: `literature_service/deploy/grafana/provisioning/dashboards.yml`

**Step 1: Create provisioning config**

```yaml
# deploy/grafana/provisioning/dashboards.yml
apiVersion: 1
providers:
  - name: 'literature-review-service'
    orgId: 1
    folder: 'Literature Review Service'
    type: file
    disableDeletion: false
    updateIntervalSeconds: 30
    options:
      path: /var/lib/grafana/dashboards/literature-review-service
```

**Step 2: Create service-overview.json**

Create a Grafana dashboard JSON with panels:
- **Row 1: Overview** — Reviews started/completed/failed counters, active reviews gauge
- **Row 2: Performance** — Review duration histogram (p50/p95/p99), papers discovered rate
- **Row 3: API** — HTTP request rate, gRPC request rate, error rate percentage
- **Row 4: Database** — Connection pool utilization, query duration (p50/p95), outbox queue size
- **Row 5: Infrastructure** — CPU usage, memory usage, pod count, restart count

All panels use the `literature_review_` metric prefix from `internal/observability/metrics.go`.
Datasource: `prometheus` (variable `$datasource`).
Time range variable, namespace variable, pod variable.

**Step 3: Commit**

```bash
git add deploy/grafana/
git commit -m "feat(deploy): add Grafana service overview dashboard"
```

---

### Task 15: Grafana Dashboard — Temporal Workflows + Paper Sources

**Files:**
- Create: `literature_service/deploy/grafana/dashboards/temporal-workflows.json`
- Create: `literature_service/deploy/grafana/dashboards/paper-sources.json`

**Step 1: Create temporal-workflows.json**

Dashboard panels:
- Workflow start rate (`literature_review_workflows_started_total`)
- Workflow completion rate by status
- Workflow duration histogram
- Active workflows gauge
- LLM request rate by provider
- LLM latency (p50/p95/p99)
- LLM token usage rate by provider and type
- Rate limit backoff rate by source
- Rate limit wait time histogram

**Step 2: Create paper-sources.json**

Dashboard panels:
- Search request rate by source
- Search latency by source (p50/p95/p99)
- Papers discovered per source
- Error rate by source
- Rate limiting events by source
- Ingestion request success/failure rate

**Step 3: Commit**

```bash
git add deploy/grafana/
git commit -m "feat(deploy): add Temporal workflow and paper source Grafana dashboards"
```

---

### Task 16: Final Verification

**Step 1: Verify all binaries build**

Run: `make build`
Expected: All 3 binaries in `bin/`

**Step 2: Verify Docker image builds**

Run: `make docker-build`
Expected: Image builds, tagged with version and latest

**Step 3: Verify Helm chart lints**

Run: `helm lint deploy/helm/literature-review-service/`
Expected: `0 chart(s) failed`

**Step 4: Verify Helm chart renders for all environments**

Run: `helm template test deploy/helm/literature-review-service/`
Run: `helm template test deploy/helm/literature-review-service/ -f deploy/helm/literature-review-service/values-staging.yaml`
Run: `helm template test deploy/helm/literature-review-service/ -f deploy/helm/literature-review-service/values-production.yaml`
Expected: All render without errors

**Step 5: Verify tests still pass**

Run: `make test`
Expected: All pass

**Step 6: Final commit**

```bash
git add -A
git commit -m "chore(deploy): final verification — phase 7 deployment complete"
```
