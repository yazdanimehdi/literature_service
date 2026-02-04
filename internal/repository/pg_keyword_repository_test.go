package repository

import (
	"context"
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

func TestPgKeywordRepository_GetOrCreate(t *testing.T) {
	t.Run("creates new keyword when not found", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		// First, expect the GetByNormalized query to return no rows
		mock.ExpectQuery(`SELECT id, keyword, normalized_keyword, created_at FROM keywords WHERE normalized_keyword = \$1`).
			WithArgs("machine learning").
			WillReturnError(pgx.ErrNoRows)

		// Then expect the insert
		keywordID := uuid.New()
		now := time.Now().UTC()
		mock.ExpectQuery(`INSERT INTO keywords`).
			WithArgs(pgxmock.AnyArg(), "Machine Learning", "machine learning", pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"id", "keyword", "normalized_keyword", "created_at"}).
				AddRow(keywordID, "Machine Learning", "machine learning", now))

		result, err := repo.GetOrCreate(ctx, "Machine Learning")
		require.NoError(t, err)
		assert.Equal(t, "machine learning", result.NormalizedKeyword)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns existing keyword", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		now := time.Now().UTC()
		mock.ExpectQuery(`SELECT id, keyword, normalized_keyword, created_at FROM keywords WHERE normalized_keyword = \$1`).
			WithArgs("machine learning").
			WillReturnRows(pgxmock.NewRows([]string{"id", "keyword", "normalized_keyword", "created_at"}).
				AddRow(keywordID, "Machine Learning", "machine learning", now))

		result, err := repo.GetOrCreate(ctx, "machine learning")
		require.NoError(t, err)
		assert.Equal(t, keywordID, result.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rejects empty keyword", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		_, err = repo.GetOrCreate(ctx, "   ")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrInvalidInput))
	})
}

func TestPgKeywordRepository_GetByID(t *testing.T) {
	t.Run("returns keyword when found", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		now := time.Now().UTC()
		mock.ExpectQuery(`SELECT id, keyword, normalized_keyword, created_at FROM keywords WHERE id = \$1`).
			WithArgs(keywordID).
			WillReturnRows(pgxmock.NewRows([]string{"id", "keyword", "normalized_keyword", "created_at"}).
				AddRow(keywordID, "Machine Learning", "machine learning", now))

		result, err := repo.GetByID(ctx, keywordID)
		require.NoError(t, err)
		assert.Equal(t, keywordID, result.ID)
		assert.Equal(t, "Machine Learning", result.Keyword)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns not found error", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		mock.ExpectQuery(`SELECT id, keyword, normalized_keyword, created_at FROM keywords WHERE id = \$1`).
			WithArgs(keywordID).
			WillReturnError(pgx.ErrNoRows)

		_, err = repo.GetByID(ctx, keywordID)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrNotFound))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgKeywordRepository_GetByNormalized(t *testing.T) {
	t.Run("returns keyword when found", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		now := time.Now().UTC()
		mock.ExpectQuery(`SELECT id, keyword, normalized_keyword, created_at FROM keywords WHERE normalized_keyword = \$1`).
			WithArgs("deep learning").
			WillReturnRows(pgxmock.NewRows([]string{"id", "keyword", "normalized_keyword", "created_at"}).
				AddRow(keywordID, "Deep Learning", "deep learning", now))

		result, err := repo.GetByNormalized(ctx, "deep learning")
		require.NoError(t, err)
		assert.Equal(t, keywordID, result.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rejects empty normalized", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		_, err = repo.GetByNormalized(ctx, "")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrInvalidInput))
	})
}

