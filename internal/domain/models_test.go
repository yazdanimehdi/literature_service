// Package domain provides domain models and business logic for the Literature Review Service.
package domain

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyword_Normalize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase conversion",
			input:    "Machine Learning",
			expected: "machine learning",
		},
		{
			name:     "trim leading whitespace",
			input:    "  neural networks",
			expected: "neural networks",
		},
		{
			name:     "trim trailing whitespace",
			input:    "deep learning  ",
			expected: "deep learning",
		},
		{
			name:     "trim both ends",
			input:    "  protein folding  ",
			expected: "protein folding",
		},
		{
			name:     "collapse multiple spaces",
			input:    "gene   expression   analysis",
			expected: "gene expression analysis",
		},
		{
			name:     "collapse tabs",
			input:    "cancer\t\tresearch",
			expected: "cancer research",
		},
		{
			name:     "collapse newlines",
			input:    "drug\n\ndiscovery",
			expected: "drug discovery",
		},
		{
			name:     "mixed whitespace",
			input:    "  CRISPR \t  CAS9  \n  ",
			expected: "crispr cas9",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   \t\n  ",
			expected: "",
		},
		{
			name:     "single word",
			input:    "Genomics",
			expected: "genomics",
		},
		{
			name:     "unicode characters preserved",
			input:    "Müller cells",
			expected: "müller cells",
		},
		{
			name:     "hyphenated words",
			input:    "COVID-19",
			expected: "covid-19",
		},
		{
			name:     "special characters preserved",
			input:    "p53 tumor suppressor",
			expected: "p53 tumor suppressor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeKeyword(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewKeyword(t *testing.T) {
	t.Run("creates keyword with normalized form", func(t *testing.T) {
		kw := NewKeyword("  Machine Learning  ")

		assert.NotEqual(t, uuid.Nil, kw.ID)
		assert.Equal(t, "  Machine Learning  ", kw.Keyword)
		assert.Equal(t, "machine learning", kw.NormalizedKeyword)
		assert.False(t, kw.CreatedAt.IsZero())
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		kw1 := NewKeyword("test")
		kw2 := NewKeyword("test")

		assert.NotEqual(t, kw1.ID, kw2.ID)
	})
}

func TestPaper_GenerateCanonicalID(t *testing.T) {
	tests := []struct {
		name        string
		identifiers PaperIdentifiers
		expected    string
	}{
		{
			name: "DOI takes priority",
			identifiers: PaperIdentifiers{
				DOI:               "10.1038/nature12373",
				ArXivID:           "1234.5678",
				PubMedID:          "12345678",
				SemanticScholarID: "abc123",
				OpenAlexID:        "W123456",
				ScopusID:          "SCOPUS_ID:123",
			},
			expected: "doi:10.1038/nature12373",
		},
		{
			name: "ArXiv when no DOI",
			identifiers: PaperIdentifiers{
				ArXivID:           "2103.14030",
				PubMedID:          "33845678",
				SemanticScholarID: "def456",
				OpenAlexID:        "W789012",
				ScopusID:          "SCOPUS_ID:456",
			},
			expected: "arxiv:2103.14030",
		},
		{
			name: "PubMed when no DOI or ArXiv",
			identifiers: PaperIdentifiers{
				PubMedID:          "33845678",
				SemanticScholarID: "ghi789",
				OpenAlexID:        "W345678",
				ScopusID:          "SCOPUS_ID:789",
			},
			expected: "pubmed:33845678",
		},
		{
			name: "SemanticScholar when no DOI, ArXiv, or PubMed",
			identifiers: PaperIdentifiers{
				SemanticScholarID: "jkl012",
				OpenAlexID:        "W901234",
				ScopusID:          "SCOPUS_ID:012",
			},
			expected: "s2:jkl012",
		},
		{
			name: "OpenAlex when no higher priority IDs",
			identifiers: PaperIdentifiers{
				OpenAlexID: "W567890",
				ScopusID:   "SCOPUS_ID:345",
			},
			expected: "openalex:W567890",
		},
		{
			name: "Scopus when only ID available",
			identifiers: PaperIdentifiers{
				ScopusID: "SCOPUS_ID:678",
			},
			expected: "scopus:SCOPUS_ID:678",
		},
		{
			name:        "empty when no identifiers",
			identifiers: PaperIdentifiers{},
			expected:    "",
		},
		{
			name: "DOI normalized to lowercase",
			identifiers: PaperIdentifiers{
				DOI: "10.1038/NATURE12373",
			},
			expected: "doi:10.1038/nature12373",
		},
		{
			name: "ArXiv without version suffix",
			identifiers: PaperIdentifiers{
				ArXivID: "2103.14030v2",
			},
			expected: "arxiv:2103.14030v2",
		},
		{
			name: "skip empty DOI to ArXiv",
			identifiers: PaperIdentifiers{
				DOI:     "",
				ArXivID: "2103.14030",
			},
			expected: "arxiv:2103.14030",
		},
		{
			name: "whitespace-only DOI skipped",
			identifiers: PaperIdentifiers{
				DOI:      "   ",
				ArXivID:  "2103.14030",
				PubMedID: "12345678",
			},
			expected: "arxiv:2103.14030",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateCanonicalID(tt.identifiers)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestReviewStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status   ReviewStatus
		expected bool
	}{
		{ReviewStatusPending, false},
		{ReviewStatusExtractingKeywords, false},
		{ReviewStatusSearching, false},
		{ReviewStatusExpanding, false},
		{ReviewStatusIngesting, false},
		{ReviewStatusPaused, false},
		{ReviewStatusCompleted, true},
		{ReviewStatusPartial, true},
		{ReviewStatusFailed, true},
		{ReviewStatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.IsTerminal())
		})
	}
}

