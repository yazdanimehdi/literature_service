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