func TestPgKeywordRepository_BulkGetOrCreate(t *testing.T) {
	t.Run("creates multiple keywords", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywords := []string{"Machine Learning", "Deep Learning", "Neural Networks"}

		mock.ExpectQuery(`INSERT INTO keywords`).
			WithArgs(
				pgxmock.AnyArg(), "Machine Learning", "machine learning", pgxmock.AnyArg(),
				pgxmock.AnyArg(), "Deep Learning", "deep learning", pgxmock.AnyArg(),
				pgxmock.AnyArg(), "Neural Networks", "neural networks", pgxmock.AnyArg(),
			).
			WillReturnRows(pgxmock.NewRows([]string{"id", "keyword", "normalized_keyword", "created_at"}).
				AddRow(uuid.New(), "Machine Learning", "machine learning", time.Now()).
				AddRow(uuid.New(), "Deep Learning", "deep learning", time.Now()).
				AddRow(uuid.New(), "Neural Networks", "neural networks", time.Now()))

		results, err := repo.BulkGetOrCreate(ctx, keywords)
		require.NoError(t, err)
		assert.Len(t, results, 3)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles empty input", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		results, err := repo.BulkGetOrCreate(ctx, []string{})
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("deduplicates by normalized form", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		// Different casing should result in only one keyword
		keywords := []string{"Machine Learning", "MACHINE LEARNING", "machine learning"}

		mock.ExpectQuery(`INSERT INTO keywords`).
			WithArgs(
				pgxmock.AnyArg(), "Machine Learning", "machine learning", pgxmock.AnyArg(),
			).
			WillReturnRows(pgxmock.NewRows([]string{"id", "keyword", "normalized_keyword", "created_at"}).
				AddRow(uuid.New(), "Machine Learning", "machine learning", time.Now()))

		results, err := repo.BulkGetOrCreate(ctx, keywords)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("skips empty keywords", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywords := []string{"", "   ", "Machine Learning"}

		mock.ExpectQuery(`INSERT INTO keywords`).
			WithArgs(
				pgxmock.AnyArg(), "Machine Learning", "machine learning", pgxmock.AnyArg(),
			).
			WillReturnRows(pgxmock.NewRows([]string{"id", "keyword", "normalized_keyword", "created_at"}).
				AddRow(uuid.New(), "Machine Learning", "machine learning", time.Now()))

		results, err := repo.BulkGetOrCreate(ctx, keywords)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgKeywordRepository_RecordSearch(t *testing.T) {
	t.Run("records new search", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		search := &domain.KeywordSearch{
			KeywordID:        keywordID,
			SourceAPI:        domain.SourceTypeSemanticScholar,
			SearchWindowHash: "hash123",
			PapersFound:      42,
			Status:           domain.SearchStatusCompleted,
		}

		// Use AnyArg for SearchedAt since it's set automatically if zero
		mock.ExpectExec(`INSERT INTO keyword_searches`).
			WithArgs(
				pgxmock.AnyArg(), keywordID, domain.SourceTypeSemanticScholar, pgxmock.AnyArg(),
				pgxmock.AnyArg(), pgxmock.AnyArg(), "hash123", 42, domain.SearchStatusCompleted, "",
			).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		err = repo.RecordSearch(ctx, search)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rejects nil search", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		err = repo.RecordSearch(ctx, nil)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrInvalidInput))
	})

	t.Run("returns not found for missing keyword", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		search := &domain.KeywordSearch{
			KeywordID:        keywordID,
			SourceAPI:        domain.SourceTypeSemanticScholar,
			SearchWindowHash: "hash123",
			Status:           domain.SearchStatusCompleted,
		}

		pgErr := &pgconn.PgError{Code: "23503"}
		mock.ExpectExec(`INSERT INTO keyword_searches`).
			WithArgs(
				pgxmock.AnyArg(), keywordID, domain.SourceTypeSemanticScholar, pgxmock.AnyArg(),
				pgxmock.AnyArg(), pgxmock.AnyArg(), "hash123", 0, domain.SearchStatusCompleted, "",
			).
			WillReturnError(pgErr)

		err = repo.RecordSearch(ctx, search)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrNotFound))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgKeywordRepository_GetLastSearch(t *testing.T) {
	t.Run("returns last search", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		searchID := uuid.New()
		now := time.Now().UTC()

		mock.ExpectQuery(`SELECT id, keyword_id, source_api, searched_at, date_from, date_to`).
			WithArgs(keywordID, domain.SourceTypeSemanticScholar).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "keyword_id", "source_api", "searched_at", "date_from", "date_to",
				"search_window_hash", "papers_found", "status", "error_message",
			}).AddRow(searchID, keywordID, domain.SourceTypeSemanticScholar, now, nil, nil, "hash123", 42, domain.SearchStatusCompleted, ""))

		result, err := repo.GetLastSearch(ctx, keywordID, domain.SourceTypeSemanticScholar)
		require.NoError(t, err)
		assert.Equal(t, searchID, result.ID)
		assert.Equal(t, 42, result.PapersFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns not found when no search exists", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		mock.ExpectQuery(`SELECT id, keyword_id, source_api, searched_at, date_from, date_to`).
			WithArgs(keywordID, domain.SourceTypeSemanticScholar).
			WillReturnError(pgx.ErrNoRows)

		_, err = repo.GetLastSearch(ctx, keywordID, domain.SourceTypeSemanticScholar)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrNotFound))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgKeywordRepository_NeedsSearch(t *testing.T) {
	t.Run("returns true when never searched", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		mock.ExpectQuery(`SELECT id, keyword_id, source_api, searched_at, date_from, date_to`).
			WithArgs(keywordID, domain.SourceTypeSemanticScholar).
			WillReturnError(pgx.ErrNoRows)

		needs, err := repo.NeedsSearch(ctx, keywordID, domain.SourceTypeSemanticScholar, 24*time.Hour)
		require.NoError(t, err)
		assert.True(t, needs)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns true when last search failed", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		searchID := uuid.New()
		now := time.Now().UTC()

		mock.ExpectQuery(`SELECT id, keyword_id, source_api, searched_at, date_from, date_to`).
			WithArgs(keywordID, domain.SourceTypeSemanticScholar).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "keyword_id", "source_api", "searched_at", "date_from", "date_to",
				"search_window_hash", "papers_found", "status", "error_message",
			}).AddRow(searchID, keywordID, domain.SourceTypeSemanticScholar, now, nil, nil, "hash123", 0, domain.SearchStatusFailed, "API error"))

		needs, err := repo.NeedsSearch(ctx, keywordID, domain.SourceTypeSemanticScholar, 24*time.Hour)
		require.NoError(t, err)
		assert.True(t, needs)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns true when search is too old", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		searchID := uuid.New()
		oldTime := time.Now().UTC().Add(-48 * time.Hour) // 48 hours ago

		mock.ExpectQuery(`SELECT id, keyword_id, source_api, searched_at, date_from, date_to`).
			WithArgs(keywordID, domain.SourceTypeSemanticScholar).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "keyword_id", "source_api", "searched_at", "date_from", "date_to",
				"search_window_hash", "papers_found", "status", "error_message",
			}).AddRow(searchID, keywordID, domain.SourceTypeSemanticScholar, oldTime, nil, nil, "hash123", 42, domain.SearchStatusCompleted, ""))

		needs, err := repo.NeedsSearch(ctx, keywordID, domain.SourceTypeSemanticScholar, 24*time.Hour)
		require.NoError(t, err)
		assert.True(t, needs)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns false when recent successful search exists", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		searchID := uuid.New()
		recentTime := time.Now().UTC().Add(-1 * time.Hour) // 1 hour ago

		mock.ExpectQuery(`SELECT id, keyword_id, source_api, searched_at, date_from, date_to`).
			WithArgs(keywordID, domain.SourceTypeSemanticScholar).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "keyword_id", "source_api", "searched_at", "date_from", "date_to",
				"search_window_hash", "papers_found", "status", "error_message",
			}).AddRow(searchID, keywordID, domain.SourceTypeSemanticScholar, recentTime, nil, nil, "hash123", 42, domain.SearchStatusCompleted, ""))

		needs, err := repo.NeedsSearch(ctx, keywordID, domain.SourceTypeSemanticScholar, 24*time.Hour)
		require.NoError(t, err)
		assert.False(t, needs)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgKeywordRepository_ListSearches(t *testing.T) {
	t.Run("returns searches with filters", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		searchID := uuid.New()
		now := time.Now().UTC()

		filter := SearchFilter{
			KeywordID: &keywordID,
			SourceAPI: ptrSourceType(domain.SourceTypeSemanticScholar),
			Status:    ptrSearchStatus(domain.SearchStatusCompleted),
			Limit:     10,
		}

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM keyword_searches`).
			WithArgs(keywordID, domain.SourceTypeSemanticScholar, domain.SearchStatusCompleted).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))

		mock.ExpectQuery(`SELECT id, keyword_id, source_api, searched_at, date_from, date_to`).
			WithArgs(keywordID, domain.SourceTypeSemanticScholar, domain.SearchStatusCompleted, 10, 0).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "keyword_id", "source_api", "searched_at", "date_from", "date_to",
				"search_window_hash", "papers_found", "status", "error_message",
			}).AddRow(searchID, keywordID, domain.SourceTypeSemanticScholar, now, nil, nil, "hash123", 42, domain.SearchStatusCompleted, ""))

		results, total, err := repo.ListSearches(ctx, filter)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, int64(1), total)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns empty list when no matches", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		filter := SearchFilter{Limit: 10}

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM keyword_searches`).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		mock.ExpectQuery(`SELECT id, keyword_id, source_api, searched_at, date_from, date_to`).
			WithArgs(10, 0).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "keyword_id", "source_api", "searched_at", "date_from", "date_to",
				"search_window_hash", "papers_found", "status", "error_message",
			}))

		results, total, err := repo.ListSearches(ctx, filter)
		require.NoError(t, err)
		assert.Empty(t, results)
		assert.Equal(t, int64(0), total)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgKeywordRepository_AddPaperMapping(t *testing.T) {
	t.Run("adds new mapping", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		paperID := uuid.New()
		confidence := 0.95

		mapping := &domain.KeywordPaperMapping{
			KeywordID:       keywordID,
			PaperID:         paperID,
			MappingType:     domain.MappingTypeExtracted,
			SourceType:      domain.SourceTypeSemanticScholar,
			ConfidenceScore: &confidence,
		}

		mock.ExpectExec(`INSERT INTO keyword_paper_mappings`).
			WithArgs(
				pgxmock.AnyArg(), keywordID, paperID, domain.MappingTypeExtracted,
				domain.SourceTypeSemanticScholar, &confidence, pgxmock.AnyArg(),
			).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		err = repo.AddPaperMapping(ctx, mapping)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rejects nil mapping", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		err = repo.AddPaperMapping(ctx, nil)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrInvalidInput))
	})

	t.Run("handles duplicate mapping gracefully", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		paperID := uuid.New()

		mapping := &domain.KeywordPaperMapping{
			KeywordID:   keywordID,
			PaperID:     paperID,
			MappingType: domain.MappingTypeExtracted,
			SourceType:  domain.SourceTypeSemanticScholar,
		}

		// ON CONFLICT DO NOTHING means no rows affected, but no error
		mock.ExpectExec(`INSERT INTO keyword_paper_mappings`).
			WithArgs(
				pgxmock.AnyArg(), keywordID, paperID, domain.MappingTypeExtracted,
				domain.SourceTypeSemanticScholar, (*float64)(nil), pgxmock.AnyArg(),
			).
			WillReturnResult(pgxmock.NewResult("INSERT", 0))

		err = repo.AddPaperMapping(ctx, mapping)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns not found for missing keyword or paper", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		paperID := uuid.New()

		mapping := &domain.KeywordPaperMapping{
			KeywordID:   keywordID,
			PaperID:     paperID,
			MappingType: domain.MappingTypeExtracted,
			SourceType:  domain.SourceTypeSemanticScholar,
		}

		pgErr := &pgconn.PgError{Code: "23503"}
		mock.ExpectExec(`INSERT INTO keyword_paper_mappings`).
			WithArgs(
				pgxmock.AnyArg(), keywordID, paperID, domain.MappingTypeExtracted,
				domain.SourceTypeSemanticScholar, (*float64)(nil), pgxmock.AnyArg(),
			).
			WillReturnError(pgErr)

		err = repo.AddPaperMapping(ctx, mapping)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrNotFound))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgKeywordRepository_BulkAddPaperMappings(t *testing.T) {
	t.Run("adds multiple mappings", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID1 := uuid.New()
		keywordID2 := uuid.New()
		paperID := uuid.New()

		mappings := []*domain.KeywordPaperMapping{
			{
				KeywordID:   keywordID1,
				PaperID:     paperID,
				MappingType: domain.MappingTypeExtracted,
				SourceType:  domain.SourceTypeSemanticScholar,
			},
			{
				KeywordID:   keywordID2,
				PaperID:     paperID,
				MappingType: domain.MappingTypeAuthorKeyword,
				SourceType:  domain.SourceTypeSemanticScholar,
			},
		}

		mock.ExpectExec(`INSERT INTO keyword_paper_mappings`).
			WithArgs(
				pgxmock.AnyArg(), keywordID1, paperID, domain.MappingTypeExtracted,
				domain.SourceTypeSemanticScholar, (*float64)(nil), pgxmock.AnyArg(),
				pgxmock.AnyArg(), keywordID2, paperID, domain.MappingTypeAuthorKeyword,
				domain.SourceTypeSemanticScholar, (*float64)(nil), pgxmock.AnyArg(),
			).
			WillReturnResult(pgxmock.NewResult("INSERT", 2))

		err = repo.BulkAddPaperMappings(ctx, mappings)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles empty input", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		err = repo.BulkAddPaperMappings(ctx, []*domain.KeywordPaperMapping{})
		require.NoError(t, err)
	})

	t.Run("rejects nil mapping in slice", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		mappings := []*domain.KeywordPaperMapping{nil}

		err = repo.BulkAddPaperMappings(ctx, mappings)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrInvalidInput))
	})
}

