# Phase 5: API Layer — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Integrate grpcauth authentication/authorization into the gRPC server, and build a complete HTTP REST API with chi router including SSE progress streaming backed by Postgres NOTIFY + Temporal query polling.

**Architecture:** The existing gRPC server gets grpcauth interceptors added (matching ingestion_service pattern). A new HTTP server (chi) runs alongside it, with REST endpoints that mirror every gRPC RPC. Both servers share the same dependencies (repos, workflow client). The SSE endpoint uses dual-source streaming: Postgres LISTEN/NOTIFY for real-time events and Temporal workflow queries for authoritative state.

**Tech Stack:** Go, chi (HTTP router), grpcauth (JWT + OPA + mTLS), pgxpool (Postgres LISTEN/NOTIFY), Temporal SDK (workflow queries), SSE (Server-Sent Events)

---

## Pre-requisites

- The `grpcauth/chiauth` package exists and provides `chiauth.NewMiddleware(cfg)` returning a `func(http.Handler) http.Handler` middleware. It follows the same pattern as `grpcauth/ginauth` but for standard `net/http`. The user is adding this now.
- All Phase 1-4 code is complete and tests pass.

---

## Task 1: Wire grpcauth Interceptor into gRPC Server

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `go.mod` (add `grpcauth/env` and `grpcauth/interceptor` imports)

**Step 1: Add grpcauth imports and load auth config**

In `cmd/server/main.go`, add these imports:

```go
"github.com/helixir/grpcauth/env"
"github.com/helixir/grpcauth/interceptor"
```

After the Temporal client creation block and before the gRPC server creation, add:

```go
// Configure authentication using shared grpcauth package.
authConfig, err := env.FromEnvironment()
if err != nil {
    return fmt.Errorf("load auth config: %w", err)
}
authConfig.SkipMethods = []string{
    "/grpc.health.v1.Health/Check",
    "/grpc.health.v1.Health/Watch",
    "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
}

serverInterceptor, err := interceptor.NewServerInterceptor(authConfig)
if err != nil {
    return fmt.Errorf("create auth interceptor: %w", err)
}
defer serverInterceptor.Close()
logger.Info().Msg("grpcauth interceptor initialized")
```

**Step 2: Add interceptors to gRPC server options**

Replace the existing `grpc.NewServer(...)` call to include the interceptors in the options:

```go
grpcServer := grpc.NewServer(
    grpc.MaxRecvMsgSize(16*1024*1024),
    grpc.MaxSendMsgSize(16*1024*1024),
    grpc.MaxConcurrentStreams(100),
    grpc.ChainUnaryInterceptor(
        serverInterceptor.UnaryServerInterceptor(),
    ),
    grpc.ChainStreamInterceptor(
        serverInterceptor.StreamServerInterceptor(),
    ),
    grpc.KeepaliveParams(keepalive.ServerParameters{
        MaxConnectionIdle:     15 * time.Minute,
        MaxConnectionAge:      30 * time.Minute,
        MaxConnectionAgeGrace: 5 * time.Minute,
        Time:                  5 * time.Minute,
        Timeout:               1 * time.Minute,
    }),
    grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
        MinTime:             5 * time.Minute,
        PermitWithoutStream: true,
    }),
)
```

**Step 3: Run tests to verify nothing breaks**

```bash
cd literature_service && go build ./...
cd literature_service && go test -short ./internal/server/... -v -count=1
```

Expected: All existing tests pass. Tests use `context.Background()` (no auth context), so they bypass auth at the handler level.

**Step 4: Commit**

```bash
cd literature_service
git add cmd/server/main.go go.mod go.sum
git commit -m "feat(server): wire grpcauth interceptor into gRPC server"
```

---

## Task 2: Add Tenant Access Validation to gRPC Handlers

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/review_handlers.go`
- Modify: `internal/server/paper_handlers.go`
- Modify: `internal/server/stream_handler.go`

**Step 1: Add tenant validation function in `server.go`**

Add the import for `grpcauth`:

```go
grpcauth "github.com/helixir/grpcauth"
```

Add a new function after `validateOrgProject`:

```go
// validateTenantAccess checks that the authenticated user has access to the
// requested org_id and project_id. In permissive mode (no auth context), this
// is a no-op. In strict mode, org/project membership is verified against JWT claims.
func validateTenantAccess(ctx context.Context, orgID, projectID string) error {
	authCtx, ok := grpcauth.AuthFromContext(ctx)
	if !ok || authCtx == nil || authCtx.User == nil {
		// No auth context — permissive mode or service-to-service call.
		return nil
	}

	user := authCtx.User

	if !user.HasOrgAccess(orgID) {
		return status.Errorf(codes.PermissionDenied, "access denied to organization %s", orgID)
	}

	if projectID != "" {
		if !user.IsOrgAdmin() && len(user.GetProjectRoles(projectID)) == 0 {
			return status.Errorf(codes.PermissionDenied, "access denied to project %s", projectID)
		}
	}

	return nil
}
```

**Step 2: Add `validateTenantAccess` calls to each handler**

In each handler in `review_handlers.go`, `paper_handlers.go`, and `stream_handler.go`, add the tenant access check immediately after `validateOrgProject`:

```go
if err := validateOrgProject(req.OrgId, req.ProjectId); err != nil {
    return nil, err
}
if err := validateTenantAccess(ctx, req.OrgId, req.ProjectId); err != nil {
    return nil, err
}
```

For `StreamLiteratureReviewProgress` in `stream_handler.go`, use `stream.Context()`:

```go
if err := validateTenantAccess(stream.Context(), req.OrgId, req.ProjectId); err != nil {
    return err
}
```

Handlers to update (6 total):
1. `StartLiteratureReview` in `review_handlers.go`
2. `GetLiteratureReviewStatus` in `review_handlers.go`
3. `CancelLiteratureReview` in `review_handlers.go`
4. `ListLiteratureReviews` in `review_handlers.go`
5. `GetLiteratureReviewPapers` in `paper_handlers.go`
6. `GetLiteratureReviewKeywords` in `paper_handlers.go`
7. `StreamLiteratureReviewProgress` in `stream_handler.go`

**Step 3: Run tests**

```bash
cd literature_service && go test -short ./internal/server/... -v -count=1
```

Expected: All existing tests still pass (they use `context.Background()` with no auth context, so `validateTenantAccess` is a no-op).

**Step 4: Add test for tenant access validation**

In `review_handlers_test.go`, add a test that verifies `validateTenantAccess` rejects unauthorized users when auth context is present:

```go
func TestStartLiteratureReview_TenantAccessDenied(t *testing.T) {
	repo := &mockReviewRepo{
		createFn: func(_ context.Context, _ *domain.LiteratureReviewRequest) error {
			return nil
		},
	}
	wfClient := &mockWorkflowClient{}
	srv := newTestServerWithWorkflow(wfClient, repo, &mockPaperRepo{}, &mockKeywordRepo{})

	// Create context with auth that does NOT have access to "org-1".
	authCtx := &grpcauth.AuthContext{
		User: &grpcauth.UserIdentity{
			Subject: "user-123",
			OrgIDs:  []string{"other-org"},
		},
	}
	ctx := grpcauth.ContextWithAuth(context.Background(), authCtx)

	_, err := srv.StartLiteratureReview(ctx, &pb.StartLiteratureReviewRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		Query:     "test query",
	})
	assertGRPCCode(t, err, codes.PermissionDenied)
}

