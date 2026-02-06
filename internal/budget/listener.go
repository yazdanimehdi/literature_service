// Package budget provides a Kafka listener for budget-related events
// that can trigger auto-resume of paused literature review workflows.
package budget

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
	"go.temporal.io/sdk/client"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"
	litemporal "github.com/helixir/literature-review-service/internal/temporal"
	"github.com/helixir/literature-review-service/internal/temporal/workflows"
)

// BudgetRefilledEvent represents the event from core_service when budget is refilled.
type BudgetRefilledEvent struct {
	OrgID         string  `json:"org_id"`
	ProjectID     string  `json:"project_id"` // Empty = org-wide refill
	AmountUSD     float64 `json:"amount_usd"`
	NewBalanceUSD float64 `json:"new_balance_usd"`
}

// Listener consumes budget events from Kafka and auto-resumes paused workflows.
type Listener struct {
	reader         *kafka.Reader
	workflowClient client.Client
	reviewRepo     repository.ReviewRepository
	logger         zerolog.Logger
}

// Config holds configuration for the budget listener.
type Config struct {
	// Brokers is the list of Kafka broker addresses.
	Brokers []string
	// Topic is the Kafka topic for budget events.
	Topic string
	// GroupID is the consumer group ID.
	GroupID string
}

// NewListener creates a new budget event listener.
func NewListener(
	cfg Config,
	workflowClient client.Client,
	reviewRepo repository.ReviewRepository,
	logger zerolog.Logger,
) *Listener {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  cfg.Brokers,
		Topic:    cfg.Topic,
		GroupID:  cfg.GroupID,
		MinBytes: 1,
		MaxBytes: 10e6,
		MaxWait:  3 * time.Second,
	})

	return &Listener{
		reader:         reader,
		workflowClient: workflowClient,
		reviewRepo:     reviewRepo,
		logger:         logger.With().Str("component", "budget_listener").Logger(),
	}
}

// Run starts the listener loop. Blocks until context is cancelled.
func (l *Listener) Run(ctx context.Context) error {
	l.logger.Info().Msg("starting budget listener")

	for {
		msg, err := l.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				l.logger.Info().Msg("budget listener stopped via context cancellation")
				return ctx.Err()
			}
			l.logger.Error().Err(err).Msg("failed to read message from Kafka")
			continue
		}

		l.logger.Debug().
			Int("partition", msg.Partition).
			Int64("offset", msg.Offset).
			Msg("received budget event")

		var event BudgetRefilledEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			l.logger.Error().Err(err).
				Str("raw_value", string(msg.Value)).
				Msg("failed to unmarshal budget event")
			continue
		}

		if err := l.handleBudgetRefilled(ctx, event); err != nil {
			l.logger.Error().Err(err).
				Str("org_id", event.OrgID).
				Str("project_id", event.ProjectID).
				Msg("failed to handle budget refill event")
		}
	}
}

// handleBudgetRefilled processes a budget refill event by resuming paused workflows.
func (l *Listener) handleBudgetRefilled(ctx context.Context, event BudgetRefilledEvent) error {
	l.logger.Info().
		Str("org_id", event.OrgID).
		Str("project_id", event.ProjectID).
		Float64("amount_usd", event.AmountUSD).
		Float64("new_balance_usd", event.NewBalanceUSD).
		Msg("handling budget refill")

	// Find all budget-paused workflows for this org/project.
	paused, err := l.reviewRepo.FindPausedByReason(ctx,
		event.OrgID,
		event.ProjectID,
		domain.PauseReasonBudgetExhausted,
	)
	if err != nil {
		return fmt.Errorf("find paused reviews: %w", err)
	}

	if len(paused) == 0 {
		l.logger.Debug().
			Str("org_id", event.OrgID).
			Str("project_id", event.ProjectID).
			Msg("no budget-paused workflows to resume")
		return nil
	}

	l.logger.Info().
		Int("count", len(paused)).
		Str("org_id", event.OrgID).
		Str("project_id", event.ProjectID).
		Msg("resuming budget-paused workflows")

	var resumeErrors int
	for _, review := range paused {
		workflowID := review.TemporalWorkflowID
		if workflowID == "" {
			l.logger.Warn().
				Str("review_id", review.ID.String()).
				Msg("paused review has no workflow ID, skipping")
			continue
		}

		err := l.workflowClient.SignalWorkflow(ctx,
			workflowID,
			"", // run ID - empty means latest run
			litemporal.SignalResume,
			workflows.ResumeSignal{ResumedBy: "budget_refill"},
		)
		if err != nil {
			l.logger.Error().Err(err).
				Str("workflow_id", workflowID).
				Str("review_id", review.ID.String()).
				Msg("failed to send resume signal to workflow")
			resumeErrors++
			// Continue with other workflows - don't fail the whole batch.
		} else {
			l.logger.Info().
				Str("workflow_id", workflowID).
				Str("review_id", review.ID.String()).
				Msg("sent resume signal to workflow")
		}
	}

	if resumeErrors > 0 {
		l.logger.Warn().
			Int("total", len(paused)).
			Int("errors", resumeErrors).
			Msg("some workflows failed to receive resume signal")
	}

	return nil
}

// Close closes the Kafka reader.
func (l *Listener) Close() error {
	l.logger.Info().Msg("closing budget listener")
	return l.reader.Close()
}
