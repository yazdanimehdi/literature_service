package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/helixir/literature-review-service/internal/domain"
)

const (
	// sseQueryInterval is how often we poll the DB for authoritative state.
	sseQueryInterval = 2 * time.Second
	// sseMaxDuration is the maximum time an SSE stream may remain open.
	sseMaxDuration = 4 * time.Hour
)

// sseEvent represents an event sent via SSE.
type sseEvent struct {
	EventType string            `json:"event_type"`
	ReviewID  string            `json:"review_id"`
	Status    string            `json:"status"`
	Progress  *progressResponse `json:"progress,omitempty"`
	Message   string            `json:"message"`
	Timestamp time.Time         `json:"timestamp"`
}

// streamProgress handles GET /literature-reviews/{reviewID}/progress (SSE).
func (s *Server) streamProgress(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromContext(r.Context())
	projectID := projectIDFromContext(r.Context())

	reviewID, ok := parseUUID(w, chi.URLParam(r, "reviewID"), "review_id")
	if !ok {
		return
	}

	review, err := s.reviewRepo.Get(r.Context(), orgID, projectID, reviewID)
	if err != nil {
		writeDomainError(w, err)
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
			current, pollErr := s.reviewRepo.Get(ctx, orgID, projectID, reviewID)
			if pollErr != nil {
				s.logger.Error().Err(pollErr).Str("review_id", reviewID.String()).Msg("failed to poll review status")
				continue
			}

			if current.Status.IsTerminal() {
				// Send only the final event to avoid duplicate terminal events.
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

			sendSSEEvent(w, flusher, sseEvent{
				EventType: "progress_update",
				ReviewID:  current.ID.String(),
				Status:    string(current.Status),
				Progress:  buildProgressData(current),
				Message:   "status: " + string(current.Status),
				Timestamp: time.Now(),
			})
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
	sanitizedChannel := pgx.Identifier{channel}.Sanitize()

	// Use the underlying pgx connection for LISTEN (not the pooled wrapper).
	pgConn := conn.Conn()
	if _, execErr := pgConn.Exec(ctx, fmt.Sprintf("LISTEN %s", sanitizedChannel)); execErr != nil {
		s.logger.Error().Err(execErr).Str("channel", channel).Msg("LISTEN failed")
		return
	}
	defer func() {
		_, _ = pgConn.Exec(context.Background(), fmt.Sprintf("UNLISTEN %s", sanitizedChannel))
	}()

	for {
		notification, waitErr := pgConn.WaitForNotification(ctx)
		if waitErr != nil {
			// Context cancelled or connection error.
			return
		}

		var event sseEvent
		if unmarshalErr := json.Unmarshal([]byte(notification.Payload), &event); unmarshalErr != nil {
			s.logger.Warn().Err(unmarshalErr).Msg("failed to parse notification payload")
			continue
		}
		event.ReviewID = reviewID
		event.Timestamp = time.Now()

		select {
		case out <- event:
		case <-ctx.Done():
			return
		default:
			// Channel full, drop event to avoid blocking the notification listener.
			s.logger.Warn().
				Str("review_id", reviewID).
				Str("event_type", event.EventType).
				Msg("SSE notification channel full, dropping event")
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

// buildProgressData converts a LiteratureReviewRequest into a progressResponse.
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

// isTerminalEventType returns true if the event type represents a terminal state.
func isTerminalEventType(eventType string) bool {
	return eventType == "completed" || eventType == "failed" || eventType == "cancelled"
}
