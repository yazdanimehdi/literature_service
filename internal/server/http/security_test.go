package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/temporal"
)

// ---------------------------------------------------------------------------
// TestSQLInjection_QueryField
// ---------------------------------------------------------------------------

// TestSQLInjection_QueryField verifies that SQL injection payloads in the
// query field are treated as opaque data, never executed. The mock repository
// succeeds for every call, proving the payload is stored verbatim and the
// handler never panics or returns a 500.
func TestSQLInjection_QueryField(t *testing.T) {
	payloads := []struct {
		name  string
		query string
	}{
		{"drop table", "'; DROP TABLE papers; --"},
		{"boolean tautology", "1 OR 1=1"},
		{"union select", "' UNION SELECT * FROM users --"},
		{"bobby tables", "Robert'); DROP TABLE students;--"},
		{"nested quotes", "'' OR ''='"},
		{"comment injection", "query/* comment */"},
		{"hex encoding", "0x27204F52202731273D2731"},
		{"stacked queries", "'; EXEC xp_cmdshell('dir'); --"},
		{"sleep injection", "'; WAITFOR DELAY '00:00:05'; --"},
		{"batch separator", "query\nGO\nDROP TABLE papers"},
	}

	for _, tc := range payloads {
		t.Run(tc.name, func(t *testing.T) {
			var capturedQuery string

			reviewRepo := &mockReviewRepo{
				createFn: func(_ context.Context, review *domain.LiteratureReviewRequest) error {
					capturedQuery = review.Title
					return nil
				},
				updateFn: func(_ context.Context, _, _ string, _ uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
					return fn(&domain.LiteratureReviewRequest{})
				},
			}

			wfClient := &mockWorkflowClient{
				startFn: func(_ context.Context, req temporal.ReviewWorkflowRequest, _ interface{}, _ interface{}) (string, string, error) {
					return "wf-" + req.RequestID, "run-test", nil
				},
			}

			srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

			bodyMap := map[string]string{"title": tc.query}
			bodyBytes, err := json.Marshal(bodyMap)
			if err != nil {
				t.Fatalf("failed to marshal request body: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, buildPath("org-1", "proj-1", ""), bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			rr := serveHTTP(srv, req)

			// The handler must not panic and must not return 500.
			if rr.Code == http.StatusInternalServerError {
				t.Errorf("SQL injection payload %q caused a 500 response: %s", tc.query, rr.Body.String())
			}

			// If the review was created successfully (201), verify the query was stored verbatim.
			if rr.Code == http.StatusCreated {
				trimmedPayload := strings.TrimSpace(tc.query)
				if capturedQuery != trimmedPayload {
					t.Errorf("expected query to be stored verbatim as %q, got %q", trimmedPayload, capturedQuery)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestResponseSanitization
// ---------------------------------------------------------------------------

// TestResponseSanitization verifies that internal error details from
// dependencies (database driver errors, connection strings, IP addresses)
// are never leaked to the HTTP client. The writeDomainError function must
// return a generic message for unrecognized errors.
func TestResponseSanitization(t *testing.T) {
	sensitiveErrors := []struct {
		name        string
		err         error
		forbidden   []string
	}{
		{
			name:      "postgres connection refused",
			err:       fmt.Errorf("pgx: connection refused to 10.0.0.5:5432"),
			forbidden: []string{"pgx", "connection refused", "10.0.0.5", "5432"},
		},
		{
			name:      "authentication failure",
			err:       fmt.Errorf("password authentication failed for user \"litservice_user\""),
			forbidden: []string{"password", "litservice_user", "authentication"},
		},
		{
			name:      "stack trace leak",
			err:       fmt.Errorf("goroutine 42 [running]: runtime/debug.Stack()"),
			forbidden: []string{"goroutine", "runtime/debug", "Stack()"},
		},
		{
			name:      "file path leak",
			err:       fmt.Errorf("open /etc/secrets/db_password: no such file or directory"),
			forbidden: []string{"/etc/secrets", "db_password"},
		},
		{
			name:      "temporal internal error",
			err:       fmt.Errorf("rpc error: code = Unavailable desc = connection error: desc = \"transport: Error while dialing: dial tcp 10.0.1.20:7233\""),
			forbidden: []string{"10.0.1.20", "7233", "dial tcp", "transport"},
		},
	}

	for _, tc := range sensitiveErrors {
		t.Run(tc.name, func(t *testing.T) {
			reviewRepo := &mockReviewRepo{
				createFn: func(_ context.Context, _ *domain.LiteratureReviewRequest) error {
					return tc.err
				},
			}

			wfClient := &mockWorkflowClient{}
			srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

			body := `{"title":"test query for sanitization"}`
			req := httptest.NewRequest(http.MethodPost, buildPath("org-1", "proj-1", ""), bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")

			rr := serveHTTP(srv, req)

			responseBody := rr.Body.String()

			for _, fragment := range tc.forbidden {
				if strings.Contains(responseBody, fragment) {
					t.Errorf("response body contains sensitive fragment %q: %s", fragment, responseBody)
				}
			}

			// Verify the response contains only a generic error message.
			var resp map[string]string
			if err := json.NewDecoder(strings.NewReader(responseBody)).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if resp["error"] != "internal server error" {
				t.Errorf("expected generic error message, got %q", resp["error"])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestMaxQueryLength_Security
// ---------------------------------------------------------------------------

// TestMaxQueryLength_Security verifies that the query length boundary is
// enforced precisely at maxQueryLength. Exactly maxQueryLength characters
// must succeed; maxQueryLength+1 must be rejected with 400.
func TestMaxQueryLength_Security(t *testing.T) {
	t.Run("exactly maxQueryLength succeeds", func(t *testing.T) {
		reviewRepo := &mockReviewRepo{
			createFn: func(_ context.Context, _ *domain.LiteratureReviewRequest) error {
				return nil
			},
			updateFn: func(_ context.Context, _, _ string, _ uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
				return fn(&domain.LiteratureReviewRequest{})
			},
		}
		wfClient := &mockWorkflowClient{}
		srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

		query := strings.Repeat("a", maxQueryLength)
		bodyMap := map[string]string{"title": query}
		bodyBytes, _ := json.Marshal(bodyMap)

		req := httptest.NewRequest(http.MethodPost, buildPath("org-1", "proj-1", ""), bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		rr := serveHTTP(srv, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("expected 201 for exactly maxQueryLength query, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("maxQueryLength plus one is rejected", func(t *testing.T) {
		wfClient := &mockWorkflowClient{}
		srv := newTestHTTPServer(wfClient, &mockReviewRepo{}, &mockPaperRepo{}, &mockKeywordRepo{})

		query := strings.Repeat("a", maxQueryLength+1)
		bodyMap := map[string]string{"title": query}
		bodyBytes, _ := json.Marshal(bodyMap)

		req := httptest.NewRequest(http.MethodPost, buildPath("org-1", "proj-1", ""), bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		rr := serveHTTP(srv, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for maxQueryLength+1 query, got %d: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]string
		decodeJSON(t, rr, &resp)
		if !strings.Contains(resp["error"], "at most") {
			t.Errorf("expected error message about length limit, got %q", resp["error"])
		}
	})

	t.Run("one character below maxQueryLength succeeds", func(t *testing.T) {
		reviewRepo := &mockReviewRepo{
			createFn: func(_ context.Context, _ *domain.LiteratureReviewRequest) error {
				return nil
			},
			updateFn: func(_ context.Context, _, _ string, _ uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
				return fn(&domain.LiteratureReviewRequest{})
			},
		}
		wfClient := &mockWorkflowClient{}
		srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

		query := strings.Repeat("b", maxQueryLength-1)
		bodyMap := map[string]string{"title": query}
		bodyBytes, _ := json.Marshal(bodyMap)

		req := httptest.NewRequest(http.MethodPost, buildPath("org-1", "proj-1", ""), bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		rr := serveHTTP(srv, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("expected 201 for maxQueryLength-1 query, got %d: %s", rr.Code, rr.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// TestXSSPayload_QueryField
// ---------------------------------------------------------------------------

// TestXSSPayload_QueryField verifies that XSS payloads in the query field
// are safely handled in JSON responses. Go's encoding/json escapes HTML
// characters (<, >, &) by default, preventing reflected XSS in JSON.
func TestXSSPayload_QueryField(t *testing.T) {
	xssPayloads := []struct {
		name    string
		query   string
		mustNot []string // raw strings that must NOT appear unescaped in response
	}{
		{
			name:    "script tag",
			query:   "<script>alert('xss')</script>",
			mustNot: []string{"<script>", "</script>"},
		},
		{
			name:    "img onerror",
			query:   `<img src=x onerror=alert('xss')>`,
			mustNot: []string{"<img", "onerror="},
		},
		{
			name:    "event handler",
			query:   `" onmouseover="alert('xss')" "`,
			mustNot: []string{"onmouseover="},
		},
		{
			name:    "javascript protocol",
			query:   "javascript:alert('xss')",
			mustNot: nil, // no HTML here, just verify no panic and proper status
		},
		{
			name:    "svg tag",
			query:   `<svg/onload=alert('xss')>`,
			mustNot: []string{"<svg"},
		},
		{
			name:    "iframe injection",
			query:   `<iframe src="javascript:alert('xss')">`,
			mustNot: []string{"<iframe"},
		},
	}

	for _, tc := range xssPayloads {
		t.Run(tc.name, func(t *testing.T) {
			reviewRepo := &mockReviewRepo{
				createFn: func(_ context.Context, _ *domain.LiteratureReviewRequest) error {
					return nil
				},
				updateFn: func(_ context.Context, _, _ string, _ uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
					return fn(&domain.LiteratureReviewRequest{})
				},
			}

			wfClient := &mockWorkflowClient{
				startFn: func(_ context.Context, req temporal.ReviewWorkflowRequest, _ interface{}, _ interface{}) (string, string, error) {
					return "wf-" + req.RequestID, "run-test", nil
				},
			}

			srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

			bodyMap := map[string]string{"title": tc.query}
			bodyBytes, err := json.Marshal(bodyMap)
			if err != nil {
				t.Fatalf("failed to marshal request body: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, buildPath("org-1", "proj-1", ""), bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			rr := serveHTTP(srv, req)

			// Must not cause a 500 or panic.
			if rr.Code == http.StatusInternalServerError {
				t.Errorf("XSS payload %q caused a 500: %s", tc.query, rr.Body.String())
			}

			if rr.Code != http.StatusCreated {
				// A 400 is also acceptable for payloads that don't pass validation.
				return
			}

			// Verify that raw HTML characters are escaped in JSON output.
			responseBody := rr.Body.String()
			for _, forbidden := range tc.mustNot {
				if strings.Contains(responseBody, forbidden) {
					t.Errorf("response contains unescaped HTML %q in body: %s", forbidden, responseBody)
				}
			}

			// Verify the Content-Type is JSON (not HTML).
			contentType := rr.Header().Get("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				t.Errorf("expected Content-Type application/json, got %q", contentType)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestWriteDomainError_NeverLeaksInternalDetails
// ---------------------------------------------------------------------------

// TestWriteDomainError_NeverLeaksInternalDetails ensures that writeDomainError
// maps arbitrary error messages to generic responses and never reflects internal
// error text in the response body.
func TestWriteDomainError_NeverLeaksInternalDetails(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "generic error with DB details",
			err:            fmt.Errorf("FATAL: password authentication failed for user \"admin\""),
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "internal server error",
		},
		{
			name:           "wrapped postgres error",
			err:            fmt.Errorf("repository: %w", fmt.Errorf("pq: relation \"papers\" does not exist")),
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "internal server error",
		},
		{
			name:           "nil error is no-op",
			err:            nil,
			expectedStatus: http.StatusOK, // writeDomainError returns without writing on nil
			expectedBody:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeDomainError(rr, tc.err)

			if tc.err == nil {
				// writeDomainError should be a no-op for nil errors.
				if rr.Code != http.StatusOK {
					t.Errorf("expected no status change for nil error, got %d", rr.Code)
				}
				return
			}

			if rr.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, rr.Code)
			}

			var resp map[string]string
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if resp["error"] != tc.expectedBody {
				t.Errorf("expected error %q, got %q", tc.expectedBody, resp["error"])
			}

			// Verify original error message does not appear in response.
			if tc.err != nil && strings.Contains(rr.Body.String(), tc.err.Error()) {
				t.Errorf("response body contains raw error message: %s", rr.Body.String())
			}
		})
	}
}