func TestSearchStatus_String(t *testing.T) {
	tests := []struct {
		status   SearchStatus
		expected string
	}{
		{SearchStatusPending, "pending"},
		{SearchStatusInProgress, "in_progress"},
		{SearchStatusCompleted, "completed"},
		{SearchStatusFailed, "failed"},
		{SearchStatusRateLimited, "rate_limited"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.status))
		})
	}
}

func TestIngestionStatus_String(t *testing.T) {
	tests := []struct {
		status   IngestionStatus
		expected string
	}{
		{IngestionStatusPending, "pending"},
		{IngestionStatusSubmitted, "submitted"},
		{IngestionStatusProcessing, "processing"},
		{IngestionStatusCompleted, "completed"},
		{IngestionStatusFailed, "failed"},
		{IngestionStatusSkipped, "skipped"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.status))
		})
	}
}

func TestMappingType_String(t *testing.T) {
	tests := []struct {
		mappingType MappingType
		expected    string
	}{
		{MappingTypeAuthorKeyword, "author_keyword"},
		{MappingTypeMeshTerm, "mesh_term"},
		{MappingTypeExtracted, "extracted"},
		{MappingTypeQueryMatch, "query_match"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.mappingType))
		})
	}
}

func TestSourceType_String(t *testing.T) {
	tests := []struct {
		sourceType SourceType
		expected   string
	}{
		{SourceTypeSemanticScholar, "semantic_scholar"},
		{SourceTypeOpenAlex, "openalex"},
		{SourceTypeScopus, "scopus"},
		{SourceTypePubMed, "pubmed"},
		{SourceTypeBioRxiv, "biorxiv"},
		{SourceTypeArXiv, "arxiv"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.sourceType))
		})
	}
}

func TestIdentifierType_String(t *testing.T) {
	tests := []struct {
		idType   IdentifierType
		expected string
	}{
		{IdentifierTypeDOI, "doi"},
		{IdentifierTypeArXivID, "arxiv_id"},
		{IdentifierTypePubMedID, "pubmed_id"},
		{IdentifierTypeSemanticScholarID, "semantic_scholar_id"},
		{IdentifierTypeOpenAlexID, "openalex_id"},
		{IdentifierTypeScopusID, "scopus_id"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.idType))
		})
	}
}

func TestLiteratureReviewRequest_Duration(t *testing.T) {
	t.Run("returns zero when not started", func(t *testing.T) {
		req := &LiteratureReviewRequest{
			StartedAt: nil,
		}
		assert.Equal(t, time.Duration(0), req.Duration())
	})

	t.Run("returns duration when completed", func(t *testing.T) {
		start := time.Now().Add(-5 * time.Minute)
		end := time.Now()
		req := &LiteratureReviewRequest{
			StartedAt:   &start,
			CompletedAt: &end,
		}
		dur := req.Duration()
		assert.True(t, dur >= 4*time.Minute && dur <= 6*time.Minute, "duration should be around 5 minutes")
	})

	t.Run("returns elapsed time when still running", func(t *testing.T) {
		start := time.Now().Add(-2 * time.Second)
		req := &LiteratureReviewRequest{
			StartedAt: &start,
		}
		dur := req.Duration()
		assert.True(t, dur >= 1*time.Second && dur <= 3*time.Second, "duration should be around 2 seconds")
	})
}

func TestLiteratureReviewRequest_IsActive(t *testing.T) {
	tests := []struct {
		name     string
		status   ReviewStatus
		expected bool
	}{
		{"pending is active", ReviewStatusPending, true},
		{"extracting_keywords is active", ReviewStatusExtractingKeywords, true},
		{"searching is active", ReviewStatusSearching, true},
		{"expanding is active", ReviewStatusExpanding, true},
		{"ingesting is active", ReviewStatusIngesting, true},
		{"completed is not active", ReviewStatusCompleted, false},
		{"partial is not active", ReviewStatusPartial, false},
		{"failed is not active", ReviewStatusFailed, false},
		{"cancelled is not active", ReviewStatusCancelled, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &LiteratureReviewRequest{Status: tt.status}
			assert.Equal(t, tt.expected, req.IsActive())
		})
	}
}

func TestAuthor_String(t *testing.T) {
	tests := []struct {
		name     string
		author   Author
		expected string
	}{
		{
			name: "name only",
			author: Author{
				Name: "Jane Doe",
			},
			expected: "Jane Doe",
		},
		{
			name: "name with affiliation",
			author: Author{
				Name:        "John Smith",
				Affiliation: "MIT",
			},
			expected: "John Smith (MIT)",
		},
		{
			name: "name with ORCID",
			author: Author{
				Name:  "Alice Johnson",
				ORCID: "0000-0001-2345-6789",
			},
			expected: "Alice Johnson [0000-0001-2345-6789]",
		},
		{
			name: "all fields",
			author: Author{
				Name:        "Bob Wilson",
				Affiliation: "Stanford University",
				ORCID:       "0000-0002-3456-7890",
			},
			expected: "Bob Wilson (Stanford University) [0000-0002-3456-7890]",
		},
		{
			name: "empty affiliation ignored",
			author: Author{
				Name:        "Carol Davis",
				Affiliation: "",
				ORCID:       "0000-0003-4567-8901",
			},
			expected: "Carol Davis [0000-0003-4567-8901]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.author.String())
		})
	}
}

func TestValidationError(t *testing.T) {
	t.Run("single field error", func(t *testing.T) {
		err := &ValidationError{
			Field:   "query",
			Message: "cannot be empty",
		}
		assert.Equal(t, "validation error: query: cannot be empty", err.Error())
	})
}

