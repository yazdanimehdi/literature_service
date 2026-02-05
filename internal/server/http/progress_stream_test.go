package httpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/helixir/literature-review-service/internal/domain"
)

// ---------------------------------------------------------------------------
// Tests: streamProgress
// ---------------------------------------------------------------------------

func TestStreamProgress_TerminalReview(t *testing.T) {
	reviewID := uuid.New()
	now := time.Now()

	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			if id != reviewID {
				return nil, domain.NewNotFoundError("review", id.String())
			}
			return &domain.LiteratureReviewRequest{
				ID:                  reviewID,
				OrgID:               orgID,
				ProjectID:           projectID,
				Status:              domain.ReviewStatusCompleted,
				PapersFoundCount:    50,
				PapersIngestedCount: 45,
				PapersFailedCount:   5,
				KeywordsFoundCount:  12,
				Configuration:       domain.DefaultReviewConfiguration(),
				CreatedAt:           now,
				UpdatedAt:           now,
			}, nil
		},
	}

	srv := newTestHTTPServer(&mockWorkflowClient{}, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet, buildPath("org-1", "proj-1", "/"+reviewID.String()+"/progress"), nil)
	rr := serveHTTP(srv, req)

	// Verify SSE headers.
	if ct := rr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %q", cc)
	}
	if conn := rr.Header().Get("Connection"); conn != "keep-alive" {
		t.Errorf("expected Connection keep-alive, got %q", conn)
	}
	if xab := rr.Header().Get("X-Accel-Buffering"); xab != "no" {
		t.Errorf("expected X-Accel-Buffering no, got %q", xab)
	}

	// Parse SSE events from the response body.
	events := parseSSEEvents(t, rr.Body.String())

	if len(events) != 1 {
		t.Fatalf("expected exactly 1 SSE event, got %d", len(events))
	}

	event := events[0]
	if event.eventType != "completed" {
		t.Errorf("expected event type completed, got %q", event.eventType)
	}

	var sseEvt sseEvent
	if err := json.Unmarshal([]byte(event.data), &sseEvt); err != nil {
		t.Fatalf("failed to parse SSE data JSON: %v", err)
	}

	if sseEvt.EventType != "completed" {
		t.Errorf("expected event_type completed, got %q", sseEvt.EventType)
	}
	if sseEvt.ReviewID != reviewID.String() {
		t.Errorf("expected review_id %s, got %s", reviewID.String(), sseEvt.ReviewID)
	}
	if sseEvt.Status != string(domain.ReviewStatusCompleted) {
		t.Errorf("expected status %q, got %q", domain.ReviewStatusCompleted, sseEvt.Status)
	}
	if sseEvt.Message != "review is in terminal state" {
		t.Errorf("expected message 'review is in terminal state', got %q", sseEvt.Message)
	}
	if sseEvt.Progress == nil {
		t.Fatal("expected progress to be set")
	}
	if sseEvt.Progress.PapersFound != 50 {
		t.Errorf("expected papers_found 50, got %d", sseEvt.Progress.PapersFound)
	}
	if sseEvt.Progress.PapersIngested != 45 {
		t.Errorf("expected papers_ingested 45, got %d", sseEvt.Progress.PapersIngested)
	}
	if sseEvt.Progress.PapersFailed != 5 {
		t.Errorf("expected papers_failed 5, got %d", sseEvt.Progress.PapersFailed)
	}
}

