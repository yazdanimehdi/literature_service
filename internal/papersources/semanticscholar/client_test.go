package semanticscholar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/papersources"
)

// Compile-time check that Client implements papersources.PaperSource.
var _ papersources.PaperSource = (*Client)(nil)

func TestNewClient(t *testing.T) {
	t.Run("creates client with default values", func(t *testing.T) {
		client := NewClient(Config{Enabled: true}, nil)

		require.NotNil(t, client)
		assert.Equal(t, DefaultBaseURL, client.config.BaseURL)
		assert.Equal(t, DefaultTimeout, client.config.Timeout)
		assert.Equal(t, DefaultRateLimit, client.config.RateLimit)
		assert.Equal(t, DefaultBurstSize, client.config.BurstSize)
		assert.Equal(t, DefaultMaxResults, client.config.MaxResults)
		assert.True(t, client.config.Enabled)
	})

	t.Run("creates client with custom config", func(t *testing.T) {
		cfg := Config{
			BaseURL:    "https://custom.api.com/v1",
			APIKey:     "test-api-key",
			Timeout:    60 * time.Second,
			RateLimit:  50.0,
			BurstSize:  20,
			MaxResults: 200,
			Enabled:    true,
		}
		client := NewClient(cfg, nil)

		require.NotNil(t, client)
		assert.Equal(t, cfg.BaseURL, client.config.BaseURL)
		assert.Equal(t, cfg.Timeout, client.config.Timeout)
		assert.Equal(t, cfg.RateLimit, client.config.RateLimit)
		assert.Equal(t, cfg.BurstSize, client.config.BurstSize)
		assert.Equal(t, cfg.MaxResults, client.config.MaxResults)
	})

	t.Run("uses provided HTTP client", func(t *testing.T) {
		httpClient := papersources.NewHTTPClient(papersources.HTTPClientConfig{
			RateLimit: 100,
			BurstSize: 50,
		})
		client := NewClient(Config{Enabled: true}, httpClient)

		require.NotNil(t, client)
		assert.Equal(t, httpClient, client.httpClient)
	})

	t.Run("implements PaperSource interface", func(t *testing.T) {
		client := NewClient(Config{Enabled: true}, nil)

		assert.Equal(t, domain.SourceTypeSemanticScholar, client.SourceType())
		assert.Equal(t, "Semantic Scholar", client.Name())
		assert.True(t, client.IsEnabled())
	})

	t.Run("disabled client returns false for IsEnabled", func(t *testing.T) {
		client := NewClient(Config{Enabled: false}, nil)
		assert.False(t, client.IsEnabled())
	})
}

