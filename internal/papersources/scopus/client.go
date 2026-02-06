package scopus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/papersources"
)

const (
	// DefaultBaseURL is the default Scopus API base URL.
	DefaultBaseURL = "https://api.elsevier.com/content"

	// DefaultRateLimit is the default rate limit (5 requests per second).
	DefaultRateLimit = 5.0

	// DefaultBurstSize is the default burst size for rate limiting.
	DefaultBurstSize = 5

	// DefaultTimeout is the default request timeout.
	DefaultTimeout = 30 * time.Second

	// DefaultMaxResults is the default maximum results per request.
	DefaultMaxResults = 25

	// apiKeyHeader is the HTTP header name for the Scopus API key.
	apiKeyHeader = "X-ELS-APIKey"

	// sourceName is the human-readable name for this source.
	sourceName = "Scopus"
)

// Config holds configuration for the Scopus client.
type Config struct {
	// BaseURL is the Scopus API base URL.
	BaseURL string

	// APIKey is the Elsevier API key for authentication.
	// Required for all Scopus API requests.
	APIKey string

	// Timeout is the request timeout.
	Timeout time.Duration

	// RateLimit is the maximum requests per second.
	RateLimit float64

	// BurstSize is the maximum burst of requests allowed.
	BurstSize int

	// MaxResults is the maximum results to return per search request.
	MaxResults int

	// Enabled indicates whether this source is enabled for searches.
	Enabled bool
}

// applyDefaults sets default values for unset configuration fields.
func (c *Config) applyDefaults() {
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	if c.Timeout == 0 {
		c.Timeout = DefaultTimeout
	}
	if c.RateLimit == 0 {
		c.RateLimit = DefaultRateLimit
	}
	if c.BurstSize == 0 {
		c.BurstSize = DefaultBurstSize
	}
	if c.MaxResults == 0 {
		c.MaxResults = DefaultMaxResults
	}
}

// Client implements the papersources.PaperSource interface for Scopus.
type Client struct {
	config     Config
	httpClient *papersources.HTTPClient
}

// Ensure Client implements PaperSource interface.
var _ papersources.PaperSource = (*Client)(nil)

// New creates a new Scopus client with the given configuration.
func New(cfg Config) *Client {
	cfg.applyDefaults()

	httpClient := papersources.NewHTTPClient(papersources.HTTPClientConfig{
		Timeout:      cfg.Timeout,
		RateLimit:    cfg.RateLimit,
		BurstSize:    cfg.BurstSize,
		UserAgent:    "Helixir-LiteratureService/1.0",
		APIKey:       cfg.APIKey,
		APIKeyHeader: apiKeyHeader,
	})

	return &Client{
		config:     cfg,
		httpClient: httpClient,
	}
}

// NewWithHTTPClient creates a new Scopus client with a custom HTTP client.
// This is useful for testing with mock servers.
func NewWithHTTPClient(cfg Config, httpClient *papersources.HTTPClient) *Client {
	cfg.applyDefaults()

	return &Client{
		config:     cfg,
		httpClient: httpClient,
	}
}

// Search queries Scopus for papers matching the given parameters.
func (c *Client) Search(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
	startTime := time.Now()

	searchURL, err := c.buildSearchURL(params)
	if err != nil {
		return nil, fmt.Errorf("building search URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, domain.NewExternalAPIError(
			sourceName,
			resp.StatusCode,
			string(body),
			nil,
		)
	}

	// Parse the JSON response (limit body to 10MB).
	var searchResp SearchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	papers := make([]*domain.Paper, 0, len(searchResp.SearchResults.Entries))
	for i := range searchResp.SearchResults.Entries {
		paper := c.entryToPaper(&searchResp.SearchResults.Entries[i])
		if paper != nil {
			papers = append(papers, paper)
		}
	}

	totalResults, _ := strconv.Atoi(searchResp.SearchResults.TotalResults)
	nextOffset := params.Offset + len(papers)
	hasMore := nextOffset < totalResults

	return &papersources.SearchResult{
		Papers:         papers,
		TotalResults:   totalResults,
		HasMore:        hasMore,
		NextOffset:     nextOffset,
		Source:         domain.SourceTypeScopus,
		SearchDuration: time.Since(startTime),
	}, nil
}

// GetByID retrieves a specific paper by DOI or EID from Scopus.
func (c *Client) GetByID(ctx context.Context, id string) (*domain.Paper, error) {
	baseURL, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing base URL: %w", err)
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/search/scopus"

	query := url.Values{}
	// Determine if the id is a DOI or EID
	if strings.HasPrefix(id, "10.") || strings.HasPrefix(id, "doi:") {
		doi := strings.TrimPrefix(id, "doi:")
		query.Set("query", fmt.Sprintf("DOI(%s)", doi))
	} else {
		query.Set("query", fmt.Sprintf("EID(%s)", id))
	}
	query.Set("view", "COMPLETE")
	baseURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.NewNotFoundError("paper", id)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, domain.NewExternalAPIError(
			sourceName,
			resp.StatusCode,
			string(body),
			nil,
		)
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(searchResp.SearchResults.Entries) == 0 {
		return nil, domain.NewNotFoundError("paper", id)
	}

	paper := c.entryToPaper(&searchResp.SearchResults.Entries[0])
	if paper == nil {
		return nil, domain.NewNotFoundError("paper", id)
	}

	return paper, nil
}

// SourceType returns the source type identifier.
func (c *Client) SourceType() domain.SourceType {
	return domain.SourceTypeScopus
}