func TestStreamProgress_NoWorkflowID(t *testing.T) {
	reviewID := uuid.New()
	now := time.Now()

	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			if id != reviewID {
				return nil, domain.NewNotFoundError("review", id.String())
			}
			return &domain.LiteratureReviewRequest{
				ID:                 reviewID,
				OrgID:              orgID,
				ProjectID:          projectID,
				Status:             domain.ReviewStatusPending,
				TemporalWorkflowID: "", // No workflow.
				Configuration:      domain.DefaultReviewConfiguration(),
				CreatedAt:          now,
				UpdatedAt:          now,
			}, nil
		},
	}

	srv := newTestHTTPServer(&mockWorkflowClient{}, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet, buildPath("org-1", "proj-1", "/"+reviewID.String()+"/progress"), nil)
	rr := serveHTTP(srv, req)

	// Verify SSE headers.
	if ct := rr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}

	// Parse SSE events.
	events := parseSSEEvents(t, rr.Body.String())

	if len(events) != 1 {
		t.Fatalf("expected exactly 1 SSE event, got %d", len(events))
	}

	event := events[0]
	if event.eventType != "error" {
		t.Errorf("expected event type error, got %q", event.eventType)
	}

	var sseEvt sseEvent
	if err := json.Unmarshal([]byte(event.data), &sseEvt); err != nil {
		t.Fatalf("failed to parse SSE data JSON: %v", err)
	}

	if sseEvt.EventType != "error" {
		t.Errorf("expected event_type error, got %q", sseEvt.EventType)
	}
	if sseEvt.ReviewID != reviewID.String() {
		t.Errorf("expected review_id %s, got %s", reviewID.String(), sseEvt.ReviewID)
	}
	if sseEvt.Status != string(domain.ReviewStatusPending) {
		t.Errorf("expected status %q, got %q", domain.ReviewStatusPending, sseEvt.Status)
	}
	if sseEvt.Message != "review has no associated workflow" {
		t.Errorf("expected message 'review has no associated workflow', got %q", sseEvt.Message)
	}
}