func TestClient_Search(t *testing.T) {
	t.Run("successful search returns papers", func(t *testing.T) {
		response := SearchResponse{
			Total:  150,
			Offset: 0,
			Next:   10,
			Data: []PaperResult{
				{
					PaperID:         "abc123",
					Title:           "CRISPR Gene Editing: A Review",
					Abstract:        "This paper reviews CRISPR technology...",
					Year:            2023,
					PublicationDate: "2023-06-15",
					Venue:           "Nature Reviews",
					Journal: &Journal{
						Name:   "Nature Reviews Genetics",
						Volume: "24",
						Pages:  "100-120",
					},
					Authors: []Author{
						{AuthorID: "auth1", Name: "Jane Doe"},
						{AuthorID: "auth2", Name: "John Smith"},
					},
					CitationCount:  50,
					ReferenceCount: 100,
					IsOpenAccess:   true,
					OpenAccessPDF: &OpenAccessPDF{
						URL:    "https://example.com/paper.pdf",
						Status: "GOLD",
					},
					ExternalIDs: &ExternalIDs{
						DOI:    "10.1038/s41576-023-00001-1",
						PubMed: "12345678",
					},
				},
				{
					PaperID:  "def456",
					Title:    "Gene Therapy Applications",
					Abstract: "Gene therapy has shown promise...",
					Year:     2022,
					Authors: []Author{
						{Name: "Alice Johnson"},
					},
					CitationCount: 25,
				},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Contains(t, r.URL.Path, "/paper/search")
			assert.Equal(t, "CRISPR gene editing", r.URL.Query().Get("query"))
			assert.Contains(t, r.URL.Query().Get("fields"), "paperId")
			assert.Contains(t, r.URL.Query().Get("fields"), "title")

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:   server.URL,
			Enabled:   true,
			RateLimit: 100,
			BurstSize: 10,
		}, nil)

		params := papersources.SearchParams{
			Query:      "CRISPR gene editing",
			MaxResults: 10,
		}

		result, err := client.Search(context.Background(), params)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 150, result.TotalResults)
		assert.True(t, result.HasMore)
		assert.Equal(t, 10, result.NextOffset)
		assert.Equal(t, domain.SourceTypeSemanticScholar, result.Source)
		assert.Greater(t, result.SearchDuration, time.Duration(0))

		require.Len(t, result.Papers, 2)

		// Verify first paper conversion
		paper1 := result.Papers[0]
		assert.Equal(t, "CRISPR Gene Editing: A Review", paper1.Title)
		assert.Equal(t, "This paper reviews CRISPR technology...", paper1.Abstract)
		assert.Equal(t, 2023, paper1.PublicationYear)
		assert.NotNil(t, paper1.PublicationDate)
		assert.Equal(t, "2023-06-15", paper1.PublicationDate.Format("2006-01-02"))
		assert.Equal(t, "Nature Reviews", paper1.Venue)
		assert.Equal(t, "Nature Reviews Genetics", paper1.Journal)
		assert.Equal(t, "24", paper1.Volume)
		assert.Equal(t, "100-120", paper1.Pages)
		assert.Equal(t, 50, paper1.CitationCount)
		assert.Equal(t, 100, paper1.ReferenceCount)
		assert.True(t, paper1.OpenAccess)
		assert.Equal(t, "https://example.com/paper.pdf", paper1.PDFURL)
		assert.Equal(t, "doi:10.1038/s41576-023-00001-1", paper1.CanonicalID)

		require.Len(t, paper1.Authors, 2)
		assert.Equal(t, "Jane Doe", paper1.Authors[0].Name)
		assert.Equal(t, "John Smith", paper1.Authors[1].Name)

		// Verify second paper with minimal data
		paper2 := result.Papers[1]
		assert.Equal(t, "Gene Therapy Applications", paper2.Title)
		assert.Equal(t, "s2:def456", paper2.CanonicalID) // Falls back to S2 ID
		assert.Nil(t, paper2.PublicationDate)
	})

	t.Run("search with offset and pagination", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "50", r.URL.Query().Get("offset"))
			assert.Equal(t, "25", r.URL.Query().Get("limit"))

			response := SearchResponse{
				Total:  100,
				Offset: 50,
				Next:   75,
				Data:   []PaperResult{},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:   server.URL,
			Enabled:   true,
			RateLimit: 100,
			BurstSize: 10,
		}, nil)

		params := papersources.SearchParams{
			Query:      "test",
			MaxResults: 25,
			Offset:     50,
		}

		result, err := client.Search(context.Background(), params)

		require.NoError(t, err)
		assert.True(t, result.HasMore)
		assert.Equal(t, 75, result.NextOffset)
	})

	t.Run("search with open access filter", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check that openAccessPdf param is present (value can be empty)
			_, hasOA := r.URL.Query()["openAccessPdf"]
			assert.True(t, hasOA, "openAccessPdf parameter should be present")

			response := SearchResponse{Total: 0, Data: []PaperResult{}}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:   server.URL,
			Enabled:   true,
			RateLimit: 100,
			BurstSize: 10,
		}, nil)

		params := papersources.SearchParams{
			Query:          "test",
			OpenAccessOnly: true,
		}

		_, err := client.Search(context.Background(), params)
		require.NoError(t, err)
	})

	t.Run("search with minimum citations", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "10", r.URL.Query().Get("minCitationCount"))

			response := SearchResponse{Total: 0, Data: []PaperResult{}}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:   server.URL,
			Enabled:   true,
			RateLimit: 100,
			BurstSize: 10,
		}, nil)

		params := papersources.SearchParams{
			Query:        "test",
			MinCitations: 10,
		}

		_, err := client.Search(context.Background(), params)
		require.NoError(t, err)
	})

	t.Run("search handles API error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{
				Error: "Invalid query parameter",
			})
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:    server.URL,
			Enabled:    true,
			RateLimit:  100,
			BurstSize:  10,
			MaxResults: 10,
		}, nil)

		params := papersources.SearchParams{Query: "test"}

		result, err := client.Search(context.Background(), params)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "Invalid query parameter")

		var apiErr *domain.ExternalAPIError
		assert.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	})

	t.Run("search respects context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(500 * time.Millisecond)
			response := SearchResponse{Total: 0, Data: []PaperResult{}}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:   server.URL,
			Enabled:   true,
			RateLimit: 100,
			BurstSize: 10,
		}, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		params := papersources.SearchParams{Query: "test"}

		_, err := client.Search(ctx, params)

		require.Error(t, err)
	})
}