func TestGetLiteratureReviewStatus_TenantAccessAllowed(t *testing.T) {
	reviewID := uuid.New()
	now := time.Now()

	repo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return &domain.LiteratureReviewRequest{
				ID:            reviewID,
				OrgID:         orgID,
				ProjectID:     projectID,
				OriginalQuery: "CRISPR",
				Status:        domain.ReviewStatusCompleted,
				Configuration: domain.DefaultReviewConfiguration(),
				CreatedAt:     now,
				UpdatedAt:     now,
			}, nil
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})

	// Create context with auth that HAS access to "org-1".
	authCtx := &grpcauth.AuthContext{
		User: &grpcauth.UserIdentity{
			Subject:      "user-123",
			OrgIDs:       []string{"org-1"},
			ActiveOrgID:  "org-1",
			ProjectRoles: map[string][]string{"proj-1": {"editor"}},
		},
	}
	ctx := grpcauth.ContextWithAuth(context.Background(), authCtx)

	resp, err := srv.GetLiteratureReviewStatus(ctx, &pb.GetLiteratureReviewStatusRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  reviewID.String(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ReviewId != reviewID.String() {
		t.Errorf("expected review_id %s, got %s", reviewID.String(), resp.ReviewId)
	}
}
```

**Step 5: Run tests**

```bash
cd literature_service && go test -short ./internal/server/... -v -count=1
```

Expected: All tests pass including the new tenant access tests.

**Step 6: Commit**

```bash
cd literature_service
git add internal/server/server.go internal/server/review_handlers.go internal/server/paper_handlers.go internal/server/stream_handler.go internal/server/review_handlers_test.go
git commit -m "feat(server): add tenant access validation using grpcauth context"
```

---

## Task 3: Add chi Dependency and Create HTTP Server Skeleton

**Files:**
- Modify: `go.mod` (add chi)
- Create: `internal/server/http/server.go`

**Step 1: Add chi dependency**

```bash
cd literature_service && go get github.com/go-chi/chi/v5
```

**Step 2: Create the HTTP server**

Create `internal/server/http/server.go`:

```go
// Package httpserver provides the HTTP REST API server for the literature review service.
package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	grpcauth "github.com/helixir/grpcauth"
	"github.com/helixir/literature-review-service/internal/database"
	"github.com/helixir/literature-review-service/internal/repository"
	servergrpc "github.com/helixir/literature-review-service/internal/server"
)

// WorkflowClient defines the interface for workflow operations used by the HTTP server.
// This is the same interface as the gRPC server uses.
type WorkflowClient = servergrpc.WorkflowClient

// Server is the HTTP REST API server.
type Server struct {
	router         chi.Router
	httpServer     *http.Server
	workflowClient WorkflowClient
	reviewRepo     repository.ReviewRepository
	paperRepo      repository.PaperRepository
	keywordRepo    repository.KeywordRepository
	db             *database.DB
	pool           *pgxpool.Pool
	logger         zerolog.Logger
	authMiddleware func(http.Handler) http.Handler
}

// Config holds HTTP server configuration.
type Config struct {
	Address         string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// NewServer creates a new HTTP server with all dependencies.
func NewServer(
	cfg Config,
	workflowClient WorkflowClient,
	reviewRepo repository.ReviewRepository,
	paperRepo repository.PaperRepository,
	keywordRepo repository.KeywordRepository,
	db *database.DB,
	logger zerolog.Logger,
	authMiddleware func(http.Handler) http.Handler,
) *Server {
	s := &Server{
		workflowClient: workflowClient,
		reviewRepo:     reviewRepo,
		paperRepo:      paperRepo,
		keywordRepo:    keywordRepo,
		db:             db,
		pool:           db.Pool(),
		logger:         logger.With().Str("component", "http-server").Logger(),
		authMiddleware: authMiddleware,
	}

	s.router = s.buildRouter()

	s.httpServer = &http.Server{
		Addr:         cfg.Address,
		Handler:      s.router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return s
}

// buildRouter creates the chi router with all middleware and routes.
func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(correlationIDMiddleware)
	r.Use(jsonContentTypeMiddleware)

	// Health endpoints (no auth)
	r.Get("/healthz", s.healthHandler)
	r.Get("/readyz", s.readinessHandler)

	// API routes with auth + tenant context
	r.Route("/api/v1/orgs/{orgID}/projects/{projectID}", func(r chi.Router) {
		if s.authMiddleware != nil {
			r.Use(s.authMiddleware)
		}
		r.Use(tenantContextMiddleware)

		r.Post("/literature-reviews", s.startLiteratureReview)
		r.Get("/literature-reviews", s.listLiteratureReviews)
		r.Get("/literature-reviews/{reviewID}", s.getLiteratureReviewStatus)
		r.Delete("/literature-reviews/{reviewID}", s.cancelLiteratureReview)
		r.Get("/literature-reviews/{reviewID}/papers", s.getLiteratureReviewPapers)
		r.Get("/literature-reviews/{reviewID}/keywords", s.getLiteratureReviewKeywords)
		r.Get("/literature-reviews/{reviewID}/progress", s.streamProgress)
	})

	return r
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	s.logger.Info().Str("address", s.httpServer.Addr).Msg("HTTP server starting")
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen on HTTP address: %w", err)
	}
	return s.httpServer.Serve(ln)
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// healthHandler returns basic liveness status.
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	health := s.db.Health(r.Context())
	if health.Status == "healthy" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "database": health.Status})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{
		"status":   "unhealthy",
		"database": health.Status,
		"error":    health.Error,
	})
}

// readinessHandler returns readiness status including Temporal connectivity.
func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
	health := s.db.Health(r.Context())
	if health.Status != "healthy" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status":   "not_ready",
			"database": health.Status,
			"error":    health.Error,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "ready",
		"database": "healthy",
	})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Best-effort log; headers already sent.
		_ = err
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{
		"error": message,
	})
}
```

**Step 3: Verify it compiles**

```bash
cd literature_service && go build ./internal/server/http/...
```

Expected: Compilation fails because handler methods and middleware don't exist yet. That's fine — we'll add them in the next tasks. If needed, add stub methods to unblock compilation:

Create stub file `internal/server/http/handlers_stub.go` (temporary, will be replaced):

```go
package httpserver

