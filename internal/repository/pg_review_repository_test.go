package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixir/literature-review-service/internal/domain"
)

// Helper to create a valid review for testing.
func newTestReview() *domain.LiteratureReviewRequest {
	now := time.Now().UTC()
	return &domain.LiteratureReviewRequest{
		ID:             uuid.New(),
		OrgID:          "org-123",
		ProjectID:      "proj-456",
		UserID:         "user-789",
		OriginalQuery:  "CRISPR gene editing",
		Status:         domain.ReviewStatusPending,
		ExpansionDepth: 2,
		ConfigSnapshot: map[string]interface{}{
			"maxPapersPerKeyword": 100,
			"maxExpansionDepth":   3,
			"sourcePriorities": map[string]interface{}{
				"semantic_scholar": 1,
				"pubmed":           2,
			},
		},
		SourceFilters: map[string]interface{}{
			"sources": []string{"semantic_scholar", "pubmed"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestIsValidStatusTransition(t *testing.T) {
	tests := []struct {
		name     string
		from     domain.ReviewStatus
		to       domain.ReviewStatus
		expected bool
	}{
		// Pending transitions
		{
			name:     "pending to extracting_keywords is valid",
			from:     domain.ReviewStatusPending,
			to:       domain.ReviewStatusExtractingKeywords,
			expected: true,
		},
		{
			name:     "pending to failed is valid",
			from:     domain.ReviewStatusPending,
			to:       domain.ReviewStatusFailed,
			expected: true,
		},
		{
			name:     "pending to cancelled is valid",
			from:     domain.ReviewStatusPending,
			to:       domain.ReviewStatusCancelled,
			expected: true,
		},
		{
			name:     "pending to searching is invalid",
			from:     domain.ReviewStatusPending,
			to:       domain.ReviewStatusSearching,
			expected: false,
		},
		{
			name:     "pending to completed is invalid",
			from:     domain.ReviewStatusPending,
			to:       domain.ReviewStatusCompleted,
			expected: false,
		},

		// Extracting keywords transitions
		{
			name:     "extracting_keywords to searching is valid",
			from:     domain.ReviewStatusExtractingKeywords,
			to:       domain.ReviewStatusSearching,
			expected: true,
		},
		{
			name:     "extracting_keywords to failed is valid",
			from:     domain.ReviewStatusExtractingKeywords,
			to:       domain.ReviewStatusFailed,
			expected: true,
		},
		{
			name:     "extracting_keywords to cancelled is valid",
			from:     domain.ReviewStatusExtractingKeywords,
			to:       domain.ReviewStatusCancelled,
			expected: true,
		},
		{
			name:     "extracting_keywords to pending is invalid",
			from:     domain.ReviewStatusExtractingKeywords,
			to:       domain.ReviewStatusPending,
			expected: false,
		},
		{
			name:     "extracting_keywords to completed is invalid",
			from:     domain.ReviewStatusExtractingKeywords,
			to:       domain.ReviewStatusCompleted,
			expected: false,
		},

		// Searching transitions
		{
			name:     "searching to expanding is valid",
			from:     domain.ReviewStatusSearching,
			to:       domain.ReviewStatusExpanding,
			expected: true,
		},
		{
			name:     "searching to ingesting is valid",
			from:     domain.ReviewStatusSearching,
			to:       domain.ReviewStatusIngesting,
			expected: true,
		},
		{
			name:     "searching to completed is valid",
			from:     domain.ReviewStatusSearching,
			to:       domain.ReviewStatusCompleted,
			expected: true,
		},
		{
			name:     "searching to partial is valid",
			from:     domain.ReviewStatusSearching,
			to:       domain.ReviewStatusPartial,
			expected: true,
		},
		{
			name:     "searching to failed is valid",
			from:     domain.ReviewStatusSearching,
			to:       domain.ReviewStatusFailed,
			expected: true,
		},
		{
			name:     "searching to cancelled is valid",
			from:     domain.ReviewStatusSearching,
			to:       domain.ReviewStatusCancelled,
			expected: true,
		},
		{
			name:     "searching to pending is invalid",
			from:     domain.ReviewStatusSearching,
			to:       domain.ReviewStatusPending,
			expected: false,
		},

		// Expanding transitions
		{
			name:     "expanding to searching is valid",
			from:     domain.ReviewStatusExpanding,
			to:       domain.ReviewStatusSearching,
			expected: true,
		},
		{
			name:     "expanding to ingesting is valid",
			from:     domain.ReviewStatusExpanding,
			to:       domain.ReviewStatusIngesting,
			expected: true,
		},
		{
			name:     "expanding to failed is valid",
			from:     domain.ReviewStatusExpanding,
			to:       domain.ReviewStatusFailed,
			expected: true,
		},
		{
			name:     "expanding to cancelled is valid",
			from:     domain.ReviewStatusExpanding,
			to:       domain.ReviewStatusCancelled,
			expected: true,
		},
		{
			name:     "expanding to completed is invalid",
			from:     domain.ReviewStatusExpanding,
			to:       domain.ReviewStatusCompleted,
			expected: false,
		},

		// Ingesting transitions
		{
			name:     "ingesting to completed is valid",
			from:     domain.ReviewStatusIngesting,
			to:       domain.ReviewStatusCompleted,
			expected: true,
		},
		{
			name:     "ingesting to partial is valid",
			from:     domain.ReviewStatusIngesting,
			to:       domain.ReviewStatusPartial,
			expected: true,
		},
		{
			name:     "ingesting to failed is valid",
			from:     domain.ReviewStatusIngesting,
			to:       domain.ReviewStatusFailed,
			expected: true,
		},
		{
			name:     "ingesting to cancelled is valid",
			from:     domain.ReviewStatusIngesting,
			to:       domain.ReviewStatusCancelled,
			expected: true,
		},
		{
			name:     "ingesting to searching is invalid",
			from:     domain.ReviewStatusIngesting,
			to:       domain.ReviewStatusSearching,
			expected: false,
		},

		// Terminal states cannot transition
		{
			name:     "completed cannot transition to anything",
			from:     domain.ReviewStatusCompleted,
			to:       domain.ReviewStatusPending,
			expected: false,
		},
		{
			name:     "completed to failed is invalid",
			from:     domain.ReviewStatusCompleted,
			to:       domain.ReviewStatusFailed,
			expected: false,
		},
		{
			name:     "failed cannot transition to anything",
			from:     domain.ReviewStatusFailed,
			to:       domain.ReviewStatusPending,
			expected: false,
		},
		{
			name:     "failed to completed is invalid",
			from:     domain.ReviewStatusFailed,
			to:       domain.ReviewStatusCompleted,
			expected: false,
		},
		{
			name:     "cancelled cannot transition to anything",
			from:     domain.ReviewStatusCancelled,
			to:       domain.ReviewStatusPending,
			expected: false,
		},
		{
			name:     "cancelled to completed is invalid",
			from:     domain.ReviewStatusCancelled,
			to:       domain.ReviewStatusCompleted,
			expected: false,
		},
		{
			name:     "partial cannot transition to anything",
			from:     domain.ReviewStatusPartial,
			to:       domain.ReviewStatusCompleted,
			expected: false,
		},
		{
			name:     "partial to failed is invalid",
			from:     domain.ReviewStatusPartial,
			to:       domain.ReviewStatusFailed,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidStatusTransition(tt.from, tt.to)
			assert.Equal(t, tt.expected, result,
				"isValidStatusTransition(%s, %s) = %v, expected %v",
				tt.from, tt.to, result, tt.expected)
		})
	}
}

func TestNewPgReviewRepository(t *testing.T) {
	t.Run("creates repository with nil db", func(t *testing.T) {
		repo := NewPgReviewRepository(nil)
		assert.NotNil(t, repo)
		assert.Nil(t, repo.db)
	})

	t.Run("creates repository with mock db", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		assert.NotNil(t, repo)
		assert.NotNil(t, repo.db)
	})
}

func TestPgReviewRepository_Create(t *testing.T) {
	ctx := context.Background()

	t.Run("creates review successfully", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		review := newTestReview()

		mock.ExpectExec("INSERT INTO literature_review_requests").
			WithArgs(
				review.ID, review.OrgID, review.ProjectID, review.UserID, review.OriginalQuery,
				pgxmock.AnyArg(), pgxmock.AnyArg(), review.Status,
				review.KeywordsFoundCount, review.PapersFoundCount, review.PapersIngestedCount, review.PapersFailedCount,
				review.ExpansionDepth, pgxmock.AnyArg(), pgxmock.AnyArg(), review.DateFrom, review.DateTo,
				pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		err = repo.Create(ctx, review)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns validation error for nil review", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		err = repo.Create(ctx, nil)

		var validationErr *domain.ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, "review", validationErr.Field)
	})

	t.Run("returns validation error for missing ID", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		review := newTestReview()
		review.ID = uuid.Nil

		err = repo.Create(ctx, review)

		var validationErr *domain.ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, "id", validationErr.Field)
	})

	t.Run("returns validation error for missing org_id", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		review := newTestReview()
		review.OrgID = ""

		err = repo.Create(ctx, review)

		var validationErr *domain.ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, "org_id", validationErr.Field)
	})

	t.Run("returns validation error for missing project_id", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		review := newTestReview()
		review.ProjectID = ""

		err = repo.Create(ctx, review)

		var validationErr *domain.ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, "project_id", validationErr.Field)
	})

	t.Run("returns validation error for missing user_id", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		review := newTestReview()
		review.UserID = ""

		err = repo.Create(ctx, review)

		var validationErr *domain.ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, "user_id", validationErr.Field)
	})

	t.Run("returns already exists error on duplicate", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		review := newTestReview()

		// Simulate unique constraint violation
		pgErr := &pgconn.PgError{Code: "23505"}
		mock.ExpectExec("INSERT INTO literature_review_requests").
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
				pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
				pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
				pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
				pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnError(pgErr)

		err = repo.Create(ctx, review)

		assert.True(t, errors.Is(err, domain.ErrAlreadyExists))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgReviewRepository_Get(t *testing.T) {
	ctx := context.Background()

	t.Run("returns review when found", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		review := newTestReview()

		configJSON, _ := json.Marshal(review.ConfigSnapshot)
		sourceFiltersJSON, _ := json.Marshal(review.SourceFilters)

		rows := pgxmock.NewRows([]string{
			"id", "org_id", "project_id", "user_id", "original_query",
			"temporal_workflow_id", "temporal_run_id", "status",
			"keywords_found_count", "papers_found_count", "papers_ingested_count", "papers_failed_count",
			"expansion_depth", "config_snapshot", "source_filters", "date_from", "date_to",
			"created_at", "updated_at", "started_at", "completed_at",
		}).AddRow(
			review.ID, review.OrgID, review.ProjectID, review.UserID, review.OriginalQuery,
			nil, nil, review.Status,
			review.KeywordsFoundCount, review.PapersFoundCount, review.PapersIngestedCount, review.PapersFailedCount,
			review.ExpansionDepth, configJSON, sourceFiltersJSON, review.DateFrom, review.DateTo,
			review.CreatedAt, review.UpdatedAt, nil, nil,
		)

		mock.ExpectQuery("SELECT .* FROM literature_review_requests WHERE id = \\$1 AND org_id = \\$2 AND project_id = \\$3").
			WithArgs(review.ID, review.OrgID, review.ProjectID).
			WillReturnRows(rows)

		result, err := repo.Get(ctx, review.OrgID, review.ProjectID, review.ID)
		require.NoError(t, err)
		assert.Equal(t, review.ID, result.ID)
		assert.Equal(t, review.OrgID, result.OrgID)
		assert.Equal(t, review.ProjectID, result.ProjectID)
		assert.Equal(t, review.OriginalQuery, result.OriginalQuery)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns not found error when not exists", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		id := uuid.New()

		mock.ExpectQuery("SELECT .* FROM literature_review_requests WHERE id = \\$1 AND org_id = \\$2 AND project_id = \\$3").
			WithArgs(id, "org-123", "proj-456").
			WillReturnError(pgx.ErrNoRows)

		result, err := repo.Get(ctx, "org-123", "proj-456", id)
		assert.Nil(t, result)
		assert.True(t, errors.Is(err, domain.ErrNotFound))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgReviewRepository_GetByWorkflowID(t *testing.T) {
	ctx := context.Background()

	t.Run("returns validation error for empty workflow ID", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		result, err := repo.GetByWorkflowID(ctx, "")

		assert.Nil(t, result)
		var validationErr *domain.ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, "workflow_id", validationErr.Field)
	})

	t.Run("returns review when found", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		review := newTestReview()
		review.TemporalWorkflowID = "workflow-123"

		configJSON, _ := json.Marshal(review.ConfigSnapshot)
		sourceFiltersJSON, _ := json.Marshal(review.SourceFilters)

		rows := pgxmock.NewRows([]string{
			"id", "org_id", "project_id", "user_id", "original_query",
			"temporal_workflow_id", "temporal_run_id", "status",
			"keywords_found_count", "papers_found_count", "papers_ingested_count", "papers_failed_count",
			"expansion_depth", "config_snapshot", "source_filters", "date_from", "date_to",
			"created_at", "updated_at", "started_at", "completed_at",
		}).AddRow(
			review.ID, review.OrgID, review.ProjectID, review.UserID, review.OriginalQuery,
			&review.TemporalWorkflowID, nil, review.Status,
			review.KeywordsFoundCount, review.PapersFoundCount, review.PapersIngestedCount, review.PapersFailedCount,
			review.ExpansionDepth, configJSON, sourceFiltersJSON, review.DateFrom, review.DateTo,
			review.CreatedAt, review.UpdatedAt, nil, nil,
		)

		mock.ExpectQuery("SELECT .* FROM literature_review_requests WHERE temporal_workflow_id = \\$1").
			WithArgs("workflow-123").
			WillReturnRows(rows)

		result, err := repo.GetByWorkflowID(ctx, "workflow-123")
		require.NoError(t, err)
		assert.Equal(t, review.ID, result.ID)
		assert.Equal(t, "workflow-123", result.TemporalWorkflowID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns not found error when not exists", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)

		mock.ExpectQuery("SELECT .* FROM literature_review_requests WHERE temporal_workflow_id = \\$1").
			WithArgs("unknown-workflow").
			WillReturnError(pgx.ErrNoRows)

		result, err := repo.GetByWorkflowID(ctx, "unknown-workflow")
		assert.Nil(t, result)
		assert.True(t, errors.Is(err, domain.ErrNotFound))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgReviewRepository_IncrementCounters(t *testing.T) {
	ctx := context.Background()

	t.Run("increments counters successfully", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		id := uuid.New()

		mock.ExpectExec("UPDATE literature_review_requests SET papers_found_count").
			WithArgs(10, 5, pgxmock.AnyArg(), id, "org-123", "proj-456").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		err = repo.IncrementCounters(ctx, "org-123", "proj-456", id, 10, 5)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns not found error when no rows affected", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		id := uuid.New()

		mock.ExpectExec("UPDATE literature_review_requests SET papers_found_count").
			WithArgs(10, 5, pgxmock.AnyArg(), id, "org-123", "proj-456").
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		err = repo.IncrementCounters(ctx, "org-123", "proj-456", id, 10, 5)
		assert.True(t, errors.Is(err, domain.ErrNotFound))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgReviewRepository_List(t *testing.T) {
	ctx := context.Background()

	t.Run("returns validation error for missing org_id", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		filter := ReviewFilter{
			OrgID: "",
			Limit: 10,
		}

		results, count, err := repo.List(ctx, filter)
		assert.Nil(t, results)
		assert.Equal(t, int64(0), count)
		assert.Error(t, err)
	})

	t.Run("lists reviews with filters", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)
		review := newTestReview()

		configJSON, _ := json.Marshal(review.ConfigSnapshot)
		sourceFiltersJSON, _ := json.Marshal(review.SourceFilters)

		// Expect count query
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM literature_review_requests WHERE org_id = \\$1").
			WithArgs("org-123").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))

		// Expect select query
		rows := pgxmock.NewRows([]string{
			"id", "org_id", "project_id", "user_id", "original_query",
			"temporal_workflow_id", "temporal_run_id", "status",
			"keywords_found_count", "papers_found_count", "papers_ingested_count", "papers_failed_count",
			"expansion_depth", "config_snapshot", "source_filters", "date_from", "date_to",
			"created_at", "updated_at", "started_at", "completed_at",
		}).AddRow(
			review.ID, review.OrgID, review.ProjectID, review.UserID, review.OriginalQuery,
			nil, nil, review.Status,
			review.KeywordsFoundCount, review.PapersFoundCount, review.PapersIngestedCount, review.PapersFailedCount,
			review.ExpansionDepth, configJSON, sourceFiltersJSON, review.DateFrom, review.DateTo,
			review.CreatedAt, review.UpdatedAt, nil, nil,
		)

		mock.ExpectQuery("SELECT .* FROM literature_review_requests WHERE org_id = \\$1 ORDER BY created_at DESC LIMIT \\$2 OFFSET \\$3").
			WithArgs("org-123", 10, 0).
			WillReturnRows(rows)

		filter := ReviewFilter{
			OrgID:  "org-123",
			Limit:  10,
			Offset: 0,
		}

		results, count, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, int64(1), count)
		assert.Equal(t, review.ID, results[0].ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("lists reviews with status filter", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgReviewRepository(mock)

		// Expect count query with status filter
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM literature_review_requests WHERE org_id = \\$1 AND status IN \\(\\$2, \\$3\\)").
			WithArgs("org-123", domain.ReviewStatusPending, domain.ReviewStatusSearching).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		// Expect select query
		mock.ExpectQuery("SELECT .* FROM literature_review_requests WHERE org_id = \\$1 AND status IN \\(\\$2, \\$3\\) ORDER BY created_at DESC LIMIT \\$4 OFFSET \\$5").
			WithArgs("org-123", domain.ReviewStatusPending, domain.ReviewStatusSearching, 10, 0).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "org_id", "project_id", "user_id", "original_query",
				"temporal_workflow_id", "temporal_run_id", "status",
				"keywords_found_count", "papers_found_count", "papers_ingested_count", "papers_failed_count",
				"expansion_depth", "config_snapshot", "source_filters", "date_from", "date_to",
				"created_at", "updated_at", "started_at", "completed_at",
			}))

		filter := ReviewFilter{
			OrgID:  "org-123",
			Status: []domain.ReviewStatus{domain.ReviewStatusPending, domain.ReviewStatusSearching},
			Limit:  10,
			Offset: 0,
		}

		results, count, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Len(t, results, 0)
		assert.Equal(t, int64(0), count)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestReviewScanDest(t *testing.T) {
	t.Run("destinations returns correct number of pointers", func(t *testing.T) {
		var dest reviewScanDest
		dests := dest.destinations()
		// Should have exactly 21 destination pointers matching the SELECT columns
		assert.Len(t, dests, 21)
	})

	t.Run("finalize handles nullable fields", func(t *testing.T) {
		workflowID := "wf-123"
		runID := "run-456"

		dest := reviewScanDest{
			review: domain.LiteratureReviewRequest{
				ID:    uuid.New(),
				OrgID: "org-123",
			},
			temporalWorkflowID: &workflowID,
			temporalRunID:      &runID,
			configJSON:         []byte(`{"maxPapersPerKeyword":100}`),
			sourceFiltersJSON:  []byte(`{"sources":["pubmed","arxiv"]}`),
		}

		result, err := dest.finalize()
		require.NoError(t, err)
		assert.Equal(t, "wf-123", result.TemporalWorkflowID)
		assert.Equal(t, "run-456", result.TemporalRunID)
		assert.Equal(t, float64(100), result.ConfigSnapshot["maxPapersPerKeyword"])
		assert.Equal(t, []interface{}{"pubmed", "arxiv"}, result.SourceFilters["sources"])
	})

	t.Run("finalize handles nil nullable fields", func(t *testing.T) {
		dest := reviewScanDest{
			review: domain.LiteratureReviewRequest{
				ID:    uuid.New(),
				OrgID: "org-123",
			},
			temporalWorkflowID: nil,
			temporalRunID:      nil,
		}

		result, err := dest.finalize()
		require.NoError(t, err)
		assert.Equal(t, "", result.TemporalWorkflowID)
		assert.Equal(t, "", result.TemporalRunID)
	})

	t.Run("finalize returns error for invalid config JSON", func(t *testing.T) {
		dest := reviewScanDest{
			configJSON: []byte(`{invalid json`),
		}

		result, err := dest.finalize()
		assert.Nil(t, result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal config snapshot")
	})

	t.Run("finalize returns error for invalid source filters JSON", func(t *testing.T) {
		dest := reviewScanDest{
			sourceFiltersJSON: []byte(`{invalid json`),
		}

		result, err := dest.finalize()
		assert.Nil(t, result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal source filters")
	})
}

func TestIsPgUniqueViolation(t *testing.T) {
	t.Run("returns true for unique violation code", func(t *testing.T) {
		err := &pgconn.PgError{Code: "23505"}
		assert.True(t, isPgUniqueViolation(err))
	})

	t.Run("returns false for other pg error codes", func(t *testing.T) {
		err := &pgconn.PgError{Code: "23503"} // foreign key violation
		assert.False(t, isPgUniqueViolation(err))
	})

	t.Run("returns false for non-pg errors", func(t *testing.T) {
		err := errors.New("some error")
		assert.False(t, isPgUniqueViolation(err))
	})

	t.Run("returns false for nil", func(t *testing.T) {
		assert.False(t, isPgUniqueViolation(nil))
	})
}

func TestNullString(t *testing.T) {
	t.Run("returns nil for empty string", func(t *testing.T) {
		result := nullString("")
		assert.Nil(t, result)
	})

	t.Run("returns pointer for non-empty string", func(t *testing.T) {
		result := nullString("hello")
		assert.NotNil(t, result)
		assert.Equal(t, "hello", *result)
	})
}

func TestPgReviewRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Integration tests would go here, requiring a real PostgreSQL database.
	// These tests should:
	// 1. Set up a test database using testcontainers
	// 2. Run migrations
	// 3. Test all repository methods
	// 4. Clean up after each test

	t.Run("Create and Get", func(t *testing.T) {
		t.Skip("integration test requires PostgreSQL")
	})

	t.Run("Update with status transition", func(t *testing.T) {
		t.Skip("integration test requires PostgreSQL")
	})

	t.Run("List with filters", func(t *testing.T) {
		t.Skip("integration test requires PostgreSQL")
	})

	t.Run("IncrementCounters", func(t *testing.T) {
		t.Skip("integration test requires PostgreSQL")
	})

	t.Run("GetByWorkflowID", func(t *testing.T) {
		t.Skip("integration test requires PostgreSQL")
	})
}