func TestClient_Search_DateFilter(t *testing.T) {
	t.Run("filters by date from only", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "2020-", r.URL.Query().Get("year"))

			response := SearchResponse{
				Total: 3,
				Data: []PaperResult{
					{PaperID: "1", Title: "Paper 2019", Year: 2019, PublicationDate: "2019-06-15"},
					{PaperID: "2", Title: "Paper 2020", Year: 2020, PublicationDate: "2020-03-01"},
					{PaperID: "3", Title: "Paper 2021", Year: 2021, PublicationDate: "2021-01-10"},
				},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:   server.URL,
			Enabled:   true,
			RateLimit: 100,
			BurstSize: 10,
		}, nil)

		dateFrom := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		params := papersources.SearchParams{
			Query:    "test",
			DateFrom: &dateFrom,
		}

		result, err := client.Search(context.Background(), params)

		require.NoError(t, err)
		// Client-side filtering should remove the 2019 paper
		require.Len(t, result.Papers, 2)
		assert.Equal(t, "Paper 2020", result.Papers[0].Title)
		assert.Equal(t, "Paper 2021", result.Papers[1].Title)
	})

	t.Run("filters by date to only", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "-2021", r.URL.Query().Get("year"))

			response := SearchResponse{
				Total: 3,
				Data: []PaperResult{
					{PaperID: "1", Title: "Paper 2020", Year: 2020, PublicationDate: "2020-06-15"},
					{PaperID: "2", Title: "Paper 2021", Year: 2021, PublicationDate: "2021-06-01"},
					{PaperID: "3", Title: "Paper 2022", Year: 2022, PublicationDate: "2022-01-10"},
				},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:   server.URL,
			Enabled:   true,
			RateLimit: 100,
			BurstSize: 10,
		}, nil)

		dateTo := time.Date(2021, 12, 31, 23, 59, 59, 0, time.UTC)
		params := papersources.SearchParams{
			Query:  "test",
			DateTo: &dateTo,
		}

		result, err := client.Search(context.Background(), params)

		require.NoError(t, err)
		// Client-side filtering should remove the 2022 paper
		require.Len(t, result.Papers, 2)
		assert.Equal(t, "Paper 2020", result.Papers[0].Title)
		assert.Equal(t, "Paper 2021", result.Papers[1].Title)
	})

	t.Run("filters by date range", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "2020-2022", r.URL.Query().Get("year"))

			response := SearchResponse{
				Total: 4,
				Data: []PaperResult{
					{PaperID: "1", Title: "Paper 2019", Year: 2019, PublicationDate: "2019-12-15"},
					{PaperID: "2", Title: "Paper 2020", Year: 2020, PublicationDate: "2020-06-01"},
					{PaperID: "3", Title: "Paper 2021", Year: 2021, PublicationDate: "2021-06-01"},
					{PaperID: "4", Title: "Paper 2023", Year: 2023, PublicationDate: "2023-01-10"},
				},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:   server.URL,
			Enabled:   true,
			RateLimit: 100,
			BurstSize: 10,
		}, nil)

		dateFrom := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		dateTo := time.Date(2022, 12, 31, 23, 59, 59, 0, time.UTC)
		params := papersources.SearchParams{
			Query:    "test",
			DateFrom: &dateFrom,
			DateTo:   &dateTo,
		}

		result, err := client.Search(context.Background(), params)

		require.NoError(t, err)
		// Client-side filtering should keep only 2020 and 2021 papers
		require.Len(t, result.Papers, 2)
		assert.Equal(t, "Paper 2020", result.Papers[0].Title)
		assert.Equal(t, "Paper 2021", result.Papers[1].Title)
	})

	t.Run("includes papers without date when filtering", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := SearchResponse{
				Total: 2,
				Data: []PaperResult{
					{PaperID: "1", Title: "Paper with date", Year: 2020, PublicationDate: "2020-06-01"},
					{PaperID: "2", Title: "Paper without date"},
				},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:   server.URL,
			Enabled:   true,
			RateLimit: 100,
			BurstSize: 10,
		}, nil)

		dateFrom := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
		params := papersources.SearchParams{
			Query:    "test",
			DateFrom: &dateFrom,
		}

		result, err := client.Search(context.Background(), params)

		require.NoError(t, err)
		// Both papers should be included (paper without date is not filtered out)
		require.Len(t, result.Papers, 2)
	})
}