import "net/http"

func (s *Server) startLiteratureReview(w http.ResponseWriter, r *http.Request)      { writeError(w, 501, "not implemented") }
func (s *Server) getLiteratureReviewStatus(w http.ResponseWriter, r *http.Request)   { writeError(w, 501, "not implemented") }
func (s *Server) cancelLiteratureReview(w http.ResponseWriter, r *http.Request)      { writeError(w, 501, "not implemented") }
func (s *Server) listLiteratureReviews(w http.ResponseWriter, r *http.Request)       { writeError(w, 501, "not implemented") }
func (s *Server) getLiteratureReviewPapers(w http.ResponseWriter, r *http.Request)   { writeError(w, 501, "not implemented") }
func (s *Server) getLiteratureReviewKeywords(w http.ResponseWriter, r *http.Request)  { writeError(w, 501, "not implemented") }
func (s *Server) streamProgress(w http.ResponseWriter, r *http.Request)              { writeError(w, 501, "not implemented") }
```

**Step 4: Verify compilation**

```bash
cd literature_service && go build ./internal/server/http/...
```

Expected: Compiles successfully.

**Step 5: Commit**

```bash
cd literature_service
git add internal/server/http/ go.mod go.sum
git commit -m "feat(http): add HTTP server skeleton with chi router and health endpoints"
```

---

## Task 4: Create HTTP Middleware

**Files:**
- Create: `internal/server/http/middleware.go`
- Create: `internal/server/http/middleware_test.go`

**Step 1: Create middleware**

Create `internal/server/http/middleware.go`:

```go
package httpserver

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	grpcauth "github.com/helixir/grpcauth"
)

type contextKey string

const (
	ctxKeyOrgID     contextKey = "org_id"
	ctxKeyProjectID contextKey = "project_id"
)

// tenantContextMiddleware extracts orgID and projectID from the URL path
// and stores them in the request context. It also validates that the
// authenticated user has access to the requested org/project.
func tenantContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID := chi.URLParam(r, "orgID")
		projectID := chi.URLParam(r, "projectID")

		if orgID == "" || projectID == "" {
			writeError(w, http.StatusBadRequest, "org_id and project_id are required")
			return
		}

		// Validate tenant access if auth context is present.
		authCtx, ok := grpcauth.AuthFromContext(r.Context())
		if ok && authCtx != nil && authCtx.User != nil {
			user := authCtx.User
			if !user.HasOrgAccess(orgID) {
				writeError(w, http.StatusForbidden, "access denied to organization")
				return
			}
			if !user.IsOrgAdmin() && len(user.GetProjectRoles(projectID)) == 0 {
				writeError(w, http.StatusForbidden, "access denied to project")
				return
			}
		}

		ctx := context.WithValue(r.Context(), ctxKeyOrgID, orgID)
		ctx = context.WithValue(ctx, ctxKeyProjectID, projectID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// correlationIDMiddleware ensures every request has a correlation ID.
// It checks the X-Correlation-ID header first, then falls back to the
// chi request ID, and finally generates a random one.
func correlationIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = middleware.GetReqID(r.Context())
		}
		if correlationID == "" {
			buf := make([]byte, 8)
			_, _ = rand.Read(buf)
			correlationID = fmt.Sprintf("%x", buf)
		}

		w.Header().Set("X-Correlation-ID", correlationID)
		ctx := grpcauth.ContextWithCorrelationID(r.Context(), correlationID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// jsonContentTypeMiddleware sets Accept: application/json for API routes.
func jsonContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// orgIDFromContext extracts the org_id from the request context.
func orgIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyOrgID).(string); ok {
		return v
	}
	return ""
}

// projectIDFromContext extracts the project_id from the request context.
func projectIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyProjectID).(string); ok {
		return v
	}
	return ""
}
```

**Step 2: Write middleware tests**

Create `internal/server/http/middleware_test.go`:

```go
package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	grpcauth "github.com/helixir/grpcauth"
)

func TestTenantContextMiddleware_SetsOrgAndProject(t *testing.T) {
	var capturedOrgID, capturedProjectID string

	r := chi.NewRouter()
	r.Route("/api/v1/orgs/{orgID}/projects/{projectID}", func(r chi.Router) {
		r.Use(tenantContextMiddleware)
		r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
			capturedOrgID = orgIDFromContext(r.Context())
			capturedProjectID = projectIDFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		})
	})

	req := httptest.NewRequest("GET", "/api/v1/orgs/my-org/projects/my-proj/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if capturedOrgID != "my-org" {
		t.Errorf("expected org_id my-org, got %s", capturedOrgID)
	}
	if capturedProjectID != "my-proj" {
		t.Errorf("expected project_id my-proj, got %s", capturedProjectID)
	}
}

func TestTenantContextMiddleware_DeniesUnauthorizedOrg(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/api/v1/orgs/{orgID}/projects/{projectID}", func(r chi.Router) {
		// Inject auth context middleware (simulating chiauth)
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				authCtx := &grpcauth.AuthContext{
					User: &grpcauth.UserIdentity{
						Subject: "user-1",
						OrgIDs:  []string{"other-org"},
					},
				}
				ctx := grpcauth.ContextWithAuth(req.Context(), authCtx)
				next.ServeHTTP(w, req.WithContext(ctx))
			})
		})
		r.Use(tenantContextMiddleware)
		r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	req := httptest.NewRequest("GET", "/api/v1/orgs/my-org/projects/my-proj/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCorrelationIDMiddleware_UsesExistingHeader(t *testing.T) {
	handler := correlationIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := grpcauth.CorrelationIDFromContext(r.Context())
		if cid != "test-correlation-123" {
			t.Errorf("expected correlation ID test-correlation-123, got %s", cid)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Correlation-ID", "test-correlation-123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-Correlation-ID") != "test-correlation-123" {
		t.Errorf("expected X-Correlation-ID header to be set")
	}
}

func TestCorrelationIDMiddleware_GeneratesIfMissing(t *testing.T) {
	handler := correlationIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := grpcauth.CorrelationIDFromContext(r.Context())
		if cid == "" {
			t.Error("expected non-empty correlation ID")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-Correlation-ID") == "" {
		t.Error("expected X-Correlation-ID header to be set")
	}
}

func TestOrgIDFromContext_ReturnsEmptyWhenMissing(t *testing.T) {
	if orgIDFromContext(context.Background()) != "" {
		t.Error("expected empty string for missing org_id")
	}
}
```

**Step 3: Run tests**

```bash
cd literature_service && go test -short ./internal/server/http/... -v -count=1
```

Expected: All middleware tests pass.

**Step 4: Commit**

```bash
cd literature_service
git add internal/server/http/middleware.go internal/server/http/middleware_test.go
git commit -m "feat(http): add tenant context and correlation ID middleware"
```

---

## Task 5: Create HTTP Response Types and Converters

**Files:**
- Create: `internal/server/http/response.go`

**Step 1: Create response structs and converter functions**

Create `internal/server/http/response.go`:

```go
package httpserver

