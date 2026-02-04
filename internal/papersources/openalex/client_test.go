package openalex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/papersources"
)

// newTestClient creates a client configured for testing with the given server URL.
func newTestClient(serverURL string, enabled bool) *Client {
	cfg := Config{
		BaseURL:    serverURL,
		Email:      "test@example.com",
		Timeout:    5 * time.Second,
		RateLimit:  100, // High rate for testing
		BurstSize:  100,
		MaxResults: 25,
		Enabled:    enabled,
	}

	httpClient := papersources.NewHTTPClient(papersources.HTTPClientConfig{
		Timeout:   cfg.Timeout,
		RateLimit: cfg.RateLimit,
		BurstSize: cfg.BurstSize,
		UserAgent: "TestClient/1.0",
	})

	return NewWithHTTPClient(cfg, httpClient)
}

// sampleSearchResponse returns a sample OpenAlex search response for testing.
func sampleSearchResponse() SearchResponse {
	return SearchResponse{
		Meta: Meta{
			Count:      2,
			DBTime:     42,
			Page:       1,
			PerPage:    25,
			NextCursor: "",
		},
		Results: []Work{
			{
				ID:              "https://openalex.org/W2741809807",
				DOI:             "https://doi.org/10.1038/nature12373",
				Title:           "CRISPR-Cas Systems for Editing",
				DisplayName:     "CRISPR-Cas Systems for Editing, Regulating and Targeting Genomes",
				PublicationYear: 2014,
				PublicationDate: "2014-06-05",
				Type:            "article",
				CitedByCount:    5000,
				IsOpenAccess:    true,
				OpenAccess: &OpenAccess{
					IsOA:     true,
					OAURL:    "https://europepmc.org/articles/pmc4022601?pdf=render",
					OAStatus: "gold",
				},
				Authorships: []Authorship{
					{
						AuthorPosition: "first",
						Author: AuthorInfo{
							ID:          "https://openalex.org/A1234567890",
							DisplayName: "John Smith",
							Orcid:       "https://orcid.org/0000-0001-2345-6789",
						},
						Institutions: []Institution{
							{
								ID:          "https://openalex.org/I123",
								DisplayName: "MIT",
							},
						},
					},
					{
						AuthorPosition: "last",
						Author: AuthorInfo{
							ID:          "https://openalex.org/A9876543210",
							DisplayName: "Jane Doe",
							Orcid:       "",
						},
						Institutions: []Institution{},
					},
				},
				PrimaryLocation: &Location{
					Source: &Source{
						ID:          "https://openalex.org/S123",
						DisplayName: "Nature Biotechnology",
						Type:        "journal",
					},
					PDFURL:  "",
					Version: "publishedVersion",
				},
				IDs: IDs{
					OpenAlex: "https://openalex.org/W2741809807",
					DOI:      "https://doi.org/10.1038/nature12373",
					MAG:      "2741809807",
					PMID:     "https://pubmed.ncbi.nlm.nih.gov/24906146",
					PMCID:    "PMC4022601",
				},
				ReferencedWorks: []string{
					"https://openalex.org/W1234",
					"https://openalex.org/W5678",
				},
				AbstractInvertedIndex: map[string][]int{
					"CRISPR":   {0},
					"is":       {1},
					"a":        {2},
					"powerful": {3},
					"tool":     {4},
					"for":      {5},
					"genome":   {6},
					"editing.": {7},
				},
			},
			{
				ID:              "https://openalex.org/W2741809808",
				DOI:             "https://doi.org/10.1126/science.1234567",
				Title:           "Gene Therapy Advances",
				DisplayName:     "Gene Therapy Advances in 2023",
				PublicationYear: 2023,
				PublicationDate: "2023-01-15",
				Type:            "article",
				CitedByCount:    150,
				IsOpenAccess:    false,
				OpenAccess: &OpenAccess{
					IsOA:     false,
					OAURL:    "",
					OAStatus: "closed",
				},
				Authorships: []Authorship{
					{
						AuthorPosition: "first",
						Author: AuthorInfo{
							ID:          "https://openalex.org/A111",
							DisplayName: "Alice Johnson",
							Orcid:       "https://orcid.org/0000-0002-1111-2222",
						},
						Institutions: []Institution{
							{
								ID:          "https://openalex.org/I456",
								DisplayName: "Stanford University",
							},
						},
					},
				},
				PrimaryLocation: &Location{
					Source: &Source{
						ID:          "https://openalex.org/S456",
						DisplayName: "Science",
						Type:        "journal",
					},
					PDFURL:  "",
					Version: "publishedVersion",
				},
				IDs: IDs{
					OpenAlex: "https://openalex.org/W2741809808",
					DOI:      "https://doi.org/10.1126/science.1234567",
				},
				ReferencedWorks: []string{},
			},
		},
	}
}