func TestNotFoundError(t *testing.T) {
	t.Run("error message", func(t *testing.T) {
		id := uuid.New()
		err := &NotFoundError{
			Entity: "paper",
			ID:     id.String(),
		}
		expected := "paper not found: " + id.String()
		assert.Equal(t, expected, err.Error())
	})

	t.Run("unwrap returns ErrNotFound", func(t *testing.T) {
		err := &NotFoundError{
			Entity: "review",
			ID:     "123",
		}
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func TestAlreadyExistsError(t *testing.T) {
	t.Run("error message", func(t *testing.T) {
		err := &AlreadyExistsError{
			Entity: "keyword",
			ID:     "machine learning",
		}
		assert.Equal(t, "keyword already exists: machine learning", err.Error())
	})

	t.Run("unwrap returns ErrAlreadyExists", func(t *testing.T) {
		err := &AlreadyExistsError{
			Entity: "paper",
			ID:     "doi:10.1234/test",
		}
		assert.ErrorIs(t, err, ErrAlreadyExists)
	})
}

func TestRateLimitError(t *testing.T) {
	t.Run("error message with retry after", func(t *testing.T) {
		retryAfter := 30 * time.Second
		err := &RateLimitError{
			Source:     "semantic_scholar",
			RetryAfter: retryAfter,
		}
		assert.Equal(t, "rate limited by semantic_scholar: retry after 30s", err.Error())
	})

	t.Run("error message without retry after", func(t *testing.T) {
		err := &RateLimitError{
			Source:     "openalex",
			RetryAfter: 0,
		}
		assert.Equal(t, "rate limited by openalex: retry after 0s", err.Error())
	})

	t.Run("unwrap returns ErrRateLimited", func(t *testing.T) {
		err := &RateLimitError{
			Source:     "pubmed",
			RetryAfter: time.Minute,
		}
		assert.ErrorIs(t, err, ErrRateLimited)
	})
}

func TestExternalAPIError(t *testing.T) {
	t.Run("error message", func(t *testing.T) {
		cause := assert.AnError
		err := &ExternalAPIError{
			Source:     "scopus",
			StatusCode: 500,
			Message:    "internal server error",
			Cause:      cause,
		}
		assert.Contains(t, err.Error(), "scopus API error")
		assert.Contains(t, err.Error(), "500")
		assert.Contains(t, err.Error(), "internal server error")
	})

	t.Run("unwrap returns cause", func(t *testing.T) {
		cause := assert.AnError
		err := &ExternalAPIError{
			Source:     "arxiv",
			StatusCode: 503,
			Message:    "service unavailable",
			Cause:      cause,
		}
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("unwrap returns ErrServiceUnavailable when no cause", func(t *testing.T) {
		err := &ExternalAPIError{
			Source:     "biorxiv",
			StatusCode: 404,
			Message:    "not found",
		}
		assert.Equal(t, ErrServiceUnavailable, err.Unwrap())
		assert.ErrorIs(t, err, ErrServiceUnavailable)
	})
}

func TestOutboxEvent(t *testing.T) {
	t.Run("NewOutboxEvent creates valid event", func(t *testing.T) {
		aggregateID := uuid.New().String()
		payload := ReviewStartedPayload{
			RequestID: uuid.New(),
			Query:     "test query",
		}

		event, err := NewOutboxEvent(EventTypeReviewStarted, aggregateID, "literature_review", payload)
		require.NoError(t, err)

		assert.NotEmpty(t, event.EventID)
		assert.Equal(t, EventTypeReviewStarted, event.EventType)
		assert.Equal(t, aggregateID, event.AggregateID)
		assert.Equal(t, "literature_review", event.AggregateType)
		assert.Equal(t, 1, event.EventVersion)
		assert.NotEmpty(t, event.Payload)
		assert.False(t, event.CreatedAt.IsZero())
	})
}

func TestReviewStartedPayload(t *testing.T) {
	t.Run("fields are correctly set", func(t *testing.T) {
		requestID := uuid.New()
		payload := ReviewStartedPayload{
			RequestID:      requestID,
			OrgID:          "org-123",
			ProjectID:      "proj-456",
			UserID:         "user-789",
			Query:          "machine learning in genomics",
			ExpansionDepth: 2,
		}

		assert.Equal(t, requestID, payload.RequestID)
		assert.Equal(t, "org-123", payload.OrgID)
		assert.Equal(t, "proj-456", payload.ProjectID)
		assert.Equal(t, "user-789", payload.UserID)
		assert.Equal(t, "machine learning in genomics", payload.Query)
		assert.Equal(t, 2, payload.ExpansionDepth)
	})
}

func TestReviewCompletedPayload(t *testing.T) {
	t.Run("fields are correctly set", func(t *testing.T) {
		requestID := uuid.New()
		duration := 5 * time.Minute
		payload := ReviewCompletedPayload{
			RequestID:      requestID,
			OrgID:          "org-123",
			ProjectID:      "proj-456",
			KeywordsFound:  15,
			PapersFound:    250,
			PapersIngested: 200,
			PapersFailed:   10,
			Duration:       duration,
		}

		assert.Equal(t, requestID, payload.RequestID)
		assert.Equal(t, 15, payload.KeywordsFound)
		assert.Equal(t, 250, payload.PapersFound)
		assert.Equal(t, 200, payload.PapersIngested)
		assert.Equal(t, 10, payload.PapersFailed)
		assert.Equal(t, duration, payload.Duration)
	})
}

func TestReviewFailedPayload(t *testing.T) {
	t.Run("fields are correctly set", func(t *testing.T) {
		requestID := uuid.New()
		payload := ReviewFailedPayload{
			RequestID: requestID,
			OrgID:     "org-123",
			ProjectID: "proj-456",
			Error:     "workflow timed out",
			Phase:     "searching",
		}

		assert.Equal(t, requestID, payload.RequestID)
		assert.Equal(t, "workflow timed out", payload.Error)
		assert.Equal(t, "searching", payload.Phase)
	})
}

func TestPapersDiscoveredPayload(t *testing.T) {
	t.Run("fields are correctly set", func(t *testing.T) {
		requestID := uuid.New()
		keywordID := uuid.New()
		paperIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
		payload := PapersDiscoveredPayload{
			RequestID: requestID,
			OrgID:     "org-123",
			ProjectID: "proj-456",
			KeywordID: keywordID,
			Source:    SourceTypeSemanticScholar,
			PaperIDs:  paperIDs,
			Count:     3,
		}

		assert.Equal(t, requestID, payload.RequestID)
		assert.Equal(t, keywordID, payload.KeywordID)
		assert.Equal(t, SourceTypeSemanticScholar, payload.Source)
		assert.Equal(t, paperIDs, payload.PaperIDs)
		assert.Equal(t, 3, payload.Count)
	})
}

func TestTenant(t *testing.T) {
	t.Run("tenant fields", func(t *testing.T) {
		tenant := Tenant{
			OrgID:     "org-abc",
			ProjectID: "proj-xyz",
			UserID:    "user-123",
		}

		assert.Equal(t, "org-abc", tenant.OrgID)
		assert.Equal(t, "proj-xyz", tenant.ProjectID)
		assert.Equal(t, "user-123", tenant.UserID)
	})
}

func TestPaper(t *testing.T) {
	t.Run("paper struct fields", func(t *testing.T) {
		id := uuid.New()
		pubDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
		now := time.Now()

		paper := Paper{
			ID:                id,
			CanonicalID:       "doi:10.1234/test",
			Title:             "Test Paper Title",
			Abstract:          "This is a test abstract.",
			Authors:           []Author{{Name: "Test Author"}},
			PublicationDate:   &pubDate,
			PublicationYear:   2024,
			Venue:             "Nature",
			Journal:           "Nature",
			Volume:            "123",
			Issue:             "4",
			Pages:             "100-110",
			CitationCount:     50,
			ReferenceCount:    25,
			PDFURL:            "https://example.com/paper.pdf",
			OpenAccess:        true,
			KeywordsExtracted: false,
			CreatedAt:         now,
			UpdatedAt:         now,
		}

		assert.Equal(t, id, paper.ID)
		assert.Equal(t, "doi:10.1234/test", paper.CanonicalID)
		assert.Equal(t, "Test Paper Title", paper.Title)
		assert.Equal(t, "This is a test abstract.", paper.Abstract)
		assert.Len(t, paper.Authors, 1)
		assert.Equal(t, 2024, paper.PublicationYear)
		assert.Equal(t, 50, paper.CitationCount)
		assert.True(t, paper.OpenAccess)
	})
}

func TestKeywordSearch(t *testing.T) {
	t.Run("keyword search fields", func(t *testing.T) {
		id := uuid.New()
		keywordID := uuid.New()
		now := time.Now()
		dateFrom := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		dateTo := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

		search := KeywordSearch{
			ID:               id,
			KeywordID:        keywordID,
			SourceAPI:        SourceTypeSemanticScholar,
			SearchedAt:       now,
			DateFrom:         &dateFrom,
			DateTo:           &dateTo,
			SearchWindowHash: "abc123hash",
			PapersFound:      100,
			Status:           SearchStatusCompleted,
			ErrorMessage:     "",
		}

		assert.Equal(t, id, search.ID)
		assert.Equal(t, keywordID, search.KeywordID)
		assert.Equal(t, SourceTypeSemanticScholar, search.SourceAPI)
		assert.Equal(t, 100, search.PapersFound)
		assert.Equal(t, SearchStatusCompleted, search.Status)
	})
}

func TestKeywordPaperMapping(t *testing.T) {
	t.Run("mapping fields", func(t *testing.T) {
		id := uuid.New()
		keywordID := uuid.New()
		paperID := uuid.New()
		now := time.Now()
		confidence := 0.95

		mapping := KeywordPaperMapping{
			ID:              id,
			KeywordID:       keywordID,
			PaperID:         paperID,
			MappingType:     MappingTypeExtracted,
			SourceType:      SourceTypeSemanticScholar,
			ConfidenceScore: &confidence,
			CreatedAt:       now,
		}

		assert.Equal(t, id, mapping.ID)
		assert.Equal(t, keywordID, mapping.KeywordID)
		assert.Equal(t, paperID, mapping.PaperID)
		assert.Equal(t, MappingTypeExtracted, mapping.MappingType)
		assert.NotNil(t, mapping.ConfidenceScore)
		assert.Equal(t, 0.95, *mapping.ConfidenceScore)
	})
}

func TestRequestKeywordMapping(t *testing.T) {
	t.Run("mapping fields", func(t *testing.T) {
		id := uuid.New()
		requestID := uuid.New()
		keywordID := uuid.New()
		sourcePaperID := uuid.New()
		now := time.Now()

		mapping := RequestKeywordMapping{
			ID:              id,
			RequestID:       requestID,
			KeywordID:       keywordID,
			ExtractionRound: 1,
			SourcePaperID:   &sourcePaperID,
			SourceType:      "paper_keywords",
			CreatedAt:       now,
		}

		assert.Equal(t, id, mapping.ID)
		assert.Equal(t, requestID, mapping.RequestID)
		assert.Equal(t, keywordID, mapping.KeywordID)
		assert.Equal(t, 1, mapping.ExtractionRound)
		assert.NotNil(t, mapping.SourcePaperID)
		assert.Equal(t, sourcePaperID, *mapping.SourcePaperID)
	})
}

func TestRequestPaperMapping(t *testing.T) {
	t.Run("mapping fields", func(t *testing.T) {
		id := uuid.New()
		requestID := uuid.New()
		paperID := uuid.New()
		keywordID := uuid.New()
		now := time.Now()

		mapping := RequestPaperMapping{
			ID:                     id,
			RequestID:              requestID,
			PaperID:                paperID,
			DiscoveredViaKeywordID: &keywordID,
			DiscoveredViaSource:    SourceTypeOpenAlex,
			ExpansionDepth:         0,
			IngestionStatus:        IngestionStatusPending,
			IngestionJobID:         "",
			IngestionError:         "",
			CreatedAt:              now,
			UpdatedAt:              now,
		}

		assert.Equal(t, id, mapping.ID)
		assert.Equal(t, requestID, mapping.RequestID)
		assert.Equal(t, paperID, mapping.PaperID)
		assert.NotNil(t, mapping.DiscoveredViaKeywordID)
		assert.Equal(t, SourceTypeOpenAlex, mapping.DiscoveredViaSource)
		assert.Equal(t, IngestionStatusPending, mapping.IngestionStatus)
	})
}

func TestReviewProgressEvent(t *testing.T) {
	t.Run("event fields", func(t *testing.T) {
		id := uuid.New()
		requestID := uuid.New()
		now := time.Now()
		eventData := map[string]interface{}{
			"keyword":      "machine learning",
			"papers_found": 42,
		}

		event := ReviewProgressEvent{
			ID:        id,
			RequestID: requestID,
			EventType: "keyword_searched",
			EventData: eventData,
			CreatedAt: now,
		}

		assert.Equal(t, id, event.ID)
		assert.Equal(t, requestID, event.RequestID)
		assert.Equal(t, "keyword_searched", event.EventType)
		assert.Equal(t, "machine learning", event.EventData["keyword"])
	})
}

func TestReviewProgress(t *testing.T) {
	t.Run("progress fields", func(t *testing.T) {
		requestID := uuid.New()
		progress := ReviewProgress{
			RequestID:        requestID,
			Status:           ReviewStatusSearching,
			CurrentPhase:     "searching",
			KeywordsFound:    10,
			KeywordsSearched: 5,
			PapersFound:      150,
			PapersIngested:   0,
			PapersFailed:     0,
			SourceProgress:   make(map[SourceType]*SourceProgress),
			StartedAt:        time.Now(),
		}

		progress.SourceProgress[SourceTypeSemanticScholar] = &SourceProgress{
			Source:      SourceTypeSemanticScholar,
			Searched:    3,
			PapersFound: 80,
			Status:      SearchStatusCompleted,
		}

		assert.Equal(t, requestID, progress.RequestID)
		assert.Equal(t, ReviewStatusSearching, progress.Status)
		assert.Equal(t, 10, progress.KeywordsFound)
		assert.Len(t, progress.SourceProgress, 1)
		assert.Equal(t, 80, progress.SourceProgress[SourceTypeSemanticScholar].PapersFound)
	})
}

func TestSourceProgress(t *testing.T) {
	t.Run("source progress fields", func(t *testing.T) {
		progress := SourceProgress{
			Source:       SourceTypePubMed,
			Searched:     5,
			PapersFound:  100,
			Status:       SearchStatusInProgress,
			ErrorMessage: "",
		}

		assert.Equal(t, SourceTypePubMed, progress.Source)
		assert.Equal(t, 5, progress.Searched)
		assert.Equal(t, 100, progress.PapersFound)
		assert.Equal(t, SearchStatusInProgress, progress.Status)
	})
}

func TestPaperIdentifier(t *testing.T) {
	t.Run("identifier fields", func(t *testing.T) {
		id := uuid.New()
		paperID := uuid.New()
		now := time.Now()

		identifier := PaperIdentifier{
			ID:              id,
			PaperID:         paperID,
			IdentifierType:  IdentifierTypeDOI,
			IdentifierValue: "10.1038/nature12373",
			SourceAPI:       SourceTypeSemanticScholar,
			DiscoveredAt:    now,
		}

		assert.Equal(t, id, identifier.ID)
		assert.Equal(t, paperID, identifier.PaperID)
		assert.Equal(t, IdentifierTypeDOI, identifier.IdentifierType)
		assert.Equal(t, "10.1038/nature12373", identifier.IdentifierValue)
	})
}

func TestPaperSource(t *testing.T) {
	t.Run("source fields", func(t *testing.T) {
		id := uuid.New()
		paperID := uuid.New()
		now := time.Now()
		metadata := map[string]interface{}{
			"relevance_score": 0.95,
		}

		source := PaperSource{
			ID:             id,
			PaperID:        paperID,
			SourceAPI:      SourceTypeOpenAlex,
			SourceMetadata: metadata,
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		assert.Equal(t, id, source.ID)
		assert.Equal(t, paperID, source.PaperID)
		assert.Equal(t, SourceTypeOpenAlex, source.SourceAPI)
		assert.Equal(t, 0.95, source.SourceMetadata["relevance_score"])
	})
}

// ---------------------------------------------------------------------------
// Tests for Paper.HasIdentifier
// ---------------------------------------------------------------------------

func TestPaper_HasIdentifier(t *testing.T) {
	tests := []struct {
		name        string
		canonicalID string
		want        bool
	}{
		{
			name:        "paper with DOI canonical ID",
			canonicalID: "doi:10.1234/test",
			want:        true,
		},
		{
			name:        "paper with ArXiv canonical ID",
			canonicalID: "arxiv:2103.14030",
			want:        true,
		},
		{
			name:        "paper with empty canonical ID",
			canonicalID: "",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paper := &Paper{
				ID:          uuid.New(),
				CanonicalID: tt.canonicalID,
			}
			assert.Equal(t, tt.want, paper.HasIdentifier())
		})
	}
}

// ---------------------------------------------------------------------------
// Tests for DefaultReviewConfiguration
// ---------------------------------------------------------------------------

func TestDefaultReviewConfiguration(t *testing.T) {
	t.Run("returns expected default values", func(t *testing.T) {
		cfg := DefaultReviewConfiguration()

		assert.Equal(t, 100, cfg.MaxPapers)
		assert.Equal(t, 2, cfg.MaxExpansionDepth)
		assert.Equal(t, 10, cfg.MaxKeywordsPerRound)
		assert.True(t, cfg.IncludePreprints)
		assert.False(t, cfg.RequireOpenAccess)
		assert.Equal(t, 0, cfg.MinCitations)
		assert.Nil(t, cfg.DateFrom)
		assert.Nil(t, cfg.DateTo)
		assert.Empty(t, cfg.LLMModel)
		assert.Nil(t, cfg.Custom)
	})

	t.Run("default sources include semantic_scholar, openalex, pubmed", func(t *testing.T) {
		cfg := DefaultReviewConfiguration()

		require.Len(t, cfg.Sources, 3)
		assert.Equal(t, SourceTypeSemanticScholar, cfg.Sources[0])
		assert.Equal(t, SourceTypeOpenAlex, cfg.Sources[1])
		assert.Equal(t, SourceTypePubMed, cfg.Sources[2])
	})
}

// ---------------------------------------------------------------------------
// Tests for LiteratureReviewRequest.GetTenant
// ---------------------------------------------------------------------------

func TestLiteratureReviewRequest_GetTenant(t *testing.T) {
	t.Run("returns tenant from request fields", func(t *testing.T) {
		req := &LiteratureReviewRequest{
			ID:        uuid.New(),
			OrgID:     "org-abc",
			ProjectID: "proj-xyz",
			UserID:    "user-123",
		}

		tenant := req.GetTenant()

		assert.Equal(t, "org-abc", tenant.OrgID)
		assert.Equal(t, "proj-xyz", tenant.ProjectID)
		assert.Equal(t, "user-123", tenant.UserID)
	})

	t.Run("returns empty tenant when fields are empty", func(t *testing.T) {
		req := &LiteratureReviewRequest{}

		tenant := req.GetTenant()

		assert.Equal(t, "", tenant.OrgID)
		assert.Equal(t, "", tenant.ProjectID)
		assert.Equal(t, "", tenant.UserID)
	})
}

// ---------------------------------------------------------------------------
// Tests for OutboxEvent.WithTenant and OutboxEvent.WithMetadata
// ---------------------------------------------------------------------------

func TestOutboxEvent_WithTenant(t *testing.T) {
	t.Run("sets org and project on event", func(t *testing.T) {
		event := &OutboxEvent{
			EventID:   "evt-1",
			EventType: EventTypeReviewStarted,
		}

		result := event.WithTenant("org-123", "proj-456")

		assert.Equal(t, "org-123", result.OrgID)
		assert.Equal(t, "proj-456", result.ProjectID)
		// Verify fluent interface returns same pointer.
		assert.Same(t, event, result)
	})

	t.Run("overwrites existing tenant", func(t *testing.T) {
		event := &OutboxEvent{
			EventID:   "evt-2",
			OrgID:     "old-org",
			ProjectID: "old-proj",
		}

		result := event.WithTenant("new-org", "new-proj")

		assert.Equal(t, "new-org", result.OrgID)
		assert.Equal(t, "new-proj", result.ProjectID)
	})
}

func TestOutboxEvent_WithMetadata(t *testing.T) {
	t.Run("sets metadata on event", func(t *testing.T) {
		event := &OutboxEvent{
			EventID:   "evt-1",
			EventType: EventTypeReviewCompleted,
		}

		meta := map[string]interface{}{
			"duration_ms": 5000,
			"source":      "temporal",
		}
		result := event.WithMetadata(meta)

		assert.Equal(t, meta, result.Metadata)
		// Verify fluent interface returns same pointer.
		assert.Same(t, event, result)
	})

	t.Run("overwrites existing metadata", func(t *testing.T) {
		event := &OutboxEvent{
			EventID:  "evt-2",
			Metadata: map[string]interface{}{"old_key": "old_value"},
		}

		newMeta := map[string]interface{}{"new_key": "new_value"}
		result := event.WithMetadata(newMeta)

		assert.Equal(t, newMeta, result.Metadata)
		assert.NotContains(t, result.Metadata, "old_key")
	})

	t.Run("nil metadata clears metadata", func(t *testing.T) {
		event := &OutboxEvent{
			EventID:  "evt-3",
			Metadata: map[string]interface{}{"key": "value"},
		}

		result := event.WithMetadata(nil)
		assert.Nil(t, result.Metadata)
	})
}

// ---------------------------------------------------------------------------
// Tests for NewOutboxEvent error path
// ---------------------------------------------------------------------------

func TestNewOutboxEvent_MarshalError(t *testing.T) {
	t.Run("returns error for unmarshalable payload", func(t *testing.T) {
		// Channels cannot be JSON-marshaled.
		unmarshalable := make(chan int)

		_, err := NewOutboxEvent("test.event", "agg-1", "test_aggregate", unmarshalable)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chan")
	})
}

func TestNewOutboxEvent_FluentChaining(t *testing.T) {
	t.Run("WithTenant and WithMetadata can be chained", func(t *testing.T) {
		payload := map[string]string{"key": "value"}
		event, err := NewOutboxEvent(EventTypeReviewStarted, "agg-1", "literature_review", payload)
		require.NoError(t, err)

		result := event.WithTenant("org-1", "proj-1").WithMetadata(map[string]interface{}{"trace_id": "abc"})

		assert.Equal(t, "org-1", result.OrgID)
		assert.Equal(t, "proj-1", result.ProjectID)
		assert.Equal(t, "abc", result.Metadata["trace_id"])
	})
}

// ---------------------------------------------------------------------------
// Tests for error constructors and ValidationError.Unwrap
// ---------------------------------------------------------------------------

func TestValidationError_Unwrap(t *testing.T) {
	t.Run("Unwrap returns ErrInvalidInput", func(t *testing.T) {
		err := &ValidationError{
			Field:   "query",
			Message: "cannot be empty",
		}
		assert.Equal(t, ErrInvalidInput, err.Unwrap())
	})

	t.Run("errors.Is matches ErrInvalidInput", func(t *testing.T) {
		err := &ValidationError{
			Field:   "max_papers",
			Message: "must be positive",
		}
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("errors.Is does not match unrelated sentinels", func(t *testing.T) {
		err := &ValidationError{
			Field:   "query",
			Message: "too long",
		}
		assert.False(t, errors.Is(err, ErrNotFound))
		assert.False(t, errors.Is(err, ErrRateLimited))
		assert.False(t, errors.Is(err, ErrAlreadyExists))
	})
}

func TestNewValidationError(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		message string
	}{
		{
			name:    "simple field validation",
			field:   "query",
			message: "cannot be empty",
		},
		{
			name:    "numeric field validation",
			field:   "max_papers",
			message: "must be greater than zero",
		},
		{
			name:    "nested field validation",
			field:   "configuration.sources",
			message: "at least one source is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewValidationError(tt.field, tt.message)

			require.NotNil(t, err)
			assert.Equal(t, tt.field, err.Field)
			assert.Equal(t, tt.message, err.Message)

			// Verify Error() output format
			expected := fmt.Sprintf("validation error: %s: %s", tt.field, tt.message)
			assert.Equal(t, expected, err.Error())

			// Verify Unwrap chain
			assert.ErrorIs(t, err, ErrInvalidInput)

			// Verify type assertion via errors.As
			var ve *ValidationError
			require.True(t, errors.As(err, &ve))
			assert.Equal(t, tt.field, ve.Field)
			assert.Equal(t, tt.message, ve.Message)
		})
	}
}

func TestNewNotFoundError(t *testing.T) {
	tests := []struct {
		name   string
		entity string
		id     string
	}{
		{
			name:   "paper not found",
			entity: "paper",
			id:     "doi:10.1038/nature12373",
		},
		{
			name:   "review request not found by UUID",
			entity: "review_request",
			id:     uuid.New().String(),
		},
		{
			name:   "keyword not found",
			entity: "keyword",
			id:     "machine-learning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewNotFoundError(tt.entity, tt.id)

			require.NotNil(t, err)
			assert.Equal(t, tt.entity, err.Entity)
			assert.Equal(t, tt.id, err.ID)

			// Verify Error() output format
			expected := fmt.Sprintf("%s not found: %s", tt.entity, tt.id)
			assert.Equal(t, expected, err.Error())

			// Verify Unwrap chain matches ErrNotFound sentinel
			assert.ErrorIs(t, err, ErrNotFound)

			// Verify errors.Is does NOT match unrelated sentinels
			assert.False(t, errors.Is(err, ErrAlreadyExists))
			assert.False(t, errors.Is(err, ErrInvalidInput))

			// Verify type assertion via errors.As
			var nfe *NotFoundError
			require.True(t, errors.As(err, &nfe))
			assert.Equal(t, tt.entity, nfe.Entity)
			assert.Equal(t, tt.id, nfe.ID)
		})
	}
}

func TestNewAlreadyExistsError(t *testing.T) {
	tests := []struct {
		name   string
		entity string
		id     string
	}{
		{
			name:   "paper already exists",
			entity: "paper",
			id:     "doi:10.1234/duplicate",
		},
		{
			name:   "keyword already exists",
			entity: "keyword",
			id:     "machine learning",
		},
		{
			name:   "review request already exists",
			entity: "review_request",
			id:     uuid.New().String(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewAlreadyExistsError(tt.entity, tt.id)

			require.NotNil(t, err)
			assert.Equal(t, tt.entity, err.Entity)
			assert.Equal(t, tt.id, err.ID)

			// Verify Error() output format
			expected := fmt.Sprintf("%s already exists: %s", tt.entity, tt.id)
			assert.Equal(t, expected, err.Error())

			// Verify Unwrap chain matches ErrAlreadyExists sentinel
			assert.ErrorIs(t, err, ErrAlreadyExists)

			// Verify errors.Is does NOT match unrelated sentinels
			assert.False(t, errors.Is(err, ErrNotFound))
			assert.False(t, errors.Is(err, ErrInvalidInput))

			// Verify type assertion via errors.As
			var aee *AlreadyExistsError
			require.True(t, errors.As(err, &aee))
			assert.Equal(t, tt.entity, aee.Entity)
			assert.Equal(t, tt.id, aee.ID)
		})
	}
}

func TestNewRateLimitError(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		retryAfter time.Duration
	}{
		{
			name:       "semantic scholar rate limit",
			source:     "semantic_scholar",
			retryAfter: 30 * time.Second,
		},
		{
			name:       "openalex rate limit with zero retry",
			source:     "openalex",
			retryAfter: 0,
		},
		{
			name:       "pubmed rate limit with long retry",
			source:     "pubmed",
			retryAfter: 5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewRateLimitError(tt.source, tt.retryAfter)

			require.NotNil(t, err)
			assert.Equal(t, tt.source, err.Source)
			assert.Equal(t, tt.retryAfter, err.RetryAfter)

			// Verify Error() output format
			expected := fmt.Sprintf("rate limited by %s: retry after %s", tt.source, tt.retryAfter)
			assert.Equal(t, expected, err.Error())

			// Verify Unwrap chain matches ErrRateLimited sentinel
			assert.ErrorIs(t, err, ErrRateLimited)

			// Verify errors.Is does NOT match unrelated sentinels
			assert.False(t, errors.Is(err, ErrNotFound))
			assert.False(t, errors.Is(err, ErrServiceUnavailable))

			// Verify type assertion via errors.As
			var rle *RateLimitError
			require.True(t, errors.As(err, &rle))
			assert.Equal(t, tt.source, rle.Source)
			assert.Equal(t, tt.retryAfter, rle.RetryAfter)
		})
	}
}

func TestNewExternalAPIError(t *testing.T) {
	t.Run("with cause error", func(t *testing.T) {
		cause := fmt.Errorf("connection refused")
		err := NewExternalAPIError("scopus", 500, "internal server error", cause)

		require.NotNil(t, err)
		assert.Equal(t, "scopus", err.Source)
		assert.Equal(t, 500, err.StatusCode)
		assert.Equal(t, "internal server error", err.Message)
		assert.Equal(t, cause, err.Cause)

		// Verify Error() output format
		assert.Equal(t, "scopus API error (status 500): internal server error", err.Error())

		// Verify Unwrap returns the cause
		assert.Equal(t, cause, err.Unwrap())

		// Verify errors.Is matches the cause through the chain
		assert.ErrorIs(t, err, cause)

		// Verify type assertion via errors.As
		var apiErr *ExternalAPIError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, "scopus", apiErr.Source)
		assert.Equal(t, 500, apiErr.StatusCode)
	})

	t.Run("without cause error", func(t *testing.T) {
		err := NewExternalAPIError("arxiv", 404, "not found", nil)

		require.NotNil(t, err)
		assert.Equal(t, "arxiv", err.Source)
		assert.Equal(t, 404, err.StatusCode)
		assert.Equal(t, "not found", err.Message)
		assert.Nil(t, err.Cause)

		// Verify Error() output format
		assert.Equal(t, "arxiv API error (status 404): not found", err.Error())

		// Verify Unwrap returns ErrServiceUnavailable when no cause
		assert.Equal(t, ErrServiceUnavailable, err.Unwrap())
		assert.ErrorIs(t, err, ErrServiceUnavailable)
	})

	t.Run("with wrapped sentinel cause", func(t *testing.T) {
		cause := fmt.Errorf("wrapped: %w", ErrServiceUnavailable)
		err := NewExternalAPIError("biorxiv", 503, "service unavailable", cause)

		// errors.Is should walk the Unwrap chain to match the sentinel
		assert.ErrorIs(t, err, ErrServiceUnavailable)
	})

	t.Run("various status codes", func(t *testing.T) {
		statusCodes := []struct {
			code    int
			message string
		}{
			{400, "bad request"},
			{401, "unauthorized"},
			{403, "forbidden"},
			{429, "too many requests"},
			{500, "internal server error"},
			{502, "bad gateway"},
			{503, "service unavailable"},
		}

		for _, sc := range statusCodes {
			t.Run(fmt.Sprintf("status_%d", sc.code), func(t *testing.T) {
				err := NewExternalAPIError("test_source", sc.code, sc.message, nil)

				assert.Equal(t, sc.code, err.StatusCode)
				assert.Contains(t, err.Error(), fmt.Sprintf("status %d", sc.code))
				assert.Contains(t, err.Error(), sc.message)
			})
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for TenantFromIDs constructor
// ---------------------------------------------------------------------------

func TestPauseReason_String(t *testing.T) {
	tests := []struct {
		reason   PauseReason
		expected string
	}{
		{PauseReasonUser, "user"},
		{PauseReasonBudgetExhausted, "budget_exhausted"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.reason))
		})
	}
}

func TestLiteratureReviewRequest_PauseFields(t *testing.T) {
	t.Run("pause fields are set correctly", func(t *testing.T) {
		pausedAt := time.Now()
		req := &LiteratureReviewRequest{
			ID:            uuid.New(),
			OrgID:         "org-123",
			ProjectID:     "proj-456",
			UserID:        "user-789",
			Status:        ReviewStatusPaused,
			PauseReason:   PauseReasonBudgetExhausted,
			PausedAt:      &pausedAt,
			PausedAtPhase: "searching",
		}

		assert.Equal(t, ReviewStatusPaused, req.Status)
		assert.Equal(t, PauseReasonBudgetExhausted, req.PauseReason)
		assert.NotNil(t, req.PausedAt)
		assert.Equal(t, pausedAt, *req.PausedAt)
		assert.Equal(t, "searching", req.PausedAtPhase)
	})

	t.Run("pause fields default to zero values", func(t *testing.T) {
		req := &LiteratureReviewRequest{
			ID:     uuid.New(),
			Status: ReviewStatusPending,
		}

		assert.Equal(t, PauseReason(""), req.PauseReason)
		assert.Nil(t, req.PausedAt)
		assert.Equal(t, "", req.PausedAtPhase)
	})

	t.Run("user-initiated pause", func(t *testing.T) {
		pausedAt := time.Now()
		req := &LiteratureReviewRequest{
			ID:            uuid.New(),
			Status:        ReviewStatusPaused,
			PauseReason:   PauseReasonUser,
			PausedAt:      &pausedAt,
			PausedAtPhase: "ingesting",
		}

		assert.Equal(t, PauseReasonUser, req.PauseReason)
		assert.Equal(t, "ingesting", req.PausedAtPhase)
	})

	t.Run("paused status is not terminal", func(t *testing.T) {
		req := &LiteratureReviewRequest{
			Status: ReviewStatusPaused,
		}
		assert.True(t, req.IsActive())
	})
}

func TestTenantFromIDs(t *testing.T) {
	tests := []struct {
		name      string
		orgID     string
		projectID string
		userID    string
	}{
		{
			name:      "typical tenant IDs",
			orgID:     "org-abc",
			projectID: "proj-xyz",
			userID:    "user-123",
		},
		{
			name:      "UUID-style IDs",
			orgID:     uuid.New().String(),
			projectID: uuid.New().String(),
			userID:    uuid.New().String(),
		},
		{
			name:      "empty IDs",
			orgID:     "",
			projectID: "",
			userID:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant := TenantFromIDs(tt.orgID, tt.projectID, tt.userID)

			assert.Equal(t, tt.orgID, tenant.OrgID)
			assert.Equal(t, tt.projectID, tenant.ProjectID)
			assert.Equal(t, tt.userID, tenant.UserID)

			// Verify it produces the same result as a struct literal
			expected := Tenant{
				OrgID:     tt.orgID,
				ProjectID: tt.projectID,
				UserID:    tt.userID,
			}
			assert.Equal(t, expected, tenant)
		})
	}
}