import (
	"time"

	"github.com/helixir/literature-review-service/internal/domain"
)

// Review response types for JSON serialization.

type startReviewResponse struct {
	ReviewID   string    `json:"review_id"`
	WorkflowID string   `json:"workflow_id"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	Message    string    `json:"message"`
}

type reviewStatusResponse struct {
	ReviewID     string              `json:"review_id"`
	Status       string              `json:"status"`
	Progress     *progressResponse   `json:"progress,omitempty"`
	ErrorMessage string              `json:"error_message,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
	StartedAt    *time.Time          `json:"started_at,omitempty"`
	CompletedAt  *time.Time          `json:"completed_at,omitempty"`
	Duration     string              `json:"duration,omitempty"`
	Config       *configResponse     `json:"configuration,omitempty"`
}

type progressResponse struct {
	InitialKeywordsCount   int `json:"initial_keywords_count"`
	TotalKeywordsProcessed int `json:"total_keywords_processed"`
	PapersFound            int `json:"papers_found"`
	PapersNew              int `json:"papers_new"`
	PapersIngested         int `json:"papers_ingested"`
	PapersFailed           int `json:"papers_failed"`
	MaxExpansionDepth      int `json:"max_expansion_depth"`
}

type configResponse struct {
	InitialKeywordCount int      `json:"initial_keyword_count"`
	PaperKeywordCount   int      `json:"paper_keyword_count"`
	MaxExpansionDepth   int      `json:"max_expansion_depth"`
	EnabledSources      []string `json:"enabled_sources"`
	DateFrom            string   `json:"date_from,omitempty"`
	DateTo              string   `json:"date_to,omitempty"`
}