// sampleWork returns a single sample work for GetByID tests.
func sampleWork() Work {
	return sampleSearchResponse().Results[0]
}

func TestNewClient(t *testing.T) {
	t.Run("creates client with default config", func(t *testing.T) {
		cfg := Config{
			Enabled: true,
		}
		client := New(cfg)

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
			BaseURL:    "https://custom.api.org",
			Email:      "researcher@university.edu",
			Timeout:    60 * time.Second,
			RateLimit:  20.0,
			BurstSize:  20,
			MaxResults: 50,
			Enabled:    true,
		}
		client := New(cfg)

		require.NotNil(t, client)
		assert.Equal(t, "https://custom.api.org", client.config.BaseURL)
		assert.Equal(t, "researcher@university.edu", client.config.Email)
		assert.Equal(t, 60*time.Second, client.config.Timeout)
		assert.Equal(t, 20.0, client.config.RateLimit)
		assert.Equal(t, 20, client.config.BurstSize)
		assert.Equal(t, 50, client.config.MaxResults)
	})

	t.Run("disabled client", func(t *testing.T) {
		cfg := Config{
			Enabled: false,
		}
		client := New(cfg)

		require.NotNil(t, client)
		assert.False(t, client.IsEnabled())
	})
}

func TestClient_SourceType(t *testing.T) {
	client := New(Config{Enabled: true})
	assert.Equal(t, domain.SourceTypeOpenAlex, client.SourceType())
}

func TestClient_Name(t *testing.T) {
	client := New(Config{Enabled: true})
	assert.Equal(t, "OpenAlex", client.Name())
}

func TestClient_IsEnabled(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		client := New(Config{Enabled: true})
		assert.True(t, client.IsEnabled())
	})

	t.Run("disabled", func(t *testing.T) {
		client := New(Config{Enabled: false})
		assert.False(t, client.IsEnabled())
	})
}

