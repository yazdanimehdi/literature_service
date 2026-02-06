package budget

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/helixir/literature-review-service/internal/domain"
	litemporal "github.com/helixir/literature-review-service/internal/temporal"
	"github.com/helixir/literature-review-service/internal/temporal/workflows"
)

// mockReviewRepository implements repository.ReviewRepository for testing.
type mockReviewRepository struct {
	mock.Mock
}

func (m *mockReviewRepository) FindPausedByReason(ctx context.Context, orgID, projectID string, reason domain.PauseReason) ([]*domain.LiteratureReviewRequest, error) {
	args := m.Called(ctx, orgID, projectID, reason)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.LiteratureReviewRequest), args.Error(1)
}

// Stub implementations for other repository.ReviewRepository methods.
func (m *mockReviewRepository) Create(ctx context.Context, review *domain.LiteratureReviewRequest) error {
	return nil
}
func (m *mockReviewRepository) Get(ctx context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
	return nil, nil
}
func (m *mockReviewRepository) Update(ctx context.Context, orgID, projectID string, id uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
	return nil
}
func (m *mockReviewRepository) UpdateStatus(ctx context.Context, orgID, projectID string, id uuid.UUID, status domain.ReviewStatus, errorMsg string) error {
	return nil
}
func (m *mockReviewRepository) List(ctx context.Context, filter interface{}) ([]*domain.LiteratureReviewRequest, int64, error) {
	return nil, 0, nil
}
func (m *mockReviewRepository) IncrementCounters(ctx context.Context, orgID, projectID string, id uuid.UUID, papersFound, papersIngested int) error {
	return nil
}
func (m *mockReviewRepository) GetByWorkflowID(ctx context.Context, workflowID string) (*domain.LiteratureReviewRequest, error) {
	return nil, nil
}
func (m *mockReviewRepository) UpdatePauseState(ctx context.Context, orgID, projectID string, requestID uuid.UUID, status domain.ReviewStatus, pauseReason domain.PauseReason, pausedAtPhase string) error {
	return nil
}

// mockWorkflowClient implements a subset of client.Client for testing.
type mockWorkflowClient struct {
	mock.Mock
}