type reviewSummaryResponse struct {
	ReviewID       string     `json:"review_id"`
	OriginalQuery  string     `json:"original_query"`
	Status         string     `json:"status"`
	PapersFound    int        `json:"papers_found"`
	PapersIngested int        `json:"papers_ingested"`
	KeywordsUsed   int        `json:"keywords_used"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	Duration       string     `json:"duration,omitempty"`
}

type listReviewsResponse struct {
	Reviews       []reviewSummaryResponse `json:"reviews"`
	NextPageToken string                  `json:"next_page_token,omitempty"`
	TotalCount    int                     `json:"total_count"`
}

type cancelReviewResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	FinalStatus string `json:"final_status"`
}

type paperResponse struct {
	ID              string           `json:"id"`
	DOI             string           `json:"doi,omitempty"`
	ArxivID         string           `json:"arxiv_id,omitempty"`
	PubmedID        string           `json:"pubmed_id,omitempty"`
	Title           string           `json:"title"`
	Abstract        string           `json:"abstract,omitempty"`
	Authors         []authorResponse `json:"authors,omitempty"`
	PublicationDate *time.Time       `json:"publication_date,omitempty"`
	PublicationYear int              `json:"publication_year,omitempty"`
	Venue           string           `json:"venue,omitempty"`
	Journal         string           `json:"journal,omitempty"`
	CitationCount   int              `json:"citation_count"`
	PdfURL          string           `json:"pdf_url,omitempty"`
	OpenAccess      bool             `json:"open_access"`
}

type authorResponse struct {
	Name        string `json:"name"`
	Affiliation string `json:"affiliation,omitempty"`
	ORCID       string `json:"orcid,omitempty"`
}

type listPapersResponse struct {
	Papers        []paperResponse `json:"papers"`
	NextPageToken string          `json:"next_page_token,omitempty"`
	TotalCount    int             `json:"total_count"`
}

type keywordResponse struct {
	ID                string  `json:"id"`
	Keyword           string  `json:"keyword"`
	NormalizedKeyword string  `json:"normalized_keyword"`
}

type listKeywordsResponse struct {
	Keywords      []keywordResponse `json:"keywords"`
	NextPageToken string            `json:"next_page_token,omitempty"`
	TotalCount    int               `json:"total_count"`
}

// Converter functions

func domainReviewToStatusResponse(r *domain.LiteratureReviewRequest) reviewStatusResponse {
	resp := reviewStatusResponse{
		ReviewID:     r.ID.String(),
		Status:       string(r.Status),
		ErrorMessage: r.ErrorMessage,
		CreatedAt:    r.CreatedAt,
		StartedAt:    r.StartedAt,
		CompletedAt:  r.CompletedAt,
		Progress: &progressResponse{
			InitialKeywordsCount:   r.Configuration.MaxKeywordsPerRound,
			TotalKeywordsProcessed: r.KeywordsFoundCount,
			PapersFound:            r.PapersFoundCount,
			PapersNew:              r.PapersFoundCount,
			PapersIngested:         r.PapersIngestedCount,
			PapersFailed:           r.PapersFailedCount,
			MaxExpansionDepth:      r.Configuration.MaxExpansionDepth,
		},
		Config: domainConfigToResponse(r.Configuration),
	}
	if d := r.Duration(); d > 0 {
		resp.Duration = d.String()
	}
	return resp
}

func domainConfigToResponse(c domain.ReviewConfiguration) *configResponse {
	sources := make([]string, len(c.Sources))
	for i, s := range c.Sources {
		sources[i] = string(s)
	}
	resp := &configResponse{
		InitialKeywordCount: c.MaxKeywordsPerRound,
		PaperKeywordCount:   c.PaperKeywordCount,
		MaxExpansionDepth:   c.MaxExpansionDepth,
		EnabledSources:      sources,
	}
	if c.DateFrom != nil {
		resp.DateFrom = c.DateFrom.Format(time.RFC3339)
	}
	if c.DateTo != nil {
		resp.DateTo = c.DateTo.Format(time.RFC3339)
	}
	return resp
}

func domainReviewToSummary(r *domain.LiteratureReviewRequest) reviewSummaryResponse {
	resp := reviewSummaryResponse{
		ReviewID:       r.ID.String(),
		OriginalQuery:  r.OriginalQuery,
		Status:         string(r.Status),
		PapersFound:    r.PapersFoundCount,
		PapersIngested: r.PapersIngestedCount,
		KeywordsUsed:   r.KeywordsFoundCount,
		CreatedAt:      r.CreatedAt,
		CompletedAt:    r.CompletedAt,
	}
	if d := r.Duration(); d > 0 {
		resp.Duration = d.String()
	}
	return resp
}

func domainPaperToResponse(p *domain.Paper) paperResponse {
	authors := make([]authorResponse, len(p.Authors))
	for i, a := range p.Authors {
		authors[i] = authorResponse{
			Name:        a.Name,
			Affiliation: a.Affiliation,
			ORCID:       a.ORCID,
		}
	}
	return paperResponse{
		ID:              p.ID.String(),
		DOI:             p.DOI,
		ArxivID:         p.ArxivID,
		Title:           p.Title,
		Abstract:        p.Abstract,
		Authors:         authors,
		PublicationDate: p.PublicationDate,
		PublicationYear: p.PublicationYear,
		Venue:           p.Venue,
		Journal:         p.Journal,
		CitationCount:   p.CitationCount,
		PdfURL:          p.PDFURL,
		OpenAccess:      p.OpenAccess,
	}
}

func domainKeywordToResponse(k *domain.Keyword) keywordResponse {
	return keywordResponse{
		ID:                k.ID.String(),
		Keyword:           k.Keyword,
		NormalizedKeyword: k.NormalizedKeyword,
	}
}
```

**Step 2: Verify compilation**

```bash
cd literature_service && go build ./internal/server/http/...
```

Expected: Compiles successfully.

**Step 3: Commit**

```bash
cd literature_service
git add internal/server/http/response.go
git commit -m "feat(http): add JSON response types and domain converters"
```

---

## Task 6: Implement HTTP Review Handlers

**Files:**
- Create: `internal/server/http/handlers.go` (replaces `handlers_stub.go`)
- Delete: `internal/server/http/handlers_stub.go`
- Create: `internal/server/http/handlers_test.go`

**Step 1: Create the review handlers**

Create `internal/server/http/handlers.go`:

```go
package httpserver

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"
	"github.com/helixir/literature-review-service/internal/temporal"
)

const (
	defaultPageSize = 50
	maxPageSize     = 100
	maxQueryLength  = 10000
)

// startLiteratureReview handles POST /literature-reviews
func (s *Server) startLiteratureReview(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromContext(r.Context())
	projectID := projectIDFromContext(r.Context())

	var body struct {
		Query              string   `json:"query"`
		InitialKeywordCount *int    `json:"initial_keyword_count,omitempty"`
		PaperKeywordCount  *int     `json:"paper_keyword_count,omitempty"`
		MaxExpansionDepth  *int     `json:"max_expansion_depth,omitempty"`
		SourceFilters      []string `json:"source_filters,omitempty"`
		DateFrom           *string  `json:"date_from,omitempty"`
		DateTo             *string  `json:"date_to,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	if len(body.Query) > maxQueryLength {
		writeError(w, http.StatusBadRequest, "query must be at most 10000 characters")
		return
	}

	cfg := domain.DefaultReviewConfiguration()
	if body.InitialKeywordCount != nil {
		cfg.MaxKeywordsPerRound = *body.InitialKeywordCount
	}
	if body.PaperKeywordCount != nil {
		cfg.PaperKeywordCount = *body.PaperKeywordCount
	}
	if body.MaxExpansionDepth != nil {
		cfg.MaxExpansionDepth = *body.MaxExpansionDepth
	}
	if len(body.SourceFilters) > 0 {
		sources := make([]domain.SourceType, len(body.SourceFilters))
		for i, sf := range body.SourceFilters {
			sources[i] = domain.SourceType(sf)
		}
		cfg.Sources = sources
	}
	if body.DateFrom != nil {
		t, err := time.Parse(time.RFC3339, *body.DateFrom)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid date_from format, use RFC3339")
			return
		}
		cfg.DateFrom = &t
	}
	if body.DateTo != nil {
		t, err := time.Parse(time.RFC3339, *body.DateTo)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid date_to format, use RFC3339")
			return
		}
		cfg.DateTo = &t
	}

	requestID := uuid.New()
	now := time.Now()
	review := &domain.LiteratureReviewRequest{
		ID:            requestID,
		OrgID:         orgID,
		ProjectID:     projectID,
		OriginalQuery: body.Query,
		Status:        domain.ReviewStatusPending,
		Configuration: cfg,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.reviewRepo.Create(r.Context(), review); err != nil {
		s.writeDomainError(w, err)
		return
	}

	wfInput := temporal.ReviewWorkflowInput{
		RequestID: requestID,
		OrgID:     orgID,
		ProjectID: projectID,
		Query:     body.Query,
		Config:    cfg,
	}

	workflowID, _, err := s.workflowClient.StartReviewWorkflow(r.Context(), temporal.ReviewWorkflowRequest{
		RequestID: requestID.String(),
		OrgID:     orgID,
		ProjectID: projectID,
		Query:     body.Query,
	}, nil, wfInput)
	if err != nil {
		s.writeDomainError(w, err)
		return
	}

	// Best-effort update of workflow IDs.
	_ = s.reviewRepo.Update(r.Context(), orgID, projectID, requestID, func(r *domain.LiteratureReviewRequest) error {
		r.TemporalWorkflowID = workflowID
		return nil
	})

	writeJSON(w, http.StatusCreated, startReviewResponse{
		ReviewID:   requestID.String(),
		WorkflowID: workflowID,
		Status:     string(domain.ReviewStatusPending),
		CreatedAt:  now,
		Message:    "literature review started",
	})
}

// getLiteratureReviewStatus handles GET /literature-reviews/{reviewID}
func (s *Server) getLiteratureReviewStatus(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromContext(r.Context())
	projectID := projectIDFromContext(r.Context())

	reviewID, err := parseUUID(chi.URLParam(r, "reviewID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid review_id")
		return
	}

	review, err := s.reviewRepo.Get(r.Context(), orgID, projectID, reviewID)
	if err != nil {
		s.writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, domainReviewToStatusResponse(review))
}