func TestStreamProgress_NotFound(t *testing.T) {
	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, _, _ string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return nil, domain.NewNotFoundError("review", id.String())
		},
	}

	srv := newTestHTTPServer(&mockWorkflowClient{}, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet, buildPath("org-1", "proj-1", "/"+uuid.New().String()+"/progress"), nil)
	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestStreamProgress_InvalidUUID(t *testing.T) {
	srv := newTestHTTPServer(&mockWorkflowClient{}, &mockReviewRepo{}, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet, buildPath("org-1", "proj-1", "/not-a-uuid/progress"), nil)
	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Tests: buildProgressData
// ---------------------------------------------------------------------------

func TestBuildProgressData(t *testing.T) {
	review := &domain.LiteratureReviewRequest{
		ID:                  uuid.New(),
		PapersFoundCount:    100,
		PapersIngestedCount: 85,
		PapersFailedCount:   10,
		KeywordsFoundCount:  20,
		Configuration: domain.ReviewConfiguration{
			MaxKeywordsPerRound: 15,
			MaxExpansionDepth:   3,
		},
	}

	progress := buildProgressData(review)

	if progress == nil {
		t.Fatal("expected non-nil progress")
	}
	if progress.InitialKeywordsCount != 15 {
		t.Errorf("expected InitialKeywordsCount 15, got %d", progress.InitialKeywordsCount)
	}
	if progress.TotalKeywordsProcessed != 20 {
		t.Errorf("expected TotalKeywordsProcessed 20, got %d", progress.TotalKeywordsProcessed)
	}
	if progress.PapersFound != 100 {
		t.Errorf("expected PapersFound 100, got %d", progress.PapersFound)
	}
	if progress.PapersNew != 100 {
		t.Errorf("expected PapersNew 100, got %d", progress.PapersNew)
	}
	if progress.PapersIngested != 85 {
		t.Errorf("expected PapersIngested 85, got %d", progress.PapersIngested)
	}
	if progress.PapersFailed != 10 {
		t.Errorf("expected PapersFailed 10, got %d", progress.PapersFailed)
	}
	if progress.MaxExpansionDepth != 3 {
		t.Errorf("expected MaxExpansionDepth 3, got %d", progress.MaxExpansionDepth)
	}
}

func TestBuildProgressData_ZeroValues(t *testing.T) {
	review := &domain.LiteratureReviewRequest{
		Configuration: domain.ReviewConfiguration{},
	}

	progress := buildProgressData(review)

	if progress == nil {
		t.Fatal("expected non-nil progress")
	}
	if progress.InitialKeywordsCount != 0 {
		t.Errorf("expected InitialKeywordsCount 0, got %d", progress.InitialKeywordsCount)
	}
	if progress.PapersFound != 0 {
		t.Errorf("expected PapersFound 0, got %d", progress.PapersFound)
	}
}

// ---------------------------------------------------------------------------
// Tests: isTerminalEventType
// ---------------------------------------------------------------------------

func TestIsTerminalEventType(t *testing.T) {
	tests := []struct {
		eventType string
		expected  bool
	}{
		{"completed", true},
		{"failed", true},
		{"cancelled", true},
		{"progress_update", false},
		{"stream_started", false},
		{"error", false},
		{"timeout", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.eventType, func(t *testing.T) {
			got := isTerminalEventType(tc.eventType)
			if got != tc.expected {
				t.Errorf("isTerminalEventType(%q) = %v, want %v", tc.eventType, got, tc.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests: sendSSEEvent
// ---------------------------------------------------------------------------

func TestSendSSEEvent(t *testing.T) {
	rr := httptest.NewRecorder()
	now := time.Date(2026, 2, 4, 12, 0, 0, 0, time.UTC)

	event := sseEvent{
		EventType: "progress_update",
		ReviewID:  "abc-123",
		Status:    "searching",
		Progress: &progressResponse{
			PapersFound:    42,
			PapersIngested: 30,
		},
		Message:   "status: searching",
		Timestamp: now,
	}

	sendSSEEvent(rr, rr, event)

	body := rr.Body.String()

	// Verify SSE format: event: <type>\ndata: <json>\n\n
	if !strings.HasPrefix(body, "event: progress_update\n") {
		t.Errorf("expected body to start with 'event: progress_update\\n', got:\n%s", body)
	}
	if !strings.Contains(body, "data: ") {
		t.Errorf("expected body to contain 'data: ', got:\n%s", body)
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Errorf("expected body to end with '\\n\\n', got:\n%q", body)
	}

	// Extract the data JSON and verify it can be unmarshalled.
	events := parseSSEEvents(t, body)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	var parsed sseEvent
	if err := json.Unmarshal([]byte(events[0].data), &parsed); err != nil {
		t.Fatalf("failed to parse SSE data as JSON: %v", err)
	}

	if parsed.EventType != "progress_update" {
		t.Errorf("expected event_type progress_update, got %q", parsed.EventType)
	}
	if parsed.ReviewID != "abc-123" {
		t.Errorf("expected review_id abc-123, got %q", parsed.ReviewID)
	}
	if parsed.Status != "searching" {
		t.Errorf("expected status searching, got %q", parsed.Status)
	}
	if parsed.Progress == nil {
		t.Fatal("expected progress to be present")
	}
	if parsed.Progress.PapersFound != 42 {
		t.Errorf("expected papers_found 42, got %d", parsed.Progress.PapersFound)
	}
	if parsed.Message != "status: searching" {
		t.Errorf("expected message 'status: searching', got %q", parsed.Message)
	}
}

func TestSendSSEEvent_MinimalEvent(t *testing.T) {
	rr := httptest.NewRecorder()

	event := sseEvent{
		EventType: "error",
		ReviewID:  "def-456",
		Message:   "something went wrong",
		Timestamp: time.Now(),
	}

	sendSSEEvent(rr, rr, event)

	body := rr.Body.String()

	if !strings.HasPrefix(body, "event: error\n") {
		t.Errorf("expected event type 'error', got:\n%s", body)
	}

	events := parseSSEEvents(t, body)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	var parsed sseEvent
	if err := json.Unmarshal([]byte(events[0].data), &parsed); err != nil {
		t.Fatalf("failed to parse SSE data: %v", err)
	}

	if parsed.Progress != nil {
		t.Error("expected nil progress for minimal event")
	}
}

// ---------------------------------------------------------------------------
// Tests: SSE constants
// ---------------------------------------------------------------------------

func TestSSEConstants(t *testing.T) {
	if sseQueryInterval != 2*time.Second {
		t.Errorf("expected sseQueryInterval 2s, got %v", sseQueryInterval)
	}
	if sseMaxDuration != 4*time.Hour {
		t.Errorf("expected sseMaxDuration 4h, got %v", sseMaxDuration)
	}
}

// ---------------------------------------------------------------------------
// SSE parsing helper
// ---------------------------------------------------------------------------

type parsedSSEEvent struct {
	eventType string
	data      string
}

// parseSSEEvents parses SSE-formatted text into individual events.
// Each event is separated by a blank line ("\n\n").
// Lines starting with "event:" set the event type.
// Lines starting with "data:" contain the event payload.
func parseSSEEvents(t *testing.T, body string) []parsedSSEEvent {
	t.Helper()
	var events []parsedSSEEvent
	var current parsedSSEEvent

	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// Empty line = event boundary.
			if current.eventType != "" || current.data != "" {
				events = append(events, current)
				current = parsedSSEEvent{}
			}
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			current.eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			current.data = strings.TrimPrefix(line, "data: ")
		}
	}

	// Handle case where body doesn't end with blank line.
	if current.eventType != "" || current.data != "" {
		events = append(events, current)
	}

	return events
}