func TestClient_Search(t *testing.T) {
	t.Run("successful search", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/works", r.URL.Path)
			assert.Equal(t, "CRISPR", r.URL.Query().Get("search"))
			assert.Equal(t, "test@example.com", r.URL.Query().Get("mailto"))

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleSearchResponse())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)
		params := papersources.SearchParams{
			Query:      "CRISPR",
			MaxResults: 25,
		}

		result, err := client.Search(context.Background(), params)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 2, len(result.Papers))
		assert.Equal(t, 2, result.TotalResults)
		assert.False(t, result.HasMore)
		assert.Equal(t, domain.SourceTypeOpenAlex, result.Source)
		assert.Greater(t, result.SearchDuration, time.Duration(0))

		// Verify first paper
		paper1 := result.Papers[0]
		assert.Equal(t, "doi:10.1038/nature12373", paper1.CanonicalID)
		assert.Equal(t, "CRISPR-Cas Systems for Editing, Regulating and Targeting Genomes", paper1.Title)
		assert.Equal(t, 2014, paper1.PublicationYear)
		assert.Equal(t, 5000, paper1.CitationCount)
		assert.True(t, paper1.OpenAccess)
		assert.Equal(t, "Nature Biotechnology", paper1.Journal)
		assert.Equal(t, 2, len(paper1.Authors))
		assert.Equal(t, "John Smith", paper1.Authors[0].Name)
		assert.Equal(t, "0000-0001-2345-6789", paper1.Authors[0].ORCID)
		assert.Equal(t, "MIT", paper1.Authors[0].Affiliation)
		assert.Equal(t, "https://europepmc.org/articles/pmc4022601?pdf=render", paper1.PDFURL)

		// Verify abstract reconstruction
		assert.Contains(t, paper1.Abstract, "CRISPR")
		assert.Contains(t, paper1.Abstract, "powerful")
		assert.Contains(t, paper1.Abstract, "tool")

		// Verify second paper
		paper2 := result.Papers[1]
		assert.Equal(t, "doi:10.1126/science.1234567", paper2.CanonicalID)
		assert.Equal(t, 2023, paper2.PublicationYear)
		assert.False(t, paper2.OpenAccess)
	})

	t.Run("search with pagination", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "2", r.URL.Query().Get("page"))
			assert.Equal(t, "10", r.URL.Query().Get("per_page"))

			resp := sampleSearchResponse()
			resp.Meta.Count = 100 // Total results
			resp.Meta.Page = 2
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)
		params := papersources.SearchParams{
			Query:      "gene therapy",
			MaxResults: 10,
			Offset:     10, // Second page
		}

		result, err := client.Search(context.Background(), params)
		require.NoError(t, err)

		assert.Equal(t, 100, result.TotalResults)
		assert.True(t, result.HasMore)
		assert.Equal(t, 12, result.NextOffset)
	})

	t.Run("empty search results", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := SearchResponse{
				Meta: Meta{
					Count:   0,
					Page:    1,
					PerPage: 25,
				},
				Results: []Work{},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)
		params := papersources.SearchParams{
			Query: "nonexistent topic xyz123",
		}

		result, err := client.Search(context.Background(), params)
		require.NoError(t, err)

		assert.Equal(t, 0, len(result.Papers))
		assert.Equal(t, 0, result.TotalResults)
		assert.False(t, result.HasMore)
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)

		// Override the HTTP client to have no retries for faster tests
		httpClient := papersources.NewHTTPClient(papersources.HTTPClientConfig{
			Timeout:    5 * time.Second,
			RateLimit:  100,
			BurstSize:  100,
			MaxRetries: 0,
		})
		client = NewWithHTTPClient(Config{
			BaseURL:    server.URL,
			MaxResults: 25,
			Enabled:    true,
		}, httpClient)

		params := papersources.SearchParams{
			Query: "CRISPR",
		}

		result, err := client.Search(context.Background(), params)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
			json.NewEncoder(w).Encode(sampleSearchResponse())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		params := papersources.SearchParams{
			Query: "CRISPR",
		}

		result, err := client.Search(ctx, params)
		require.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestClient_Search_WithFilters(t *testing.T) {
	t.Run("date range filter", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			filter := r.URL.Query().Get("filter")
			assert.Contains(t, filter, "from_publication_date:2020-01-01")
			assert.Contains(t, filter, "to_publication_date:2023-12-31")

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleSearchResponse())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)

		dateFrom := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		dateTo := time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)

		params := papersources.SearchParams{
			Query:    "CRISPR",
			DateFrom: &dateFrom,
			DateTo:   &dateTo,
		}

		result, err := client.Search(context.Background(), params)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("open access filter", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			filter := r.URL.Query().Get("filter")
			assert.Contains(t, filter, "is_oa:true")

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleSearchResponse())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)
		params := papersources.SearchParams{
			Query:          "CRISPR",
			OpenAccessOnly: true,
		}

		result, err := client.Search(context.Background(), params)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("minimum citations filter", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			filter := r.URL.Query().Get("filter")
			assert.Contains(t, filter, "cited_by_count:>99")

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleSearchResponse())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)
		params := papersources.SearchParams{
			Query:        "CRISPR",
			MinCitations: 100,
		}

		result, err := client.Search(context.Background(), params)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("exclude preprints filter", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			filter := r.URL.Query().Get("filter")
			assert.Contains(t, filter, "type:!preprint")

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleSearchResponse())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)
		params := papersources.SearchParams{
			Query:            "CRISPR",
			IncludePreprints: false,
		}

		result, err := client.Search(context.Background(), params)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("include preprints", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			filter := r.URL.Query().Get("filter")
			assert.NotContains(t, filter, "type:!preprint")

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleSearchResponse())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)
		params := papersources.SearchParams{
			Query:            "CRISPR",
			IncludePreprints: true,
		}

		result, err := client.Search(context.Background(), params)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("combined filters", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			filter := r.URL.Query().Get("filter")
			assert.Contains(t, filter, "from_publication_date:2022-01-01")
			assert.Contains(t, filter, "is_oa:true")
			assert.Contains(t, filter, "cited_by_count:>49")
			assert.Contains(t, filter, "type:!preprint")

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleSearchResponse())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)

		dateFrom := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)

		params := papersources.SearchParams{
			Query:            "CRISPR",
			DateFrom:         &dateFrom,
			OpenAccessOnly:   true,
			MinCitations:     50,
			IncludePreprints: false,
		}

		result, err := client.Search(context.Background(), params)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})
}

