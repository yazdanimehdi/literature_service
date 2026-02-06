//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"
)

func TestPgReviewRepository_Integration(t *testing.T) {
	cleanTable(t, "literature_review_requests")
	repo := repository.NewPgReviewRepository(testPool)
	ctx := context.Background()

	t.Run("Create and Get roundtrip", func(t *testing.T) {
		review := &domain.LiteratureReviewRequest{
			ID:            uuid.New(),
			OrgID:         "org-integration",
			ProjectID:     "proj-integration",
			UserID:        "user-integration",
			Title: "integration test query",
			Status:        domain.ReviewStatusPending,
			Configuration: domain.DefaultReviewConfiguration(),
			CreatedAt:     time.Now().UTC().Truncate(time.Microsecond),
			UpdatedAt:     time.Now().UTC().Truncate(time.Microsecond),
		}

		err := repo.Create(ctx, review)
		require.NoError(t, err)

		got, err := repo.Get(ctx, "org-integration", "proj-integration", review.ID)
		require.NoError(t, err)
		assert.Equal(t, review.ID, got.ID)
		assert.Equal(t, review.Title, got.Title)
		assert.Equal(t, domain.ReviewStatusPending, got.Status)
		assert.Equal(t, review.OrgID, got.OrgID)
		assert.Equal(t, review.ProjectID, got.ProjectID)
		assert.Equal(t, review.UserID, got.UserID)
	})

	t.Run("Create duplicate returns already exists", func(t *testing.T) {
		id := uuid.New()
		review := &domain.LiteratureReviewRequest{
			ID:            id,
			OrgID:         "org-integration",
			ProjectID:     "proj-integration",
			UserID:        "user-integration",
			Title: "duplicate test",
			Status:        domain.ReviewStatusPending,
			Configuration: domain.DefaultReviewConfiguration(),
			CreatedAt:     time.Now().UTC().Truncate(time.Microsecond),
			UpdatedAt:     time.Now().UTC().Truncate(time.Microsecond),
		}

		require.NoError(t, repo.Create(ctx, review))

		// Creating the same review again should fail.
		err := repo.Create(ctx, review)
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
	})

	t.Run("UpdateStatus transitions", func(t *testing.T) {
		review := &domain.LiteratureReviewRequest{
			ID:            uuid.New(),
			OrgID:         "org-integration",
			ProjectID:     "proj-integration",
			UserID:        "user-integration",
			Title: "status test",
			Status:        domain.ReviewStatusPending,
			Configuration: domain.DefaultReviewConfiguration(),
			CreatedAt:     time.Now().UTC().Truncate(time.Microsecond),
			UpdatedAt:     time.Now().UTC().Truncate(time.Microsecond),
		}
		require.NoError(t, repo.Create(ctx, review))

		// Pending -> ExtractingKeywords is a valid transition.
		err := repo.UpdateStatus(ctx, review.OrgID, review.ProjectID, review.ID, domain.ReviewStatusExtractingKeywords, "")
		require.NoError(t, err)

		got, err := repo.Get(ctx, review.OrgID, review.ProjectID, review.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.ReviewStatusExtractingKeywords, got.Status)
		assert.NotNil(t, got.StartedAt, "StartedAt should be set on transition to extracting_keywords")

		// ExtractingKeywords -> Searching is a valid transition.
		err = repo.UpdateStatus(ctx, review.OrgID, review.ProjectID, review.ID, domain.ReviewStatusSearching, "")
		require.NoError(t, err)

		got, err = repo.Get(ctx, review.OrgID, review.ProjectID, review.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.ReviewStatusSearching, got.Status)
	})

	t.Run("UpdateStatus invalid transition returns error", func(t *testing.T) {
		review := &domain.LiteratureReviewRequest{
			ID:            uuid.New(),
			OrgID:         "org-integration",
			ProjectID:     "proj-integration",
			UserID:        "user-integration",
			Title: "invalid transition test",
			Status:        domain.ReviewStatusPending,
			Configuration: domain.DefaultReviewConfiguration(),
			CreatedAt:     time.Now().UTC().Truncate(time.Microsecond),
			UpdatedAt:     time.Now().UTC().Truncate(time.Microsecond),
		}
		require.NoError(t, repo.Create(ctx, review))

		// Pending -> Searching is NOT a valid transition (must go through extracting_keywords).
		err := repo.UpdateStatus(ctx, review.OrgID, review.ProjectID, review.ID, domain.ReviewStatusSearching, "")
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrInvalidInput)
	})

	t.Run("List with filters", func(t *testing.T) {
		reviews, total, err := repo.List(ctx, repository.ReviewFilter{
			OrgID:     "org-integration",
			ProjectID: "proj-integration",
			Limit:     10,
			Offset:    0,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, int(total), 2, "should have at least 2 reviews from previous subtests")
		assert.NotEmpty(t, reviews)
	})

	t.Run("List with status filter", func(t *testing.T) {
		reviews, total, err := repo.List(ctx, repository.ReviewFilter{
			OrgID:  "org-integration",
			Status: []domain.ReviewStatus{domain.ReviewStatusPending},
			Limit:  10,
			Offset: 0,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, int(total), 1)
		for _, r := range reviews {
			assert.Equal(t, domain.ReviewStatusPending, r.Status)
		}
	})

	t.Run("Get with wrong org returns not found", func(t *testing.T) {
		review := &domain.LiteratureReviewRequest{
			ID:            uuid.New(),
			OrgID:         "org-a",
			ProjectID:     "proj-a",
			UserID:        "user-a",
			Title: "tenant isolation test",
			Status:        domain.ReviewStatusPending,
			Configuration: domain.DefaultReviewConfiguration(),
			CreatedAt:     time.Now().UTC().Truncate(time.Microsecond),
			UpdatedAt:     time.Now().UTC().Truncate(time.Microsecond),
		}
		require.NoError(t, repo.Create(ctx, review))

		// Try to access from a different org -- should return not found.
		_, err := repo.Get(ctx, "org-b", "proj-a", review.ID)
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("Get with wrong project returns not found", func(t *testing.T) {
		review := &domain.LiteratureReviewRequest{
			ID:            uuid.New(),
			OrgID:         "org-tenant-test",
			ProjectID:     "proj-tenant-test",
			UserID:        "user-tenant-test",
			Title: "project isolation test",
			Status:        domain.ReviewStatusPending,
			Configuration: domain.DefaultReviewConfiguration(),
			CreatedAt:     time.Now().UTC().Truncate(time.Microsecond),
			UpdatedAt:     time.Now().UTC().Truncate(time.Microsecond),
		}
		require.NoError(t, repo.Create(ctx, review))

		// Same org, different project -- should return not found.
		_, err := repo.Get(ctx, "org-tenant-test", "proj-other", review.ID)
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("IncrementCounters", func(t *testing.T) {
		review := &domain.LiteratureReviewRequest{
			ID:            uuid.New(),
			OrgID:         "org-integration",
			ProjectID:     "proj-integration",
			UserID:        "user-integration",
			Title: "counter test",
			Status:        domain.ReviewStatusPending,
			Configuration: domain.DefaultReviewConfiguration(),
			CreatedAt:     time.Now().UTC().Truncate(time.Microsecond),
			UpdatedAt:     time.Now().UTC().Truncate(time.Microsecond),
		}
		require.NoError(t, repo.Create(ctx, review))

		err := repo.IncrementCounters(ctx, review.OrgID, review.ProjectID, review.ID, 5, 3)
		require.NoError(t, err)

		got, err := repo.Get(ctx, review.OrgID, review.ProjectID, review.ID)
		require.NoError(t, err)
		assert.Equal(t, 5, got.PapersFoundCount)
		assert.Equal(t, 3, got.PapersIngestedCount)

		// Increment again to verify additive behavior.
		err = repo.IncrementCounters(ctx, review.OrgID, review.ProjectID, review.ID, 2, 1)
		require.NoError(t, err)

		got, err = repo.Get(ctx, review.OrgID, review.ProjectID, review.ID)
		require.NoError(t, err)
		assert.Equal(t, 7, got.PapersFoundCount)
		assert.Equal(t, 4, got.PapersIngestedCount)
	})

	t.Run("IncrementCounters nonexistent review returns not found", func(t *testing.T) {
		err := repo.IncrementCounters(ctx, "org-x", "proj-x", uuid.New(), 1, 1)
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestPgPaperRepository_Integration(t *testing.T) {
	cleanTable(t, "papers")
	repo := repository.NewPgPaperRepository(testPool)
	ctx := context.Background()

	t.Run("Create and GetByCanonicalID roundtrip", func(t *testing.T) {
		paper := &domain.Paper{
			ID:          uuid.New(),
			CanonicalID: "doi:10.1234/integration-test-create",
			Title:       "Integration Test Paper Create",
			Abstract:    "Test abstract for create roundtrip",
		}

		created, err := repo.Create(ctx, paper)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, created.ID)
		assert.False(t, created.CreatedAt.IsZero())
		assert.False(t, created.UpdatedAt.IsZero())

		got, err := repo.GetByCanonicalID(ctx, "doi:10.1234/integration-test-create")
		require.NoError(t, err)
		assert.Equal(t, created.ID, got.ID)
		assert.Equal(t, "Integration Test Paper Create", got.Title)
		assert.Equal(t, "Test abstract for create roundtrip", got.Abstract)
	})

	t.Run("Create upsert updates existing paper", func(t *testing.T) {
		paper := &domain.Paper{
			ID:            uuid.New(),
			CanonicalID:   "doi:10.1234/integration-test-upsert",
			Title:         "Original Title",
			Abstract:      "Original abstract",
			CitationCount: 10,
		}

		first, err := repo.Create(ctx, paper)
		require.NoError(t, err)

		// Upsert the same canonical ID with updated data.
		updated := &domain.Paper{
			ID:            uuid.New(), // different UUID, but same canonical ID
			CanonicalID:   "doi:10.1234/integration-test-upsert",
			Title:         "Updated Title",
			Abstract:      "Updated abstract",
			CitationCount: 20,
		}

		second, err := repo.Create(ctx, updated)
		require.NoError(t, err)
		assert.Equal(t, first.ID, second.ID, "upsert should return the same ID")
		assert.Equal(t, "Updated Title", second.Title)
		assert.Equal(t, 20, second.CitationCount, "should keep the greater citation count")
	})

	t.Run("GetByID", func(t *testing.T) {
		paper := &domain.Paper{
			ID:          uuid.New(),
			CanonicalID: "doi:10.1234/integration-test-getbyid",
			Title:       "GetByID Test Paper",
		}

		created, err := repo.Create(ctx, paper)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, created.ID, got.ID)
		assert.Equal(t, "GetByID Test Paper", got.Title)
	})

	t.Run("GetByID nonexistent returns not found", func(t *testing.T) {
		_, err := repo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("BulkUpsert idempotency", func(t *testing.T) {
		cleanTable(t, "papers")

		papers := []*domain.Paper{
			{
				ID:          uuid.New(),
				CanonicalID: "doi:10.1234/integration-test-1",
				Title:       "Integration Test Paper 1",
				Abstract:    "Test abstract 1",
			},
			{
				ID:          uuid.New(),
				CanonicalID: "doi:10.1234/integration-test-2",
				Title:       "Integration Test Paper 2",
				Abstract:    "Test abstract 2",
			},
		}

		// First upsert -- both papers are new.
		result, err := repo.BulkUpsert(ctx, papers)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		for _, p := range result {
			assert.NotEqual(t, uuid.Nil, p.ID)
			assert.False(t, p.CreatedAt.IsZero())
			assert.False(t, p.UpdatedAt.IsZero())
		}

		firstIDs := []uuid.UUID{result[0].ID, result[1].ID}

		// Second upsert with same canonical IDs -- should be idempotent.
		result2, err := repo.BulkUpsert(ctx, papers)
		require.NoError(t, err)
		assert.Len(t, result2, 2)

		// IDs should remain the same after the second upsert.
		assert.Equal(t, firstIDs[0], result2[0].ID)
		assert.Equal(t, firstIDs[1], result2[1].ID)
	})

	t.Run("BulkUpsert empty slice returns empty", func(t *testing.T) {
		result, err := repo.BulkUpsert(ctx, []*domain.Paper{})
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("MarkKeywordsExtracted", func(t *testing.T) {
		paper := &domain.Paper{
			ID:          uuid.New(),
			CanonicalID: "doi:10.1234/integration-test-keywords",
			Title:       "Keywords Extracted Test",
		}

		created, err := repo.Create(ctx, paper)
		require.NoError(t, err)
		assert.False(t, created.KeywordsExtracted)

		err = repo.MarkKeywordsExtracted(ctx, created.ID)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.True(t, got.KeywordsExtracted)
	})

	t.Run("MarkKeywordsExtracted nonexistent returns not found", func(t *testing.T) {
		err := repo.MarkKeywordsExtracted(ctx, uuid.New())
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestPgKeywordRepository_Integration(t *testing.T) {
	cleanTable(t, "keywords")
	repo := repository.NewPgKeywordRepository(testPool)
	ctx := context.Background()

	t.Run("GetOrCreate creates new keyword", func(t *testing.T) {
		kw, err := repo.GetOrCreate(ctx, "Machine Learning")
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, kw.ID)
		assert.Equal(t, "machine learning", kw.NormalizedKeyword)
		assert.False(t, kw.CreatedAt.IsZero())
	})

	t.Run("GetOrCreate returns existing keyword", func(t *testing.T) {
		kw1, err := repo.GetOrCreate(ctx, "Deep Learning")
		require.NoError(t, err)

		// Same keyword, different casing and spacing.
		kw2, err := repo.GetOrCreate(ctx, "  deep   LEARNING  ")
		require.NoError(t, err)

		assert.Equal(t, kw1.ID, kw2.ID, "should return the same keyword due to normalization")
		assert.Equal(t, kw1.NormalizedKeyword, kw2.NormalizedKeyword)
	})

	t.Run("GetByID", func(t *testing.T) {
		kw, err := repo.GetOrCreate(ctx, "Neural Networks")
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, kw.ID)
		require.NoError(t, err)
		assert.Equal(t, kw.ID, got.ID)
		assert.Equal(t, "neural networks", got.NormalizedKeyword)
	})

	t.Run("GetByID nonexistent returns not found", func(t *testing.T) {
		_, err := repo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("GetByNormalized", func(t *testing.T) {
		_, err := repo.GetOrCreate(ctx, "Transformer Architecture")
		require.NoError(t, err)

		got, err := repo.GetByNormalized(ctx, "transformer architecture")
		require.NoError(t, err)
		assert.Equal(t, "transformer architecture", got.NormalizedKeyword)
	})

	t.Run("BulkGetOrCreate", func(t *testing.T) {
		keywords := []string{"Protein Folding", "Gene Expression", "CRISPR"}
		results, err := repo.BulkGetOrCreate(ctx, keywords)
		require.NoError(t, err)
		assert.Len(t, results, 3)

		// Calling again with same keywords and some new ones.
		keywords2 := []string{"Protein Folding", "Drug Discovery", "CRISPR"}
		results2, err := repo.BulkGetOrCreate(ctx, keywords2)
		require.NoError(t, err)
		assert.Len(t, results2, 3)
	})

	t.Run("BulkGetOrCreate deduplicates within batch", func(t *testing.T) {
		keywords := []string{"Genomics", "GENOMICS", "  genomics  "}
		results, err := repo.BulkGetOrCreate(ctx, keywords)
		require.NoError(t, err)
		assert.Len(t, results, 1, "should deduplicate by normalized form")
	})

	t.Run("BulkGetOrCreate empty returns empty", func(t *testing.T) {
		results, err := repo.BulkGetOrCreate(ctx, []string{})
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}