func TestClient_GetByID(t *testing.T) {
	t.Run("successful get by paper ID", func(t *testing.T) {
		paperResult := PaperResult{
			PaperID:         "abc123",
			Title:           "Test Paper",
			Abstract:        "This is a test abstract",
			Year:            2023,
			PublicationDate: "2023-06-15",
			Venue:           "Test Conference",
			Authors: []Author{
				{AuthorID: "auth1", Name: "Test Author"},
			},
			CitationCount:  10,
			ReferenceCount: 20,
			IsOpenAccess:   true,
			ExternalIDs: &ExternalIDs{
				DOI: "10.1234/test.2023",
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Contains(t, r.URL.Path, "/paper/abc123")
			assert.Contains(t, r.URL.Query().Get("fields"), "paperId")

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(paperResult)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:   server.URL,
			Enabled:   true,
			RateLimit: 100,
			BurstSize: 10,
		}, nil)

		paper, err := client.GetByID(context.Background(), "abc123")

		require.NoError(t, err)
		require.NotNil(t, paper)
		assert.Equal(t, "Test Paper", paper.Title)
		assert.Equal(t, "This is a test abstract", paper.Abstract)
		assert.Equal(t, 2023, paper.PublicationYear)
		assert.Equal(t, "doi:10.1234/test.2023", paper.CanonicalID)
		require.Len(t, paper.Authors, 1)
		assert.Equal(t, "Test Author", paper.Authors[0].Name)
	})

	t.Run("get by DOI", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// DOI should be URL-encoded in the path
			assert.Contains(t, r.URL.Path, "/paper/")

			paperResult := PaperResult{
				PaperID: "xyz789",
				Title:   "DOI Paper",
				ExternalIDs: &ExternalIDs{
					DOI: "10.1234/example",
				},
			}
			json.NewEncoder(w).Encode(paperResult)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:   server.URL,
			Enabled:   true,
			RateLimit: 100,
			BurstSize: 10,
		}, nil)

		paper, err := client.GetByID(context.Background(), "DOI:10.1234/example")

		require.NoError(t, err)
		require.NotNil(t, paper)
		assert.Equal(t, "DOI Paper", paper.Title)
	})

	t.Run("returns not found error for missing paper", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{
				Error: "Paper not found",
			})
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:   server.URL,
			Enabled:   true,
			RateLimit: 100,
			BurstSize: 10,
		}, nil)

		paper, err := client.GetByID(context.Background(), "nonexistent")

		require.Error(t, err)
		assert.Nil(t, paper)
		assert.ErrorIs(t, err, domain.ErrNotFound)

		var notFoundErr *domain.NotFoundError
		assert.ErrorAs(t, err, &notFoundErr)
		assert.Equal(t, "paper", notFoundErr.Entity)
		assert.Equal(t, "nonexistent", notFoundErr.ID)
	})

	t.Run("handles API error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use 400 Bad Request which is not retried by the HTTP client
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{
				Message: "Invalid paper ID format",
			})
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:    server.URL,
			Enabled:    true,
			RateLimit:  100,
			BurstSize:  10,
			MaxResults: 10,
		}, nil)

		paper, err := client.GetByID(context.Background(), "abc123")

		require.Error(t, err)
		assert.Nil(t, paper)

		var apiErr *domain.ExternalAPIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
		assert.Contains(t, apiErr.Message, "Invalid paper ID format")
	})
}