func TestPgKeywordRepository_GetPapersForKeyword(t *testing.T) {
	t.Run("returns papers for keyword", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		paperID := uuid.New()
		now := time.Now().UTC()

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM keyword_paper_mappings WHERE keyword_id = \$1`).
			WithArgs(keywordID).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))

		mock.ExpectQuery(`SELECT p.id, p.canonical_id, p.title, p.abstract, p.authors`).
			WithArgs(keywordID, 10, 0).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "canonical_id", "title", "abstract", "authors",
				"publication_date", "publication_year", "venue", "journal",
				"volume", "issue", "pages", "citation_count", "reference_count",
				"pdf_url", "open_access", "keywords_extracted", "raw_metadata",
				"created_at", "updated_at",
			}).AddRow(
				paperID, "doi:10.1234/test", "Test Paper", "Abstract", []byte(`[{"name":"Author"}]`),
				nil, 2024, "Venue", "Journal",
				nil, nil, nil, 10, 5,
				nil, false, false, nil,
				now, now,
			))

		papers, total, err := repo.GetPapersForKeyword(ctx, keywordID, 10, 0)
		require.NoError(t, err)
		assert.Len(t, papers, 1)
		assert.Equal(t, int64(1), total)
		assert.Equal(t, paperID, papers[0].ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("applies default limits", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM keyword_paper_mappings WHERE keyword_id = \$1`).
			WithArgs(keywordID).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		mock.ExpectQuery(`SELECT p.id, p.canonical_id, p.title, p.abstract, p.authors`).
			WithArgs(keywordID, 100, 0). // Default limit
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "canonical_id", "title", "abstract", "authors",
				"publication_date", "publication_year", "venue", "journal",
				"volume", "issue", "pages", "citation_count", "reference_count",
				"pdf_url", "open_access", "keywords_extracted", "raw_metadata",
				"created_at", "updated_at",
			}))

		_, _, err = repo.GetPapersForKeyword(ctx, keywordID, 0, -1)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgKeywordRepository_List(t *testing.T) {
	t.Run("returns keywords with substring filter", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		now := time.Now().UTC()

		filter := KeywordFilter{
			NormalizedContains: "learning",
			Limit:              10,
		}

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM keywords k`).
			WithArgs("%learning%").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))

		mock.ExpectQuery(`SELECT k.id, k.keyword, k.normalized_keyword, k.created_at`).
			WithArgs("%learning%", 10, 0).
			WillReturnRows(pgxmock.NewRows([]string{"id", "keyword", "normalized_keyword", "created_at"}).
				AddRow(keywordID, "Machine Learning", "machine learning", now))

		results, total, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, int64(1), total)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns keywords with HasSearchInSource filter", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		keywordID := uuid.New()
		now := time.Now().UTC()

		filter := KeywordFilter{
			HasSearchInSource: ptrSourceType(domain.SourceTypeSemanticScholar),
			Limit:             10,
		}

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM keywords k WHERE EXISTS`).
			WithArgs(domain.SourceTypeSemanticScholar).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))

		mock.ExpectQuery(`SELECT k.id, k.keyword, k.normalized_keyword, k.created_at FROM keywords k WHERE EXISTS`).
			WithArgs(domain.SourceTypeSemanticScholar, 10, 0).
			WillReturnRows(pgxmock.NewRows([]string{"id", "keyword", "normalized_keyword", "created_at"}).
				AddRow(keywordID, "Machine Learning", "machine learning", now))

		results, total, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, int64(1), total)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns empty list when no matches", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		filter := KeywordFilter{Limit: 10}

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM keywords k`).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		mock.ExpectQuery(`SELECT k.id, k.keyword, k.normalized_keyword, k.created_at FROM keywords k`).
			WithArgs(10, 0).
			WillReturnRows(pgxmock.NewRows([]string{"id", "keyword", "normalized_keyword", "created_at"}))

		results, total, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Empty(t, results)
		assert.Equal(t, int64(0), total)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgKeywordRepository_InterfaceCompliance(t *testing.T) {
	// Ensure PgKeywordRepository implements KeywordRepository
	var _ KeywordRepository = (*PgKeywordRepository)(nil)
}

func TestPgKeywordRepository_ErrorHandling(t *testing.T) {
	t.Run("handles database errors gracefully", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgKeywordRepository(mock)
		ctx := context.Background()

		dbErr := errors.New("connection reset")
		mock.ExpectQuery(`SELECT id, keyword, normalized_keyword, created_at FROM keywords WHERE id = \$1`).
			WithArgs(pgxmock.AnyArg()).
			WillReturnError(dbErr)

		_, err = repo.GetByID(ctx, uuid.New())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connection reset")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// Helper functions for creating pointers
func ptrSourceType(s domain.SourceType) *domain.SourceType {
	return &s
}

func ptrSearchStatus(s domain.SearchStatus) *domain.SearchStatus {
	return &s
}
