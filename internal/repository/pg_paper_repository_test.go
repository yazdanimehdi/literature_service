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

// Helper to create a valid paper for testing.
func newTestPaper() *domain.Paper {
	now := time.Now().UTC()
	return &domain.Paper{
		ID:          uuid.New(),
		CanonicalID: "doi:10.1234/test.paper",
		Title:       "Test Paper Title",
		Abstract:    "This is a test abstract for the paper.",
		Authors: []domain.Author{
			{Name: "John Doe", Affiliation: "Test University", ORCID: "0000-0001-2345-6789"},
			{Name: "Jane Smith", Affiliation: "Research Institute"},
		},
		PublicationYear: 2024,
		Venue:           "Test Conference",
		Journal:         "Test Journal",
		CitationCount:   10,
		ReferenceCount:  25,
		PDFURL:          "https://example.com/paper.pdf",
		OpenAccess:      true,
		RawMetadata: map[string]interface{}{
			"source": "semantic_scholar",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestNewPgPaperRepository(t *testing.T) {
	t.Run("creates repository with nil db", func(t *testing.T) {
		repo := NewPgPaperRepository(nil)
		assert.NotNil(t, repo)
		assert.Nil(t, repo.db)
	})

	t.Run("creates repository with mock db", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		assert.NotNil(t, repo)
		assert.NotNil(t, repo.db)
	})
}

func TestPgPaperRepository_Create(t *testing.T) {
	ctx := context.Background()

	t.Run("creates paper successfully", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paper := newTestPaper()

		mock.ExpectQuery("INSERT INTO papers").
			WithArgs(
				pgxmock.AnyArg(), paper.CanonicalID, paper.Title, paper.Abstract, pgxmock.AnyArg(),
				pgxmock.AnyArg(), paper.PublicationYear, paper.Venue, paper.Journal,
				paper.Volume, paper.Issue, paper.Pages, paper.CitationCount, paper.ReferenceCount,
				paper.PDFURL, paper.OpenAccess, paper.KeywordsExtracted, pgxmock.AnyArg(),
				pgxmock.AnyArg(), pgxmock.AnyArg(),
			).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).
				AddRow(paper.ID, paper.CreatedAt, paper.UpdatedAt))

		result, err := repo.Create(ctx, paper)
		require.NoError(t, err)
		assert.Equal(t, paper.ID, result.ID)
		assert.Equal(t, paper.CanonicalID, result.CanonicalID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns validation error for nil paper", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		result, err := repo.Create(ctx, nil)

		assert.Nil(t, result)
		var validationErr *domain.ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, "paper", validationErr.Field)
	})

	t.Run("returns validation error for missing canonical_id", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paper := newTestPaper()
		paper.CanonicalID = ""

		result, err := repo.Create(ctx, paper)

		assert.Nil(t, result)
		var validationErr *domain.ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, "canonical_id", validationErr.Field)
	})
}