// cancelLiteratureReview handles DELETE /literature-reviews/{reviewID}
func (s *Server) cancelLiteratureReview(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromContext(r.Context())
	projectID := projectIDFromContext(r.Context())

	reviewID, err := parseUUID(chi.URLParam(r, "reviewID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid review_id")
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	// Body is optional for cancel.
	_ = json.NewDecoder(r.Body).Decode(&body)

	review, err := s.reviewRepo.Get(r.Context(), orgID, projectID, reviewID)
	if err != nil {
		s.writeDomainError(w, err)
		return
	}

	if review.Status.IsTerminal() {
		writeError(w, http.StatusConflict, "review is already in terminal state")
		return
	}

	if err := s.workflowClient.SignalWorkflow(r.Context(), review.TemporalWorkflowID, review.TemporalRunID, temporal.SignalCancel, body.Reason); err != nil {
		s.writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, cancelReviewResponse{
		Success:     true,
		Message:     "cancellation requested",
		FinalStatus: string(review.Status),
	})
}

// listLiteratureReviews handles GET /literature-reviews
func (s *Server) listLiteratureReviews(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromContext(r.Context())
	projectID := projectIDFromContext(r.Context())

	limit, offset := parsePaginationParams(r)

	filter := repository.ReviewFilter{
		OrgID:     orgID,
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	}

	if statusParam := r.URL.Query().Get("status"); statusParam != "" {
		filter.Status = []domain.ReviewStatus{domain.ReviewStatus(statusParam)}
	}

	if after := r.URL.Query().Get("created_after"); after != "" {
		t, err := time.Parse(time.RFC3339, after)
		if err == nil {
			filter.CreatedAfter = &t
		}
	}
	if before := r.URL.Query().Get("created_before"); before != "" {
		t, err := time.Parse(time.RFC3339, before)
		if err == nil {
			filter.CreatedBefore = &t
		}
	}

	reviews, totalCount, err := s.reviewRepo.List(r.Context(), filter)
	if err != nil {
		s.writeDomainError(w, err)
		return
	}

	summaries := make([]reviewSummaryResponse, len(reviews))
	for i, r := range reviews {
		summaries[i] = domainReviewToSummary(r)
	}

	writeJSON(w, http.StatusOK, listReviewsResponse{
		Reviews:       summaries,
		NextPageToken: encodeHTTPPageToken(offset, limit, totalCount),
		TotalCount:    int(totalCount),
	})
}

// writeDomainError maps domain errors to HTTP status codes.
func (s *Server) writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		writeError(w, http.StatusConflict, "already exists")
	case errors.Is(err, domain.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, domain.ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, "rate limited")
	case errors.Is(err, domain.ErrServiceUnavailable):
		writeError(w, http.StatusServiceUnavailable, "service unavailable")
	case errors.Is(err, temporal.ErrWorkflowNotFound):
		writeError(w, http.StatusNotFound, "workflow not found")
	case errors.Is(err, temporal.ErrWorkflowAlreadyStarted):
		writeError(w, http.StatusConflict, "workflow already started")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// Helper functions

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func parsePaginationParams(r *http.Request) (limit, offset int) {
	limit = defaultPageSize
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if n, err := strconv.Atoi(ps); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}

	if pt := r.URL.Query().Get("page_token"); pt != "" {
		decoded, err := base64.StdEncoding.DecodeString(pt)
		if err == nil {
			if n, parseErr := strconv.Atoi(string(decoded)); parseErr == nil && n > 0 {
				offset = n
			}
		}
	}

	return limit, offset
}

func encodeHTTPPageToken(offset, limit int, totalCount int64) string {
	if offset+limit < int(totalCount) {
		return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset + limit)))
	}
	return ""
}
```

**Step 2: Delete the stub file**

```bash
rm literature_service/internal/server/http/handlers_stub.go
```

**Step 3: Verify compilation**

```bash
cd literature_service && go build ./internal/server/http/...
```

Expected: Fails because paper/keyword handlers and streamProgress aren't defined yet. Create a minimal stub for just those:

Create `internal/server/http/paper_handlers.go` (stub for now):

```go
package httpserver

import "net/http"

func (s *Server) getLiteratureReviewPapers(w http.ResponseWriter, r *http.Request)  { writeError(w, 501, "not implemented") }
func (s *Server) getLiteratureReviewKeywords(w http.ResponseWriter, r *http.Request) { writeError(w, 501, "not implemented") }
```

Create `internal/server/http/progress_stream.go` (stub for now):

```go
package httpserver

import "net/http"

func (s *Server) streamProgress(w http.ResponseWriter, r *http.Request) { writeError(w, 501, "not implemented") }
```

**Step 4: Write handler tests**

Create `internal/server/http/handlers_test.go` with tests for StartReview, GetStatus, Cancel, List. Use `httptest.NewRecorder` and the chi test router with mock repos (same mock pattern as gRPC tests).

The test file should define mock repos, create a test server, and exercise each endpoint. Use the existing mock patterns from `internal/server/review_handlers_test.go` as reference.

**Step 5: Run tests**

```bash
cd literature_service && go test -short ./internal/server/http/... -v -count=1
```

Expected: All handler tests pass.

**Step 6: Commit**

```bash
cd literature_service
git add internal/server/http/handlers.go internal/server/http/handlers_test.go internal/server/http/paper_handlers.go internal/server/http/progress_stream.go
git rm -f internal/server/http/handlers_stub.go 2>/dev/null || true
git commit -m "feat(http): implement review HTTP handlers (start, status, cancel, list)"
```

---

## Task 7: Implement HTTP Paper & Keyword Handlers

**Files:**
- Replace: `internal/server/http/paper_handlers.go`
- Create: `internal/server/http/paper_handlers_test.go`

**Step 1: Implement paper and keyword handlers**

Replace `internal/server/http/paper_handlers.go` with full implementations:

```go
package httpserver

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"
)

// getLiteratureReviewPapers handles GET /literature-reviews/{reviewID}/papers
func (s *Server) getLiteratureReviewPapers(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromContext(r.Context())
	projectID := projectIDFromContext(r.Context())

	reviewID, err := parseUUID(chi.URLParam(r, "reviewID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid review_id")
		return
	}

	// Verify review exists within tenant scope.
	if _, err := s.reviewRepo.Get(r.Context(), orgID, projectID, reviewID); err != nil {
		s.writeDomainError(w, err)
		return
	}

	limit, offset := parsePaginationParams(r)

	filter := repository.PaperFilter{
		ReviewID: &reviewID,
		Limit:    limit,
		Offset:   offset,
	}

	if sourceParam := r.URL.Query().Get("source"); sourceParam != "" {
		source := domain.SourceType(sourceParam)
		filter.Source = &source
	}

	if statusParam := r.URL.Query().Get("ingestion_status"); statusParam != "" {
		ingStatus := domain.IngestionStatus(statusParam)
		filter.IngestionStatus = &ingStatus
	}

	papers, totalCount, err := s.paperRepo.List(r.Context(), filter)
	if err != nil {
		s.writeDomainError(w, err)
		return
	}

	responses := make([]paperResponse, len(papers))
	for i, p := range papers {
		responses[i] = domainPaperToResponse(p)
	}

	writeJSON(w, http.StatusOK, listPapersResponse{
		Papers:        responses,
		NextPageToken: encodeHTTPPageToken(offset, limit, totalCount),
		TotalCount:    int(totalCount),
	})
}