func TestClient_convertToPaper(t *testing.T) {
	client := NewClient(Config{Enabled: true}, nil)

	t.Run("converts paper with all fields", func(t *testing.T) {
		result := PaperResult{
			PaperID:         "paper123",
			Title:           "Full Paper",
			Abstract:        "Full abstract",
			Year:            2023,
			PublicationDate: "2023-06-15",
			Venue:           "Conference 2023",
			Journal: &Journal{
				Name:   "Journal Name",
				Volume: "10",
				Pages:  "1-20",
			},
			Authors: []Author{
				{AuthorID: "a1", Name: "Author One"},
				{AuthorID: "a2", Name: "Author Two"},
			},
			CitationCount:  100,
			ReferenceCount: 50,
			IsOpenAccess:   true,
			OpenAccessPDF: &OpenAccessPDF{
				URL:    "https://example.com/paper.pdf",
				Status: "GOLD",
			},
			ExternalIDs: &ExternalIDs{
				DOI:           "10.1234/paper",
				ArXiv:         "2306.12345",
				PubMed:        "12345678",
				PubMedCentral: "PMC12345",
			},
		}

		paper := client.convertToPaper(result)

		assert.Equal(t, "Full Paper", paper.Title)
		assert.Equal(t, "Full abstract", paper.Abstract)
		assert.Equal(t, 2023, paper.PublicationYear)
		require.NotNil(t, paper.PublicationDate)
		assert.Equal(t, "2023-06-15", paper.PublicationDate.Format("2006-01-02"))
		assert.Equal(t, "Conference 2023", paper.Venue)
		assert.Equal(t, "Journal Name", paper.Journal)
		assert.Equal(t, "10", paper.Volume)
		assert.Equal(t, "1-20", paper.Pages)
		assert.Equal(t, 100, paper.CitationCount)
		assert.Equal(t, 50, paper.ReferenceCount)
		assert.True(t, paper.OpenAccess)
		assert.Equal(t, "https://example.com/paper.pdf", paper.PDFURL)
		// DOI has highest priority for canonical ID
		assert.Equal(t, "doi:10.1234/paper", paper.CanonicalID)

		require.Len(t, paper.Authors, 2)
		assert.Equal(t, "Author One", paper.Authors[0].Name)
		assert.Equal(t, "Author Two", paper.Authors[1].Name)
	})

	t.Run("handles paper with minimal fields", func(t *testing.T) {
		result := PaperResult{
			PaperID: "minimal123",
			Title:   "Minimal Paper",
		}

		paper := client.convertToPaper(result)

		assert.Equal(t, "Minimal Paper", paper.Title)
		assert.Empty(t, paper.Abstract)
		assert.Zero(t, paper.PublicationYear)
		assert.Nil(t, paper.PublicationDate)
		assert.Empty(t, paper.Journal)
		assert.Empty(t, paper.PDFURL)
		assert.Empty(t, paper.Authors)
		// Falls back to S2 ID
		assert.Equal(t, "s2:minimal123", paper.CanonicalID)
	})

	t.Run("canonical ID priority: DOI > ArXiv > PubMed > S2", func(t *testing.T) {
		// With DOI
		result := PaperResult{
			PaperID: "s2id",
			ExternalIDs: &ExternalIDs{
				DOI:    "10.1234/doi",
				ArXiv:  "2306.12345",
				PubMed: "12345678",
			},
		}
		paper := client.convertToPaper(result)
		assert.Equal(t, "doi:10.1234/doi", paper.CanonicalID)

		// Without DOI, ArXiv is next
		result.ExternalIDs.DOI = ""
		paper = client.convertToPaper(result)
		assert.Equal(t, "arxiv:2306.12345", paper.CanonicalID)

		// Without DOI and ArXiv, PubMed is next
		result.ExternalIDs.ArXiv = ""
		paper = client.convertToPaper(result)
		assert.Equal(t, "pubmed:12345678", paper.CanonicalID)

		// Without any external IDs, S2 ID is used
		result.ExternalIDs = nil
		paper = client.convertToPaper(result)
		assert.Equal(t, "s2:s2id", paper.CanonicalID)
	})

	t.Run("handles invalid publication date", func(t *testing.T) {
		result := PaperResult{
			PaperID:         "paper123",
			Title:           "Paper with bad date",
			PublicationDate: "invalid-date",
			Year:            2023,
		}

		paper := client.convertToPaper(result)

		assert.Nil(t, paper.PublicationDate)
		assert.Equal(t, 2023, paper.PublicationYear)
	})
}

func TestBuildSearchQuery(t *testing.T) {
	t.Run("single term returns term", func(t *testing.T) {
		result := BuildSearchQuery([]string{"CRISPR"}, "AND")
		assert.Equal(t, "CRISPR", result)
	})

	t.Run("empty terms returns empty", func(t *testing.T) {
		result := BuildSearchQuery([]string{}, "AND")
		assert.Empty(t, result)
	})

	t.Run("multiple terms with AND", func(t *testing.T) {
		result := BuildSearchQuery([]string{"CRISPR", "gene editing", "therapy"}, "AND")
		assert.Equal(t, "CRISPR AND gene editing AND therapy", result)
	})

	t.Run("multiple terms with OR", func(t *testing.T) {
		result := BuildSearchQuery([]string{"CRISPR", "TALEN", "zinc finger"}, "OR")
		assert.Equal(t, "CRISPR OR TALEN OR zinc finger", result)
	})

	t.Run("defaults to AND for invalid operator", func(t *testing.T) {
		result := BuildSearchQuery([]string{"term1", "term2"}, "XOR")
		assert.Equal(t, "term1 AND term2", result)
	})

	t.Run("case insensitive operator", func(t *testing.T) {
		result := BuildSearchQuery([]string{"term1", "term2"}, "or")
		assert.Equal(t, "term1 OR term2", result)
	})
}
