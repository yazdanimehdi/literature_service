package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/helixir/literature-review-service/internal/domain"
)

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