// getLiteratureReviewKeywords handles GET /literature-reviews/{reviewID}/keywords
func (s *Server) getLiteratureReviewKeywords(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromContext(r.Context())
	projectID := projectIDFromContext(r.Context())

	reviewID, err := parseUUID(chi.URLParam(r, "reviewID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid review_id")
		return
	}

	// Verify review exists within tenant scope.
	if _, err := s.reviewRepo.Get(r.Context(), orgID, projectID, reviewID); err != nil {
		s.writeDomainError(w, err)
		return
	}

	limit, offset := parsePaginationParams(r)

	filter := repository.KeywordFilter{
		ReviewID: &reviewID,
		Limit:    limit,
		Offset:   offset,
	}

	if roundParam := r.URL.Query().Get("extraction_round"); roundParam != "" {
		if round, err := strconv.Atoi(roundParam); err == nil {
			filter.ExtractionRound = &round
		}
	}

	if stParam := r.URL.Query().Get("source_type"); stParam != "" {
		filter.SourceType = &stParam
	}

	keywords, totalCount, err := s.keywordRepo.List(r.Context(), filter)
	if err != nil {
		s.writeDomainError(w, err)
		return
	}

	responses := make([]keywordResponse, len(keywords))
	for i, k := range keywords {
		responses[i] = domainKeywordToResponse(k)
	}

	writeJSON(w, http.StatusOK, listKeywordsResponse{
		Keywords:      responses,
		NextPageToken: encodeHTTPPageToken(offset, limit, totalCount),
		TotalCount:    int(totalCount),
	})
}
```

**Step 2: Write tests**

Create `internal/server/http/paper_handlers_test.go` with tests for GetPapers and GetKeywords endpoints.

**Step 3: Run tests**

```bash
cd literature_service && go test -short ./internal/server/http/... -v -count=1
```

**Step 4: Commit**

```bash
cd literature_service
git add internal/server/http/paper_handlers.go internal/server/http/paper_handlers_test.go
git commit -m "feat(http): implement paper and keyword HTTP handlers"
```

---

## Task 8: Implement SSE Progress Streaming

**Files:**
- Replace: `internal/server/http/progress_stream.go`
- Create: `internal/server/http/progress_stream_test.go`

**Step 1: Implement the SSE streaming handler**

Replace `internal/server/http/progress_stream.go`:

```go
package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/helixir/literature-review-service/internal/domain"
)

const (
	// sseQueryInterval is how often we poll Temporal for authoritative state.
	sseQueryInterval = 2 * time.Second
	// sseMaxDuration is the maximum time an SSE stream may remain open.
	sseMaxDuration = 4 * time.Hour
)

// sseEvent represents an event sent via SSE.
type sseEvent struct {
	EventType string      `json:"event_type"`
	ReviewID  string      `json:"review_id"`
	Status    string      `json:"status"`
	Progress  interface{} `json:"progress,omitempty"`
	Message   string      `json:"message"`
	Timestamp time.Time   `json:"timestamp"`
}

// streamProgress handles GET /literature-reviews/{reviewID}/progress (SSE)
func (s *Server) streamProgress(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromContext(r.Context())
	projectID := projectIDFromContext(r.Context())

	reviewID, err := parseUUID(chi.URLParam(r, "reviewID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid review_id")
		return
	}

	review, err := s.reviewRepo.Get(r.Context(), orgID, projectID, reviewID)
	if err != nil {
		s.writeDomainError(w, err)
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// If already terminal, send one event and close.
	if review.Status.IsTerminal() {
		sendSSEEvent(w, flusher, sseEvent{
			EventType: "completed",
			ReviewID:  reviewID.String(),
			Status:    string(review.Status),
			Progress:  buildProgressData(review),
			Message:   "review is in terminal state",
			Timestamp: time.Now(),
		})
		return
	}

	if review.TemporalWorkflowID == "" {
		sendSSEEvent(w, flusher, sseEvent{
			EventType: "error",
			ReviewID:  reviewID.String(),
			Status:    string(review.Status),
			Message:   "review has no associated workflow",
			Timestamp: time.Now(),
		})
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Channel for Postgres NOTIFY events.
	notifyCh := make(chan sseEvent, 100)

	// Start listening for Postgres NOTIFY.
	go s.listenForNotifications(ctx, reviewID.String(), notifyCh)

	// Send initial state.
	sendSSEEvent(w, flusher, sseEvent{
		EventType: "stream_started",
		ReviewID:  reviewID.String(),
		Status:    string(review.Status),
		Progress:  buildProgressData(review),
		Message:   "progress stream started",
		Timestamp: time.Now(),
	})

	deadlineTimer := time.NewTimer(sseMaxDuration)
	defer deadlineTimer.Stop()
	ticker := time.NewTicker(sseQueryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-deadlineTimer.C:
			sendSSEEvent(w, flusher, sseEvent{
				EventType: "timeout",
				ReviewID:  reviewID.String(),
				Message:   "stream max duration exceeded",
				Timestamp: time.Now(),
			})
			return

		case event := <-notifyCh:
			sendSSEEvent(w, flusher, event)
			if isTerminalEventType(event.EventType) {
				return
			}

		case <-ticker.C:
			// Poll DB for authoritative state.
			current, err := s.reviewRepo.Get(ctx, orgID, projectID, reviewID)
			if err != nil {
				s.logger.Error().Err(err).Str("review_id", reviewID.String()).Msg("failed to poll review status")
				continue
			}

			sendSSEEvent(w, flusher, sseEvent{
				EventType: "progress_update",
				ReviewID:  current.ID.String(),
				Status:    string(current.Status),
				Progress:  buildProgressData(current),
				Message:   "status: " + string(current.Status),
				Timestamp: time.Now(),
			})

			if current.Status.IsTerminal() {
				sendSSEEvent(w, flusher, sseEvent{
					EventType: "completed",
					ReviewID:  current.ID.String(),
					Status:    string(current.Status),
					Progress:  buildProgressData(current),
					Message:   "review completed with status: " + string(current.Status),
					Timestamp: time.Now(),
				})
				return
			}
		}
	}
}

// listenForNotifications listens for Postgres NOTIFY events on the review progress channel.
func (s *Server) listenForNotifications(ctx context.Context, reviewID string, out chan<- sseEvent) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to acquire connection for LISTEN")
		return
	}
	defer conn.Release()

	channel := fmt.Sprintf("review_progress_%s", reviewID)

	// Use the underlying pgx connection for LISTEN (not the pooled wrapper).
	pgConn := conn.Conn()
	if _, err := pgConn.Exec(ctx, fmt.Sprintf("LISTEN %s", channel)); err != nil {
		s.logger.Error().Err(err).Str("channel", channel).Msg("LISTEN failed")
		return
	}
	defer func() {
		_, _ = pgConn.Exec(context.Background(), fmt.Sprintf("UNLISTEN %s", channel))
	}()

	for {
		notification, err := pgConn.WaitForNotification(ctx)
		if err != nil {
			// Context cancelled or connection error.
			return
		}

		var event sseEvent
		if err := json.Unmarshal([]byte(notification.Payload), &event); err != nil {
			s.logger.Warn().Err(err).Msg("failed to parse notification payload")
			continue
		}
		event.ReviewID = reviewID
		event.Timestamp = time.Now()

		select {
		case out <- event:
		case <-ctx.Done():
			return
		default:
			// Channel full, drop event.
		}
	}
}