func TestClient_GetByID(t *testing.T) {
	t.Run("get by OpenAlex ID", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/works/W2741809807", r.URL.Path)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleWork())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)

		paper, err := client.GetByID(context.Background(), "W2741809807")
		require.NoError(t, err)
		require.NotNil(t, paper)

		assert.Equal(t, "doi:10.1038/nature12373", paper.CanonicalID)
		assert.Equal(t, "CRISPR-Cas Systems for Editing, Regulating and Targeting Genomes", paper.Title)
	})

	t.Run("get by full OpenAlex URL", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/works/W2741809807", r.URL.Path)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleWork())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)

		paper, err := client.GetByID(context.Background(), "https://openalex.org/W2741809807")
		require.NoError(t, err)
		require.NotNil(t, paper)
	})

	t.Run("get by DOI", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// DOI should be URL-encoded
			assert.Contains(t, r.URL.Path, "/works/")
			assert.Contains(t, r.URL.EscapedPath(), "10.1038")

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleWork())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)

		paper, err := client.GetByID(context.Background(), "10.1038/nature12373")
		require.NoError(t, err)
		require.NotNil(t, paper)
	})

	t.Run("get by full DOI URL", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleWork())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)

		paper, err := client.GetByID(context.Background(), "https://doi.org/10.1038/nature12373")
		require.NoError(t, err)
		require.NotNil(t, paper)
	})

	t.Run("get by canonical DOI format", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleWork())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)

		paper, err := client.GetByID(context.Background(), "doi:10.1038/nature12373")
		require.NoError(t, err)
		require.NotNil(t, paper)
	})

	t.Run("not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "Work not found"}`))
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)

		paper, err := client.GetByID(context.Background(), "W9999999999")
		require.Error(t, err)
		assert.Nil(t, paper)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		}))
		defer server.Close()

		// Use client with no retries
		httpClient := papersources.NewHTTPClient(papersources.HTTPClientConfig{
			Timeout:    5 * time.Second,
			RateLimit:  100,
			BurstSize:  100,
			MaxRetries: 0,
		})
		client := NewWithHTTPClient(Config{
			BaseURL:    server.URL,
			MaxResults: 25,
			Enabled:    true,
		}, httpClient)

		paper, err := client.GetByID(context.Background(), "W2741809807")
		require.Error(t, err)
		assert.Nil(t, paper)
	})

	t.Run("includes mailto parameter", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "test@example.com", r.URL.Query().Get("mailto"))

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleWork())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)

		paper, err := client.GetByID(context.Background(), "W2741809807")
		require.NoError(t, err)
		require.NotNil(t, paper)
	})
}