func (m *mockWorkflowClient) SignalWorkflow(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error {
	args := m.Called(ctx, workflowID, runID, signalName, arg)
	return args.Error(0)
}

// testableListener is a version of Listener for unit testing that accepts mock dependencies.
type testableListener struct {
	workflowSignaler workflowSignaler
	reviewRepo       reviewFinder
	logger           zerolog.Logger
}

// workflowSignaler is an interface for sending signals to workflows.
type workflowSignaler interface {
	SignalWorkflow(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error
}

// reviewFinder is an interface for finding paused reviews.
type reviewFinder interface {
	FindPausedByReason(ctx context.Context, orgID, projectID string, reason domain.PauseReason) ([]*domain.LiteratureReviewRequest, error)
}

// handleBudgetRefilled processes a budget refill event by resuming paused workflows.
func (l *testableListener) handleBudgetRefilled(ctx context.Context, event BudgetRefilledEvent) error {
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
		return err
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

		err := l.workflowSignaler.SignalWorkflow(ctx,
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

func newTestLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func TestHandleBudgetRefilled_ResumesWorkflows(t *testing.T) {
	ctx := context.Background()

	review1 := &domain.LiteratureReviewRequest{
		ID:                 uuid.New(),
		OrgID:              "org-123",
		ProjectID:          "proj-456",
		TemporalWorkflowID: "workflow-1",
		Status:             domain.ReviewStatusPaused,
		PauseReason:        domain.PauseReasonBudgetExhausted,
	}
	review2 := &domain.LiteratureReviewRequest{
		ID:                 uuid.New(),
		OrgID:              "org-123",
		ProjectID:          "proj-456",
		TemporalWorkflowID: "workflow-2",
		Status:             domain.ReviewStatusPaused,
		PauseReason:        domain.PauseReasonBudgetExhausted,
	}
	pausedReviews := []*domain.LiteratureReviewRequest{review1, review2}

	mockRepo := new(mockReviewRepository)
	mockRepo.On("FindPausedByReason", ctx, "org-123", "proj-456", domain.PauseReasonBudgetExhausted).
		Return(pausedReviews, nil)

	mockClient := new(mockWorkflowClient)
	mockClient.On("SignalWorkflow", ctx, "workflow-1", "", litemporal.SignalResume, workflows.ResumeSignal{ResumedBy: "budget_refill"}).
		Return(nil)
	mockClient.On("SignalWorkflow", ctx, "workflow-2", "", litemporal.SignalResume, workflows.ResumeSignal{ResumedBy: "budget_refill"}).
		Return(nil)

	listener := &testableListener{
		workflowSignaler: mockClient,
		reviewRepo:       mockRepo,
		logger:           newTestLogger(),
	}

	event := BudgetRefilledEvent{
		OrgID:         "org-123",
		ProjectID:     "proj-456",
		AmountUSD:     100.0,
		NewBalanceUSD: 150.0,
	}

	err := listener.handleBudgetRefilled(ctx, event)

	require.NoError(t, err)
	mockRepo.AssertExpectations(t)
	mockClient.AssertExpectations(t)
	mockClient.AssertNumberOfCalls(t, "SignalWorkflow", 2)
}

func TestHandleBudgetRefilled_NoPausedWorkflows(t *testing.T) {
	ctx := context.Background()

	mockRepo := new(mockReviewRepository)
	mockRepo.On("FindPausedByReason", ctx, "org-123", "proj-456", domain.PauseReasonBudgetExhausted).
		Return([]*domain.LiteratureReviewRequest{}, nil)

	mockClient := new(mockWorkflowClient)

	listener := &testableListener{
		workflowSignaler: mockClient,
		reviewRepo:       mockRepo,
		logger:           newTestLogger(),
	}

	event := BudgetRefilledEvent{
		OrgID:         "org-123",
		ProjectID:     "proj-456",
		AmountUSD:     100.0,
		NewBalanceUSD: 150.0,
	}

	err := listener.handleBudgetRefilled(ctx, event)

	require.NoError(t, err)
	mockRepo.AssertExpectations(t)
	// Verify no signals were sent when no paused workflows exist.
	mockClient.AssertNotCalled(t, "SignalWorkflow", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestHandleBudgetRefilled_ContinuesOnSignalError(t *testing.T) {
	ctx := context.Background()

	review1 := &domain.LiteratureReviewRequest{
		ID:                 uuid.New(),
		OrgID:              "org-123",
		ProjectID:          "proj-456",
		TemporalWorkflowID: "workflow-1",
		Status:             domain.ReviewStatusPaused,
		PauseReason:        domain.PauseReasonBudgetExhausted,
	}
	review2 := &domain.LiteratureReviewRequest{
		ID:                 uuid.New(),
		OrgID:              "org-123",
		ProjectID:          "proj-456",
		TemporalWorkflowID: "workflow-2",
		Status:             domain.ReviewStatusPaused,
		PauseReason:        domain.PauseReasonBudgetExhausted,
	}
	review3 := &domain.LiteratureReviewRequest{
		ID:                 uuid.New(),
		OrgID:              "org-123",
		ProjectID:          "proj-456",
		TemporalWorkflowID: "workflow-3",
		Status:             domain.ReviewStatusPaused,
		PauseReason:        domain.PauseReasonBudgetExhausted,
	}
	pausedReviews := []*domain.LiteratureReviewRequest{review1, review2, review3}

	mockRepo := new(mockReviewRepository)
	mockRepo.On("FindPausedByReason", ctx, "org-123", "proj-456", domain.PauseReasonBudgetExhausted).
		Return(pausedReviews, nil)

	mockClient := new(mockWorkflowClient)
	// First workflow succeeds.
	mockClient.On("SignalWorkflow", ctx, "workflow-1", "", litemporal.SignalResume, workflows.ResumeSignal{ResumedBy: "budget_refill"}).
		Return(nil)
	// Second workflow fails.
	mockClient.On("SignalWorkflow", ctx, "workflow-2", "", litemporal.SignalResume, workflows.ResumeSignal{ResumedBy: "budget_refill"}).
		Return(errors.New("workflow not found"))
	// Third workflow succeeds (proves we continue after error).
	mockClient.On("SignalWorkflow", ctx, "workflow-3", "", litemporal.SignalResume, workflows.ResumeSignal{ResumedBy: "budget_refill"}).
		Return(nil)

	listener := &testableListener{
		workflowSignaler: mockClient,
		reviewRepo:       mockRepo,
		logger:           newTestLogger(),
	}

	event := BudgetRefilledEvent{
		OrgID:         "org-123",
		ProjectID:     "proj-456",
		AmountUSD:     100.0,
		NewBalanceUSD: 150.0,
	}

	// Even with signal errors, handleBudgetRefilled should return nil
	// because it continues processing other workflows.
	err := listener.handleBudgetRefilled(ctx, event)

	require.NoError(t, err)
	mockRepo.AssertExpectations(t)
	mockClient.AssertExpectations(t)
	// All three workflows should have received signal attempts.
	mockClient.AssertNumberOfCalls(t, "SignalWorkflow", 3)
}

func TestHandleBudgetRefilled_RepositoryError(t *testing.T) {
	ctx := context.Background()

	expectedErr := errors.New("database connection lost")

	mockRepo := new(mockReviewRepository)
	mockRepo.On("FindPausedByReason", ctx, "org-123", "proj-456", domain.PauseReasonBudgetExhausted).
		Return(nil, expectedErr)

	mockClient := new(mockWorkflowClient)

	listener := &testableListener{
		workflowSignaler: mockClient,
		reviewRepo:       mockRepo,
		logger:           newTestLogger(),
	}

	event := BudgetRefilledEvent{
		OrgID:         "org-123",
		ProjectID:     "proj-456",
		AmountUSD:     100.0,
		NewBalanceUSD: 150.0,
	}

	err := listener.handleBudgetRefilled(ctx, event)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	mockRepo.AssertExpectations(t)
	// No signals should be sent when repository lookup fails.
	mockClient.AssertNotCalled(t, "SignalWorkflow", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestHandleBudgetRefilled_SkipsReviewsWithoutWorkflowID(t *testing.T) {
	ctx := context.Background()

	reviewWithWorkflowID := &domain.LiteratureReviewRequest{
		ID:                 uuid.New(),
		OrgID:              "org-123",
		ProjectID:          "proj-456",
		TemporalWorkflowID: "workflow-1",
		Status:             domain.ReviewStatusPaused,
		PauseReason:        domain.PauseReasonBudgetExhausted,
	}
	reviewWithoutWorkflowID := &domain.LiteratureReviewRequest{
		ID:                 uuid.New(),
		OrgID:              "org-123",
		ProjectID:          "proj-456",
		TemporalWorkflowID: "", // No workflow ID.
		Status:             domain.ReviewStatusPaused,
		PauseReason:        domain.PauseReasonBudgetExhausted,
	}
	pausedReviews := []*domain.LiteratureReviewRequest{reviewWithWorkflowID, reviewWithoutWorkflowID}

	mockRepo := new(mockReviewRepository)
	mockRepo.On("FindPausedByReason", ctx, "org-123", "proj-456", domain.PauseReasonBudgetExhausted).
		Return(pausedReviews, nil)

	mockClient := new(mockWorkflowClient)
	// Only the review with a workflow ID should receive a signal.
	mockClient.On("SignalWorkflow", ctx, "workflow-1", "", litemporal.SignalResume, workflows.ResumeSignal{ResumedBy: "budget_refill"}).
		Return(nil)

	listener := &testableListener{
		workflowSignaler: mockClient,
		reviewRepo:       mockRepo,
		logger:           newTestLogger(),
	}

	event := BudgetRefilledEvent{
		OrgID:         "org-123",
		ProjectID:     "proj-456",
		AmountUSD:     100.0,
		NewBalanceUSD: 150.0,
	}

	err := listener.handleBudgetRefilled(ctx, event)

	require.NoError(t, err)
	mockRepo.AssertExpectations(t)
	mockClient.AssertExpectations(t)
	// Only one signal should be sent (the review with a workflow ID).
	mockClient.AssertNumberOfCalls(t, "SignalWorkflow", 1)
}

func TestHandleBudgetRefilled_OrgWideRefill(t *testing.T) {
	ctx := context.Background()

	review := &domain.LiteratureReviewRequest{
		ID:                 uuid.New(),
		OrgID:              "org-123",
		ProjectID:          "proj-789",
		TemporalWorkflowID: "workflow-1",
		Status:             domain.ReviewStatusPaused,
		PauseReason:        domain.PauseReasonBudgetExhausted,
	}
	pausedReviews := []*domain.LiteratureReviewRequest{review}

	mockRepo := new(mockReviewRepository)
	// Empty project ID indicates org-wide refill.
	mockRepo.On("FindPausedByReason", ctx, "org-123", "", domain.PauseReasonBudgetExhausted).
		Return(pausedReviews, nil)

	mockClient := new(mockWorkflowClient)
	mockClient.On("SignalWorkflow", ctx, "workflow-1", "", litemporal.SignalResume, workflows.ResumeSignal{ResumedBy: "budget_refill"}).
		Return(nil)

	listener := &testableListener{
		workflowSignaler: mockClient,
		reviewRepo:       mockRepo,
		logger:           newTestLogger(),
	}

	event := BudgetRefilledEvent{
		OrgID:         "org-123",
		ProjectID:     "", // Empty = org-wide refill.
		AmountUSD:     500.0,
		NewBalanceUSD: 1000.0,
	}

	err := listener.handleBudgetRefilled(ctx, event)

	require.NoError(t, err)
	mockRepo.AssertExpectations(t)
	mockClient.AssertExpectations(t)
}

func TestBudgetRefilledEvent_JSONUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected BudgetRefilledEvent
	}{
		{
			name: "full event with project",
			json: `{"org_id":"org-123","project_id":"proj-456","amount_usd":100.50,"new_balance_usd":250.75}`,
			expected: BudgetRefilledEvent{
				OrgID:         "org-123",
				ProjectID:     "proj-456",
				AmountUSD:     100.50,
				NewBalanceUSD: 250.75,
			},
		},
		{
			name: "org-wide event without project",
			json: `{"org_id":"org-123","project_id":"","amount_usd":500.00,"new_balance_usd":1000.00}`,
			expected: BudgetRefilledEvent{
				OrgID:         "org-123",
				ProjectID:     "",
				AmountUSD:     500.00,
				NewBalanceUSD: 1000.00,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var event BudgetRefilledEvent
			err := json.Unmarshal([]byte(tc.json), &event)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, event)
		})
	}
}