// sendSSEEvent writes a single SSE event to the response writer.
func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, event sseEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.EventType, data)
	flusher.Flush()
}

func buildProgressData(r *domain.LiteratureReviewRequest) *progressResponse {
	return &progressResponse{
		InitialKeywordsCount:   r.Configuration.MaxKeywordsPerRound,
		TotalKeywordsProcessed: r.KeywordsFoundCount,
		PapersFound:            r.PapersFoundCount,
		PapersNew:              r.PapersFoundCount,
		PapersIngested:         r.PapersIngestedCount,
		PapersFailed:           r.PapersFailedCount,
		MaxExpansionDepth:      r.Configuration.MaxExpansionDepth,
	}
}

func isTerminalEventType(eventType string) bool {
	return eventType == "completed" || eventType == "failed" || eventType == "cancelled"
}
```

**Step 2: Write SSE tests**

Create `internal/server/http/progress_stream_test.go`:

Test the following scenarios:
1. SSE for already-completed review sends one event and closes
2. SSE for review without workflow ID returns error event
3. SSE headers are set correctly
4. `buildProgressData` returns correct values
5. `isTerminalEventType` returns correct results

For the polling/NOTIFY tests, use a mock review repo that returns changing status on successive calls.

**Step 3: Run tests**

```bash
cd literature_service && go test -short ./internal/server/http/... -v -count=1
```

**Step 4: Commit**

```bash
cd literature_service
git add internal/server/http/progress_stream.go internal/server/http/progress_stream_test.go
git commit -m "feat(http): implement SSE progress streaming with dual-source (NOTIFY + DB poll)"
```

---

## Task 9: Wire HTTP Server into cmd/server/main.go

**Files:**
- Modify: `cmd/server/main.go`

**Step 1: Add HTTP server creation and startup**

Add the import:

```go
httpserver "github.com/helixir/literature-review-service/internal/server/http"
```

After the gRPC server creation block, replace the existing HTTP health/metrics server with the new HTTP server:

```go
// Create HTTP REST API server.
// The chiauth middleware is created from the same auth config used for gRPC.
// TODO: Replace with chiauth.NewMiddleware(authConfig) once grpcauth/chiauth is available.
var httpAuthMiddleware func(http.Handler) http.Handler
// httpAuthMiddleware = chiauth.NewMiddleware(authConfig)  // uncomment when available

httpCfg := httpserver.Config{
    Address:         cfg.Server.HTTPAddress(),
    ReadTimeout:     cfg.Server.ReadTimeout,
    WriteTimeout:    5 * time.Minute, // Long timeout for SSE streaming
    IdleTimeout:     2 * time.Minute,
    ShutdownTimeout: cfg.Server.ShutdownTimeout,
}

httpSrv := httpserver.NewServer(
    httpCfg,
    workflowClient,
    reviewRepo,
    paperRepo,
    keywordRepo,
    db,
    logger,
    httpAuthMiddleware,
)

// Also set up Prometheus metrics handler on a separate port if configured.
var metricsServer *http.Server
if cfg.Metrics.Enabled {
    metricsMux := http.NewServeMux()
    metricsMux.Handle(cfg.Metrics.Path, promhttp.Handler())
    metricsServer = &http.Server{
        Addr:    fmt.Sprintf(":%d", cfg.Server.MetricsPort),
        Handler: metricsMux,
    }
}
```

Update the server startup goroutines:

```go
// Start HTTP REST API server in background.
go func() {
    logger.Info().
        Str("address", httpCfg.Address).
        Msg("HTTP REST API server starting")
    if err := httpSrv.Start(); err != nil && err != http.ErrServerClosed {
        errCh <- fmt.Errorf("HTTP server error: %w", err)
    }
}()

// Start metrics server if configured.
if metricsServer != nil {
    go func() {
        logger.Info().
            Str("address", metricsServer.Addr).
            Msg("metrics server starting")
        if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            errCh <- fmt.Errorf("metrics server error: %w", err)
        }
    }()
}
```

Update shutdown to include the new HTTP server:

```go
if err := httpSrv.Shutdown(shutdownCtx); err != nil {
    logger.Error().Err(err).Msg("HTTP server shutdown error")
}
if metricsServer != nil {
    if err := metricsServer.Shutdown(shutdownCtx); err != nil {
        logger.Error().Err(err).Msg("metrics server shutdown error")
    }
}
```

**Step 2: Verify compilation**

```bash
cd literature_service && go build ./cmd/server/...
```

Expected: Compiles successfully.

**Step 3: Run all tests**

```bash
cd literature_service && go test -short ./... -count=1
```

Expected: All tests pass.

**Step 4: Commit**

```bash
cd literature_service
git add cmd/server/main.go
git commit -m "feat(server): wire HTTP REST API server into main entrypoint"
```

---

## Task 10: Final Integration Verification

**Step 1: Run full test suite**

```bash
cd literature_service && go test -short -race ./... -count=1
```

Expected: All tests pass with no race conditions.

**Step 2: Verify all binaries build**

```bash
cd literature_service && make build
```

Or if no Makefile:

```bash
cd literature_service && go build ./cmd/server/... && go build ./cmd/worker/... && go build ./cmd/migrate/...
```

Expected: All binaries compile successfully.

**Step 3: Run linter**

```bash
cd literature_service && make lint
```

Or:

```bash
cd literature_service && golangci-lint run ./...
```

Expected: No new lint warnings.

**Step 4: Final commit if any cleanup needed**

```bash
cd literature_service
git add -A
git commit -m "chore(server): phase 5 api layer cleanup and final verification"
```

---

## Summary of Deliverables

| ID | Deliverable | Status | Files |
|----|-------------|--------|-------|
| D5.1 | gRPC Server with grpcauth | Task 1 | `cmd/server/main.go` |
| D5.2 | HTTP REST API | Tasks 3, 6, 7, 9 | `internal/server/http/*` |
| D5.3 | Tenant Middleware | Tasks 2, 4 | `internal/server/server.go`, `internal/server/http/middleware.go` |
| D5.4 | Progress Streaming (SSE) | Task 8 | `internal/server/http/progress_stream.go` |
| D5.5 | Error Handling | Tasks 6, 7 | `internal/server/http/handlers.go` |
| D5.6 | Request Validation | Tasks 6, 7 | `internal/server/http/handlers.go`, `internal/server/http/paper_handlers.go` |

## Dependencies

- `github.com/go-chi/chi/v5` — HTTP router
- `github.com/helixir/grpcauth` — Authentication/authorization (already a module dependency)
- `github.com/jackc/pgx/v5` — Postgres LISTEN/NOTIFY (already a dependency)