func TestClient_normalizeDOI(t *testing.T) {
	client := New(Config{Enabled: true})

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "https prefix",
			input:    "https://doi.org/10.1038/nature12373",
			expected: "10.1038/nature12373",
		},
		{
			name:     "http prefix",
			input:    "http://doi.org/10.1038/nature12373",
			expected: "10.1038/nature12373",
		},
		{
			name:     "doi prefix",
			input:    "doi:10.1038/nature12373",
			expected: "10.1038/nature12373",
		},
		{
			name:     "no prefix",
			input:    "10.1038/nature12373",
			expected: "10.1038/nature12373",
		},
		{
			name:     "uppercase DOI",
			input:    "https://doi.org/10.1038/NATURE12373",
			expected: "10.1038/nature12373",
		},
		{
			name:     "with whitespace",
			input:    "  https://doi.org/10.1038/nature12373  ",
			expected: "10.1038/nature12373",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := client.normalizeDOI(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestClient_normalizeOpenAlexID(t *testing.T) {
	client := New(Config{Enabled: true})

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "full URL",
			input:    "https://openalex.org/W2741809807",
			expected: "W2741809807",
		},
		{
			name:     "short ID",
			input:    "W2741809807",
			expected: "W2741809807",
		},
		{
			name:     "with whitespace",
			input:    "  W2741809807  ",
			expected: "W2741809807",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := client.normalizeOpenAlexID(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestClient_reconstructAbstract(t *testing.T) {
	client := New(Config{Enabled: true})

	t.Run("empty index", func(t *testing.T) {
		result := client.reconstructAbstract(nil)
		assert.Equal(t, "", result)

		result = client.reconstructAbstract(map[string][]int{})
		assert.Equal(t, "", result)
	})

	t.Run("simple abstract", func(t *testing.T) {
		index := map[string][]int{
			"Hello":  {0},
			"world!": {1},
		}
		result := client.reconstructAbstract(index)
		assert.Equal(t, "Hello world!", result)
	})

	t.Run("word appearing multiple times", func(t *testing.T) {
		index := map[string][]int{
			"the":  {0, 2},
			"cat":  {1},
			"sat.": {3},
		}
		result := client.reconstructAbstract(index)
		assert.Equal(t, "the cat the sat.", result)
	})

	t.Run("complex abstract", func(t *testing.T) {
		index := map[string][]int{
			"CRISPR":   {0},
			"is":       {1},
			"a":        {2},
			"powerful": {3},
			"tool":     {4},
			"for":      {5},
			"genome":   {6},
			"editing.": {7},
		}
		result := client.reconstructAbstract(index)
		assert.Equal(t, "CRISPR is a powerful tool for genome editing.", result)
	})
}

func TestClient_workToPaper(t *testing.T) {
	client := New(Config{Enabled: true})

	t.Run("complete work", func(t *testing.T) {
		work := sampleWork()
		paper := client.workToPaper(&work)

		require.NotNil(t, paper)
		assert.Equal(t, "doi:10.1038/nature12373", paper.CanonicalID)
		assert.Equal(t, "CRISPR-Cas Systems for Editing, Regulating and Targeting Genomes", paper.Title)
		assert.Equal(t, 2014, paper.PublicationYear)
		assert.NotNil(t, paper.PublicationDate)
		assert.Equal(t, 5000, paper.CitationCount)
		assert.Equal(t, 2, paper.ReferenceCount)
		assert.True(t, paper.OpenAccess)
		assert.Equal(t, "Nature Biotechnology", paper.Journal)
		assert.Equal(t, "https://europepmc.org/articles/pmc4022601?pdf=render", paper.PDFURL)

		// Authors
		require.Len(t, paper.Authors, 2)
		assert.Equal(t, "John Smith", paper.Authors[0].Name)
		assert.Equal(t, "0000-0001-2345-6789", paper.Authors[0].ORCID)
		assert.Equal(t, "MIT", paper.Authors[0].Affiliation)
		assert.Equal(t, "Jane Doe", paper.Authors[1].Name)
		assert.Empty(t, paper.Authors[1].ORCID)
		assert.Empty(t, paper.Authors[1].Affiliation)

		// Raw metadata
		assert.NotNil(t, paper.RawMetadata)
		assert.Equal(t, "W2741809807", paper.RawMetadata["openalex_id"])
	})

	t.Run("work without DOI uses OpenAlex ID", func(t *testing.T) {
		work := Work{
			ID:              "https://openalex.org/W123456789",
			Title:           "Paper Without DOI",
			DisplayName:     "Paper Without DOI",
			PublicationYear: 2020,
			IDs: IDs{
				OpenAlex: "https://openalex.org/W123456789",
			},
		}

		paper := client.workToPaper(&work)

		require.NotNil(t, paper)
		assert.Equal(t, "openalex:W123456789", paper.CanonicalID)
	})

	t.Run("work without any identifier returns nil", func(t *testing.T) {
		work := Work{
			Title:           "No Identifiers",
			DisplayName:     "No Identifiers",
			PublicationYear: 2020,
		}

		paper := client.workToPaper(&work)
		assert.Nil(t, paper)
	})

	t.Run("nil work", func(t *testing.T) {
		paper := client.workToPaper(nil)
		assert.Nil(t, paper)
	})

	t.Run("work with PDF URL in primary location", func(t *testing.T) {
		work := Work{
			ID:              "https://openalex.org/W123",
			DOI:             "https://doi.org/10.1234/test",
			Title:           "Test Paper",
			DisplayName:     "Test Paper",
			PublicationYear: 2023,
			PrimaryLocation: &Location{
				PDFURL: "https://example.com/paper.pdf",
				Source: &Source{
					DisplayName: "Test Journal",
				},
			},
			IDs: IDs{
				OpenAlex: "https://openalex.org/W123",
				DOI:      "https://doi.org/10.1234/test",
			},
		}

		paper := client.workToPaper(&work)
		require.NotNil(t, paper)
		assert.Equal(t, "https://example.com/paper.pdf", paper.PDFURL)
	})

	t.Run("prefers OpenAccess URL over primary location PDF", func(t *testing.T) {
		work := Work{
			ID:              "https://openalex.org/W123",
			DOI:             "https://doi.org/10.1234/test",
			Title:           "Test Paper",
			DisplayName:     "Test Paper",
			PublicationYear: 2023,
			OpenAccess: &OpenAccess{
				IsOA:  true,
				OAURL: "https://oa.example.com/paper.pdf",
			},
			PrimaryLocation: &Location{
				PDFURL: "https://example.com/paper.pdf",
			},
			IDs: IDs{
				OpenAlex: "https://openalex.org/W123",
				DOI:      "https://doi.org/10.1234/test",
			},
		}

		paper := client.workToPaper(&work)
		require.NotNil(t, paper)
		assert.Equal(t, "https://oa.example.com/paper.pdf", paper.PDFURL)
	})
}

func TestClient_InterfaceImplementation(t *testing.T) {
	// Verify that Client implements the PaperSource interface
	var _ papersources.PaperSource = (*Client)(nil)

	client := New(Config{Enabled: true})

	// Call all interface methods to ensure they are implemented
	_ = client.SourceType()
	_ = client.Name()
	_ = client.IsEnabled()

	// Search and GetByID require a server, tested separately
}

func TestConfig_applyDefaults(t *testing.T) {
	t.Run("applies all defaults", func(t *testing.T) {
		cfg := Config{}
		cfg.applyDefaults()

		assert.Equal(t, DefaultBaseURL, cfg.BaseURL)
		assert.Equal(t, DefaultTimeout, cfg.Timeout)
		assert.Equal(t, DefaultRateLimit, cfg.RateLimit)
		assert.Equal(t, DefaultBurstSize, cfg.BurstSize)
		assert.Equal(t, DefaultMaxResults, cfg.MaxResults)
	})

	t.Run("does not override set values", func(t *testing.T) {
		cfg := Config{
			BaseURL:    "https://custom.api.org",
			Timeout:    60 * time.Second,
			RateLimit:  20.0,
			BurstSize:  20,
			MaxResults: 50,
		}
		cfg.applyDefaults()

		assert.Equal(t, "https://custom.api.org", cfg.BaseURL)
		assert.Equal(t, 60*time.Second, cfg.Timeout)
		assert.Equal(t, 20.0, cfg.RateLimit)
		assert.Equal(t, 20, cfg.BurstSize)
		assert.Equal(t, 50, cfg.MaxResults)
	})
}

func TestClient_buildSearchURL(t *testing.T) {
	t.Run("basic search URL", func(t *testing.T) {
		client := newTestClient("https://api.openalex.org", true)
		params := papersources.SearchParams{
			Query: "CRISPR",
		}

		url, err := client.buildSearchURL(params)
		require.NoError(t, err)

		assert.Contains(t, url, "https://api.openalex.org/works")
		assert.Contains(t, url, "search=CRISPR")
		assert.Contains(t, url, "mailto=test%40example.com")
	})

	t.Run("URL with max results cap", func(t *testing.T) {
		client := newTestClient("https://api.openalex.org", true)
		params := papersources.SearchParams{
			Query:      "CRISPR",
			MaxResults: 500, // Over the 200 limit
		}

		url, err := client.buildSearchURL(params)
		require.NoError(t, err)

		assert.Contains(t, url, "per_page=200") // Should be capped at 200
	})

	t.Run("URL without email", func(t *testing.T) {
		cfg := Config{
			BaseURL:    "https://api.openalex.org",
			MaxResults: 25,
			Enabled:    true,
		}
		httpClient := papersources.NewHTTPClient(papersources.HTTPClientConfig{
			RateLimit: 100,
			BurstSize: 100,
		})
		client := NewWithHTTPClient(cfg, httpClient)

		params := papersources.SearchParams{
			Query: "CRISPR",
		}

		url, err := client.buildSearchURL(params)
		require.NoError(t, err)

		assert.NotContains(t, url, "mailto")
	})
}

func TestClient_buildFilters(t *testing.T) {
	client := New(Config{Enabled: true})

	t.Run("no filters with preprints included", func(t *testing.T) {
		params := papersources.SearchParams{
			Query:            "CRISPR",
			IncludePreprints: true, // Must be set to true to have no filters
		}
		filters := client.buildFilters(params)
		assert.Empty(t, filters)
	})

	t.Run("default excludes preprints", func(t *testing.T) {
		params := papersources.SearchParams{
			Query: "CRISPR",
			// IncludePreprints defaults to false, so preprints are excluded
		}
		filters := client.buildFilters(params)
		assert.Len(t, filters, 1)
		assert.Contains(t, filters, "type:!preprint")
	})

	t.Run("date from filter", func(t *testing.T) {
		date := time.Date(2020, 6, 15, 0, 0, 0, 0, time.UTC)
		params := papersources.SearchParams{
			DateFrom: &date,
		}
		filters := client.buildFilters(params)
		assert.Contains(t, filters, "from_publication_date:2020-06-15")
	})

	t.Run("date to filter", func(t *testing.T) {
		date := time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)
		params := papersources.SearchParams{
			DateTo: &date,
		}
		filters := client.buildFilters(params)
		assert.Contains(t, filters, "to_publication_date:2023-12-31")
	})

	t.Run("open access filter", func(t *testing.T) {
		params := papersources.SearchParams{
			OpenAccessOnly: true,
		}
		filters := client.buildFilters(params)
		assert.Contains(t, filters, "is_oa:true")
	})

	t.Run("min citations filter", func(t *testing.T) {
		params := papersources.SearchParams{
			MinCitations: 100,
		}
		filters := client.buildFilters(params)
		assert.Contains(t, filters, "cited_by_count:>99")
	})

	t.Run("exclude preprints filter", func(t *testing.T) {
		params := papersources.SearchParams{
			IncludePreprints: false,
		}
		filters := client.buildFilters(params)
		assert.Contains(t, filters, "type:!preprint")
	})

	t.Run("include preprints has no filter", func(t *testing.T) {
		params := papersources.SearchParams{
			IncludePreprints: true,
		}
		filters := client.buildFilters(params)
		for _, f := range filters {
			assert.NotContains(t, f, "type:")
		}
	})
}

func TestClient_buildGetByIDURL(t *testing.T) {
	client := newTestClient("https://api.openalex.org", true)

	testCases := []struct {
		name         string
		id           string
		expectedPath string
	}{
		{
			name:         "short OpenAlex ID",
			id:           "W2741809807",
			expectedPath: "/works/W2741809807",
		},
		{
			name:         "full OpenAlex URL",
			id:           "https://openalex.org/W2741809807",
			expectedPath: "/works/W2741809807",
		},
		{
			name:         "short DOI",
			id:           "10.1038/nature12373",
			expectedPath: "/works/https://doi.org/10.1038/nature12373",
		},
		{
			name:         "full DOI URL",
			id:           "https://doi.org/10.1038/nature12373",
			expectedPath: "/works/https://doi.org/10.1038/nature12373",
		},
		{
			name:         "canonical DOI format",
			id:           "doi:10.1038/nature12373",
			expectedPath: "/works/https://doi.org/10.1038/nature12373",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url, err := client.buildGetByIDURL(tc.id)
			require.NoError(t, err)
			assert.Contains(t, url, tc.expectedPath)
			assert.Contains(t, url, "mailto=test%40example.com")
		})
	}
}

func TestClient_normalizeORCID(t *testing.T) {
	client := New(Config{Enabled: true})

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "full URL",
			input:    "https://orcid.org/0000-0001-2345-6789",
			expected: "0000-0001-2345-6789",
		},
		{
			name:     "short ORCID",
			input:    "0000-0001-2345-6789",
			expected: "0000-0001-2345-6789",
		},
		{
			name:     "with whitespace",
			input:    "  0000-0001-2345-6789  ",
			expected: "0000-0001-2345-6789",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := client.normalizeORCID(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestClient_normalizePMID(t *testing.T) {
	client := New(Config{Enabled: true})

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "full URL",
			input:    "https://pubmed.ncbi.nlm.nih.gov/24906146",
			expected: "24906146",
		},
		{
			name:     "short PMID",
			input:    "24906146",
			expected: "24906146",
		},
		{
			name:     "with whitespace",
			input:    "  24906146  ",
			expected: "24906146",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := client.normalizePMID(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMaxResultsCapping(t *testing.T) {
	t.Run("respects 200 page limit", func(t *testing.T) {
		receivedPerPage := ""
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedPerPage = r.URL.Query().Get("per_page")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleSearchResponse())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)
		params := papersources.SearchParams{
			Query:      "CRISPR",
			MaxResults: 500, // Over API limit
		}

		_, err := client.Search(context.Background(), params)
		require.NoError(t, err)

		assert.Equal(t, "200", receivedPerPage)
	})
}

// Test that query parameters with special characters are properly encoded
func TestQueryParameterEncoding(t *testing.T) {
	t.Run("encodes special characters in search query", func(t *testing.T) {
		var receivedQuery string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedQuery = r.URL.Query().Get("search")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleSearchResponse())
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)
		params := papersources.SearchParams{
			Query: "CRISPR-Cas9 & gene editing",
		}

		_, err := client.Search(context.Background(), params)
		require.NoError(t, err)

		// The URL query parsing should properly decode it
		assert.Equal(t, "CRISPR-Cas9 & gene editing", receivedQuery)
	})
}

// Test handling of malformed JSON response
func TestMalformedJSONResponse(t *testing.T) {
	t.Run("returns error on malformed JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{invalid json`))
		}))
		defer server.Close()

		client := newTestClient(server.URL, true)
		params := papersources.SearchParams{
			Query: "CRISPR",
		}

		result, err := client.Search(context.Background(), params)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, strings.ToLower(err.Error()), "decoding")
	})
}