func TestPgPaperRepository_GetByCanonicalID(t *testing.T) {
	ctx := context.Background()

	t.Run("returns paper when found", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paper := newTestPaper()

		authorsJSON, _ := json.Marshal(paper.Authors)
		metadataJSON, _ := json.Marshal(paper.RawMetadata)

		rows := pgxmock.NewRows([]string{
			"id", "canonical_id", "title", "abstract", "authors",
			"publication_date", "publication_year", "venue", "journal",
			"volume", "issue", "pages", "citation_count", "reference_count",
			"pdf_url", "open_access", "keywords_extracted", "raw_metadata",
			"created_at", "updated_at",
		}).AddRow(
			paper.ID, paper.CanonicalID, paper.Title, paper.Abstract, authorsJSON,
			paper.PublicationDate, paper.PublicationYear, paper.Venue, paper.Journal,
			paper.Volume, paper.Issue, paper.Pages, paper.CitationCount, paper.ReferenceCount,
			paper.PDFURL, paper.OpenAccess, paper.KeywordsExtracted, metadataJSON,
			paper.CreatedAt, paper.UpdatedAt,
		)

		mock.ExpectQuery("SELECT .* FROM papers WHERE canonical_id = \\$1").
			WithArgs(paper.CanonicalID).
			WillReturnRows(rows)

		result, err := repo.GetByCanonicalID(ctx, paper.CanonicalID)
		require.NoError(t, err)
		assert.Equal(t, paper.ID, result.ID)
		assert.Equal(t, paper.CanonicalID, result.CanonicalID)
		assert.Equal(t, paper.Title, result.Title)
		assert.Len(t, result.Authors, 2)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns validation error for empty canonical_id", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		result, err := repo.GetByCanonicalID(ctx, "")

		assert.Nil(t, result)
		var validationErr *domain.ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, "canonical_id", validationErr.Field)
	})

	t.Run("returns not found error when not exists", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)

		mock.ExpectQuery("SELECT .* FROM papers WHERE canonical_id = \\$1").
			WithArgs("nonexistent").
			WillReturnError(pgx.ErrNoRows)

		result, err := repo.GetByCanonicalID(ctx, "nonexistent")
		assert.Nil(t, result)
		assert.True(t, errors.Is(err, domain.ErrNotFound))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgPaperRepository_GetByID(t *testing.T) {
	ctx := context.Background()

	t.Run("returns paper when found", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paper := newTestPaper()

		authorsJSON, _ := json.Marshal(paper.Authors)
		metadataJSON, _ := json.Marshal(paper.RawMetadata)

		rows := pgxmock.NewRows([]string{
			"id", "canonical_id", "title", "abstract", "authors",
			"publication_date", "publication_year", "venue", "journal",
			"volume", "issue", "pages", "citation_count", "reference_count",
			"pdf_url", "open_access", "keywords_extracted", "raw_metadata",
			"created_at", "updated_at",
		}).AddRow(
			paper.ID, paper.CanonicalID, paper.Title, paper.Abstract, authorsJSON,
			paper.PublicationDate, paper.PublicationYear, paper.Venue, paper.Journal,
			paper.Volume, paper.Issue, paper.Pages, paper.CitationCount, paper.ReferenceCount,
			paper.PDFURL, paper.OpenAccess, paper.KeywordsExtracted, metadataJSON,
			paper.CreatedAt, paper.UpdatedAt,
		)

		mock.ExpectQuery("SELECT .* FROM papers WHERE id = \\$1").
			WithArgs(paper.ID).
			WillReturnRows(rows)

		result, err := repo.GetByID(ctx, paper.ID)
		require.NoError(t, err)
		assert.Equal(t, paper.ID, result.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns not found error when not exists", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		id := uuid.New()

		mock.ExpectQuery("SELECT .* FROM papers WHERE id = \\$1").
			WithArgs(id).
			WillReturnError(pgx.ErrNoRows)

		result, err := repo.GetByID(ctx, id)
		assert.Nil(t, result)
		assert.True(t, errors.Is(err, domain.ErrNotFound))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgPaperRepository_FindByIdentifier(t *testing.T) {
	ctx := context.Background()

	t.Run("returns validation error for empty value", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		result, err := repo.FindByIdentifier(ctx, domain.IdentifierTypeDOI, "")

		assert.Nil(t, result)
		var validationErr *domain.ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, "value", validationErr.Field)
	})

	t.Run("returns paper when found", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paper := newTestPaper()

		authorsJSON, _ := json.Marshal(paper.Authors)
		metadataJSON, _ := json.Marshal(paper.RawMetadata)

		rows := pgxmock.NewRows([]string{
			"id", "canonical_id", "title", "abstract", "authors",
			"publication_date", "publication_year", "venue", "journal",
			"volume", "issue", "pages", "citation_count", "reference_count",
			"pdf_url", "open_access", "keywords_extracted", "raw_metadata",
			"created_at", "updated_at",
		}).AddRow(
			paper.ID, paper.CanonicalID, paper.Title, paper.Abstract, authorsJSON,
			paper.PublicationDate, paper.PublicationYear, paper.Venue, paper.Journal,
			paper.Volume, paper.Issue, paper.Pages, paper.CitationCount, paper.ReferenceCount,
			paper.PDFURL, paper.OpenAccess, paper.KeywordsExtracted, metadataJSON,
			paper.CreatedAt, paper.UpdatedAt,
		)

		mock.ExpectQuery("SELECT .* FROM papers p INNER JOIN paper_identifiers pi").
			WithArgs(domain.IdentifierTypeDOI, "10.1234/test").
			WillReturnRows(rows)

		result, err := repo.FindByIdentifier(ctx, domain.IdentifierTypeDOI, "10.1234/test")
		require.NoError(t, err)
		assert.Equal(t, paper.ID, result.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns not found when not exists", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)

		mock.ExpectQuery("SELECT .* FROM papers p INNER JOIN paper_identifiers pi").
			WithArgs(domain.IdentifierTypeDOI, "nonexistent").
			WillReturnError(pgx.ErrNoRows)

		result, err := repo.FindByIdentifier(ctx, domain.IdentifierTypeDOI, "nonexistent")
		assert.Nil(t, result)
		assert.True(t, errors.Is(err, domain.ErrNotFound))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgPaperRepository_UpsertIdentifier(t *testing.T) {
	ctx := context.Background()

	t.Run("returns validation error for empty value", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		err = repo.UpsertIdentifier(ctx, uuid.New(), domain.IdentifierTypeDOI, "", domain.SourceTypeSemanticScholar)

		var validationErr *domain.ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, "value", validationErr.Field)
	})

	t.Run("upserts identifier successfully", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paperID := uuid.New()

		mock.ExpectExec("INSERT INTO paper_identifiers").
			WithArgs(paperID, domain.IdentifierTypeDOI, "10.1234/test", domain.SourceTypeSemanticScholar, pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		err = repo.UpsertIdentifier(ctx, paperID, domain.IdentifierTypeDOI, "10.1234/test", domain.SourceTypeSemanticScholar)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns not found error for foreign key violation", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paperID := uuid.New()

		pgErr := &pgconn.PgError{Code: "23503"} // Foreign key violation
		mock.ExpectExec("INSERT INTO paper_identifiers").
			WithArgs(paperID, domain.IdentifierTypeDOI, "10.1234/test", domain.SourceTypeSemanticScholar, pgxmock.AnyArg()).
			WillReturnError(pgErr)

		err = repo.UpsertIdentifier(ctx, paperID, domain.IdentifierTypeDOI, "10.1234/test", domain.SourceTypeSemanticScholar)
		assert.True(t, errors.Is(err, domain.ErrNotFound))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns already exists error for unique constraint violation", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paperID := uuid.New()

		pgErr := &pgconn.PgError{Code: "23505"} // Unique constraint violation
		mock.ExpectExec("INSERT INTO paper_identifiers").
			WithArgs(paperID, domain.IdentifierTypeDOI, "10.1234/test", domain.SourceTypeSemanticScholar, pgxmock.AnyArg()).
			WillReturnError(pgErr)

		err = repo.UpsertIdentifier(ctx, paperID, domain.IdentifierTypeDOI, "10.1234/test", domain.SourceTypeSemanticScholar)
		assert.True(t, errors.Is(err, domain.ErrAlreadyExists))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgPaperRepository_AddSource(t *testing.T) {
	ctx := context.Background()

	t.Run("adds source successfully", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paperID := uuid.New()
		metadata := map[string]interface{}{"key": "value"}

		mock.ExpectExec("INSERT INTO paper_sources").
			WithArgs(paperID, domain.SourceTypeSemanticScholar, pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		err = repo.AddSource(ctx, paperID, domain.SourceTypeSemanticScholar, metadata)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("adds source with nil metadata", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paperID := uuid.New()

		// nil metadata marshals to nil []byte
		mock.ExpectExec("INSERT INTO paper_sources").
			WithArgs(paperID, domain.SourceTypeSemanticScholar, pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		err = repo.AddSource(ctx, paperID, domain.SourceTypeSemanticScholar, nil)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns not found error for foreign key violation", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paperID := uuid.New()

		pgErr := &pgconn.PgError{Code: "23503"} // Foreign key violation
		mock.ExpectExec("INSERT INTO paper_sources").
			WithArgs(paperID, domain.SourceTypeSemanticScholar, pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnError(pgErr)

		err = repo.AddSource(ctx, paperID, domain.SourceTypeSemanticScholar, nil)
		assert.True(t, errors.Is(err, domain.ErrNotFound))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgPaperRepository_List(t *testing.T) {
	ctx := context.Background()

	t.Run("lists papers with no filters", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paper := newTestPaper()

		authorsJSON, _ := json.Marshal(paper.Authors)
		metadataJSON, _ := json.Marshal(paper.RawMetadata)

		// Expect count query
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM papers").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))

		// Expect select query - no WHERE clause when no filters
		rows := pgxmock.NewRows([]string{
			"id", "canonical_id", "title", "abstract", "authors",
			"publication_date", "publication_year", "venue", "journal",
			"volume", "issue", "pages", "citation_count", "reference_count",
			"pdf_url", "open_access", "keywords_extracted", "raw_metadata",
			"created_at", "updated_at",
		}).AddRow(
			paper.ID, paper.CanonicalID, paper.Title, paper.Abstract, authorsJSON,
			paper.PublicationDate, paper.PublicationYear, paper.Venue, paper.Journal,
			paper.Volume, paper.Issue, paper.Pages, paper.CitationCount, paper.ReferenceCount,
			paper.PDFURL, paper.OpenAccess, paper.KeywordsExtracted, metadataJSON,
			paper.CreatedAt, paper.UpdatedAt,
		)

		mock.ExpectQuery("SELECT .* FROM papers p\\s+ORDER BY p.created_at DESC LIMIT \\$1 OFFSET \\$2").
			WithArgs(100, 0).
			WillReturnRows(rows)

		filter := PaperFilter{
			Limit:  100,
			Offset: 0,
		}

		results, count, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, int64(1), count)
		assert.Equal(t, paper.ID, results[0].ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("lists papers with HasPDF filter true", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)

		// Expect count query with HasPDF filter
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM papers p WHERE p.pdf_url IS NOT NULL").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		// Expect select query
		mock.ExpectQuery("SELECT .* FROM papers p WHERE p.pdf_url IS NOT NULL").
			WithArgs(100, 0).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "canonical_id", "title", "abstract", "authors",
				"publication_date", "publication_year", "venue", "journal",
				"volume", "issue", "pages", "citation_count", "reference_count",
				"pdf_url", "open_access", "keywords_extracted", "raw_metadata",
				"created_at", "updated_at",
			}))

		hasPDF := true
		filter := PaperFilter{
			HasPDF: &hasPDF,
			Limit:  100,
			Offset: 0,
		}

		results, count, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Len(t, results, 0)
		assert.Equal(t, int64(0), count)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("applies default limit", func(t *testing.T) {
		filter := PaperFilter{
			Limit: 0, // Should default to 100
		}
		err := filter.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 100, filter.Limit)
	})

	t.Run("caps max limit", func(t *testing.T) {
		filter := PaperFilter{
			Limit: 5000, // Should be capped to 1000
		}
		err := filter.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 1000, filter.Limit)
	})
}

func TestPgPaperRepository_MarkKeywordsExtracted(t *testing.T) {
	ctx := context.Background()

	t.Run("marks keywords extracted successfully", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paperID := uuid.New()

		mock.ExpectExec("UPDATE papers SET keywords_extracted = true").
			WithArgs(pgxmock.AnyArg(), paperID).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		err = repo.MarkKeywordsExtracted(ctx, paperID)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns not found error when paper doesn't exist", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paperID := uuid.New()

		mock.ExpectExec("UPDATE papers SET keywords_extracted = true").
			WithArgs(pgxmock.AnyArg(), paperID).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		err = repo.MarkKeywordsExtracted(ctx, paperID)
		assert.True(t, errors.Is(err, domain.ErrNotFound))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPgPaperRepository_BulkUpsert(t *testing.T) {
	ctx := context.Background()

	t.Run("returns empty slice for empty input", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		results, err := repo.BulkUpsert(ctx, []*domain.Paper{})

		require.NoError(t, err)
		assert.Len(t, results, 0)
	})

	t.Run("returns validation error for nil paper in slice", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		papers := []*domain.Paper{newTestPaper(), nil}

		results, err := repo.BulkUpsert(ctx, papers)

		assert.Nil(t, results)
		var validationErr *domain.ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Contains(t, validationErr.Message, "index 1")
	})

	t.Run("returns validation error for paper without canonical_id", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paper := newTestPaper()
		paper.CanonicalID = ""
		papers := []*domain.Paper{paper}

		results, err := repo.BulkUpsert(ctx, papers)

		assert.Nil(t, results)
		var validationErr *domain.ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, "canonical_id", validationErr.Field)
	})

	t.Run("upserts multiple papers successfully", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := NewPgPaperRepository(mock)
		paper1 := newTestPaper()
		paper1.CanonicalID = "doi:10.1234/paper1"
		paper2 := newTestPaper()
		paper2.CanonicalID = "doi:10.1234/paper2"
		papers := []*domain.Paper{paper1, paper2}

		// BulkUpsert now uses pgx.Batch for a single network roundtrip
		expectedBatch := mock.ExpectBatch()
		for i, paper := range papers {
			expectedBatch.ExpectQuery("INSERT INTO papers").
				WithArgs(
					pgxmock.AnyArg(), paper.CanonicalID, paper.Title, paper.Abstract, pgxmock.AnyArg(),
					pgxmock.AnyArg(), paper.PublicationYear, paper.Venue, paper.Journal,
					paper.Volume, paper.Issue, paper.Pages, paper.CitationCount, paper.ReferenceCount,
					paper.PDFURL, paper.OpenAccess, paper.KeywordsExtracted, pgxmock.AnyArg(),
					pgxmock.AnyArg(), pgxmock.AnyArg(),
				).
				WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).
					AddRow(papers[i].ID, papers[i].CreatedAt, papers[i].UpdatedAt))
		}

		results, err := repo.BulkUpsert(ctx, papers)
		require.NoError(t, err)
		assert.Len(t, results, 2)
		assert.Equal(t, paper1.CanonicalID, results[0].CanonicalID)
		assert.Equal(t, paper2.CanonicalID, results[1].CanonicalID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPaperScanDest(t *testing.T) {
	t.Run("destinations returns correct number of pointers", func(t *testing.T) {
		var dest paperScanDest
		dests := dest.destinations()
		// Should have exactly 20 destination pointers matching the SELECT columns
		assert.Len(t, dests, 20)
	})

	t.Run("finalize handles authors JSON", func(t *testing.T) {
		dest := paperScanDest{
			paper: domain.Paper{
				ID:          uuid.New(),
				CanonicalID: "doi:test",
			},
			authorsJSON: []byte(`[{"name":"John Doe","affiliation":"Test Uni"}]`),
		}

		result, err := dest.finalize()
		require.NoError(t, err)
		assert.Len(t, result.Authors, 1)
		assert.Equal(t, "John Doe", result.Authors[0].Name)
		assert.Equal(t, "Test Uni", result.Authors[0].Affiliation)
	})

	t.Run("finalize handles metadata JSON", func(t *testing.T) {
		dest := paperScanDest{
			paper: domain.Paper{
				ID:          uuid.New(),
				CanonicalID: "doi:test",
			},
			metadataJSON: []byte(`{"source":"pubmed"}`),
		}

		result, err := dest.finalize()
		require.NoError(t, err)
		assert.Equal(t, "pubmed", result.RawMetadata["source"])
	})

	t.Run("finalize returns error for invalid authors JSON", func(t *testing.T) {
		dest := paperScanDest{
			authorsJSON: []byte(`{invalid json`),
		}

		result, err := dest.finalize()
		assert.Nil(t, result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal authors")
	})

	t.Run("finalize returns error for invalid metadata JSON", func(t *testing.T) {
		dest := paperScanDest{
			metadataJSON: []byte(`{invalid json`),
		}

		result, err := dest.finalize()
		assert.Nil(t, result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal metadata")
	})
}
