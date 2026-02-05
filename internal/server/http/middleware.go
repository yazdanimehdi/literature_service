package httpserver

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	grpcauth "github.com/helixir/grpcauth"
)

type contextKey string

const (
	ctxKeyOrgID     contextKey = "org_id"
	ctxKeyProjectID contextKey = "project_id"
)

// tenantContextMiddleware extracts orgID and projectID from URL path params
// and stores them in the request context.
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
func correlationIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = middleware.GetReqID(r.Context())
		}
		if correlationID == "" {
			buf := make([]byte, 8)
			if _, err := rand.Read(buf); err != nil {
				// Fallback to timestamp-based ID if crypto/rand fails.
				correlationID = fmt.Sprintf("%x", time.Now().UnixNano())
			} else {
				correlationID = fmt.Sprintf("%x", buf)
			}
		}

		w.Header().Set("X-Correlation-ID", correlationID)
		ctx := grpcauth.ContextWithCorrelationID(r.Context(), correlationID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// jsonContentTypeMiddleware sets Content-Type: application/json for all responses.
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