// Name returns the human-readable name for this source.
func (c *Client) Name() string {
	return sourceName
}

// IsEnabled returns whether this source is enabled.
// Scopus requires an API key, so it returns false if the key is empty.
func (c *Client) IsEnabled() bool {
	return c.config.Enabled && c.config.APIKey != ""
}

// buildSearchURL constructs the Scopus search API URL.
func (c *Client) buildSearchURL(params papersources.SearchParams) (string, error) {
	baseURL, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return "", fmt.Errorf("parsing base URL: %w", err)
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/search/scopus"

	queryParts := []string{fmt.Sprintf("TITLE-ABS-KEY(%s)", params.Query)}

	// Date filter (year-based ranges)
	if params.DateFrom != nil && params.DateTo != nil {
		queryParts = append(queryParts, fmt.Sprintf("PUBYEAR > %d AND PUBYEAR < %d",
			params.DateFrom.Year()-1, params.DateTo.Year()+1))
	} else if params.DateFrom != nil {
		queryParts = append(queryParts, fmt.Sprintf("PUBYEAR > %d", params.DateFrom.Year()-1))
	} else if params.DateTo != nil {
		queryParts = append(queryParts, fmt.Sprintf("PUBYEAR < %d", params.DateTo.Year()+1))
	}

	// Open access filter
	if params.OpenAccessOnly {
		queryParts = append(queryParts, "OPENACCESS(1)")
	}

	urlQuery := url.Values{}
	urlQuery.Set("query", strings.Join(queryParts, " AND "))
	urlQuery.Set("view", "COMPLETE")

	// Pagination
	maxResults := params.MaxResults
	if maxResults == 0 {
		maxResults = c.config.MaxResults
	}
	urlQuery.Set("count", strconv.Itoa(maxResults))

	if params.Offset > 0 {
		urlQuery.Set("start", strconv.Itoa(params.Offset))
	}

	baseURL.RawQuery = urlQuery.Encode()
	return baseURL.String(), nil
}

// entryToPaper converts a Scopus entry to a domain Paper.
func (c *Client) entryToPaper(entry *Entry) *domain.Paper {
	if entry == nil {
		return nil
	}

	// Extract Scopus ID from dc:identifier (strip "SCOPUS_ID:" prefix)
	scopusID := strings.TrimPrefix(entry.Identifier, "SCOPUS_ID:")
	scopusID = strings.TrimSpace(scopusID)

	doi := strings.TrimSpace(entry.DOI)
	pubmedID := strings.TrimSpace(entry.PubMedID)

	// Generate canonical ID
	canonicalID := domain.GenerateCanonicalID(domain.PaperIdentifiers{
		DOI:      doi,
		ScopusID: scopusID,
		PubMedID: pubmedID,
	})

	if canonicalID == "" {
		return nil
	}

	// Parse publication date
	var pubDate *time.Time
	var pubYear int
	if entry.CoverDate != "" {
		if t, err := time.Parse("2006-01-02", entry.CoverDate); err == nil {
			pubDate = &t
			pubYear = t.Year()
		}
	}

	// Extract authors from COMPLETE view, fallback to dc:creator
	authors := c.extractAuthors(entry)

	// Parse citation count
	citationCount, _ := strconv.Atoi(entry.CitedByCount)

	rawMetadata := map[string]interface{}{
		"scopus_id": scopusID,
		"eid":       entry.EID,
	}
	if doi != "" {
		rawMetadata["doi"] = doi
	}
	if pubmedID != "" {
		rawMetadata["pubmed_id"] = pubmedID
	}
	if entry.SubType != "" {
		rawMetadata["subtype"] = entry.SubType
	}
	if entry.Aggregation != "" {
		rawMetadata["aggregation_type"] = entry.Aggregation
	}

	return &domain.Paper{
		CanonicalID:     canonicalID,
		Title:           strings.TrimSpace(entry.Title),
		Abstract:        strings.TrimSpace(entry.Description),
		Authors:         authors,
		PublicationDate: pubDate,
		PublicationYear: pubYear,
		Journal:         strings.TrimSpace(entry.PublicationName),
		Venue:           strings.TrimSpace(entry.PublicationName),
		Volume:          strings.TrimSpace(entry.Volume),
		Issue:           strings.TrimSpace(entry.IssueID),
		Pages:           strings.TrimSpace(entry.PageRange),
		CitationCount:   citationCount,
		OpenAccess:      entry.OpenAccessFlag,
		RawMetadata:     rawMetadata,
	}
}

// extractAuthors extracts authors from the Scopus entry.
// Uses the COMPLETE view author list when available, otherwise falls back to dc:creator.
func (c *Client) extractAuthors(entry *Entry) []domain.Author {
	if entry.Authors != nil && len(entry.Authors.Authors) > 0 {
		authors := make([]domain.Author, 0, len(entry.Authors.Authors))
		for _, sa := range entry.Authors.Authors {
			name := strings.TrimSpace(sa.Name)
			if name == "" {
				// Build name from parts
				if sa.GivenName != "" && sa.Surname != "" {
					name = sa.GivenName + " " + sa.Surname
				} else if sa.Surname != "" {
					name = sa.Surname
				}
			}
			if name == "" {
				continue
			}
			authors = append(authors, domain.Author{
				Name:  name,
				ORCID: strings.TrimSpace(sa.ORCID),
			})
		}
		return authors
	}

	// Fallback to dc:creator (first author only)
	if creator := strings.TrimSpace(entry.Creator); creator != "" {
		return []domain.Author{{Name: creator}}
	}

	return nil
}
