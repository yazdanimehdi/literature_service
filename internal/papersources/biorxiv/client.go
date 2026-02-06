package biorxiv

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
	// DefaultBaseURL is the default Europe PMC API base URL.
	DefaultBaseURL = "https://www.ebi.ac.uk/europepmc/webservices/rest"

	// DefaultRateLimit is the default rate limit (5 requests per second).
	DefaultRateLimit = 5.0

	// DefaultBurstSize is the default burst size for rate limiting.
	DefaultBurstSize = 5

	// DefaultTimeout is the default request timeout.
	DefaultTimeout = 30 * time.Second

	// DefaultMaxResults is the default maximum results per request.
	DefaultMaxResults = 100
)

// Config holds configuration for the bioRxiv/medRxiv client.
type Config struct {
	// BaseURL is the Europe PMC API base URL.
	BaseURL string

	// Server is the preprint server name ("bioRxiv" or "medRxiv").
	// Used in the PUBLISHER filter for Europe PMC queries.
	Server string

	// SourceType is the domain source type (SourceTypeBioRxiv or SourceTypeMedRxiv).
	SourceType domain.SourceType

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
	if c.Server == "" {
		c.Server = "bioRxiv"
	}
	if c.SourceType == "" {
		c.SourceType = domain.SourceTypeBioRxiv
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

// Client implements the papersources.PaperSource interface for bioRxiv/medRxiv
// using the Europe PMC API as a proxy.
type Client struct {
	config     Config
	httpClient *papersources.HTTPClient
}

// Ensure Client implements PaperSource interface.
var _ papersources.PaperSource = (*Client)(nil)

// New creates a new bioRxiv/medRxiv client with the given configuration.
func New(cfg Config) *Client {
	cfg.applyDefaults()

	httpClient := papersources.NewHTTPClient(papersources.HTTPClientConfig{
		Timeout:   cfg.Timeout,
		RateLimit: cfg.RateLimit,
		BurstSize: cfg.BurstSize,
		UserAgent: "Helixir-LiteratureService/1.0",
	})

	return &Client{
		config:     cfg,
		httpClient: httpClient,
	}
}

// NewWithHTTPClient creates a new bioRxiv/medRxiv client with a custom HTTP client.
// This is useful for testing with mock servers.
func NewWithHTTPClient(cfg Config, httpClient *papersources.HTTPClient) *Client {
	cfg.applyDefaults()

	return &Client{
		config:     cfg,
		httpClient: httpClient,
	}
}

// Search queries Europe PMC for bioRxiv/medRxiv papers matching the given parameters.
func (c *Client) Search(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
	startTime := time.Now()

	searchURL, err := c.buildSearchURL(params, "*")
	if err != nil {
		return nil, fmt.Errorf("building search URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, domain.NewExternalAPIError(
			c.config.Server,
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

	papers := make([]*domain.Paper, 0, len(searchResp.ResultList.Result))
	for i := range searchResp.ResultList.Result {
		paper := c.articleToPaper(&searchResp.ResultList.Result[i])
		if paper != nil {
			papers = append(papers, paper)
		}
	}

	// Europe PMC uses cursor-based pagination
	hasMore := searchResp.NextCursorMark != "" && len(searchResp.ResultList.Result) > 0
	nextOffset := params.Offset + len(papers)

	return &papersources.SearchResult{
		Papers:         papers,
		TotalResults:   searchResp.HitCount,
		HasMore:        hasMore,
		NextOffset:     nextOffset,
		Source:         c.config.SourceType,
		SearchDuration: time.Since(startTime),
	}, nil
}

// GetByID retrieves a specific paper by DOI from Europe PMC.
func (c *Client) GetByID(ctx context.Context, id string) (*domain.Paper, error) {
	baseURL, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing base URL: %w", err)
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/search"

	// Strip doi: prefix if present
	doi := strings.TrimPrefix(id, "doi:")

	query := url.Values{}
	query.Set("query", fmt.Sprintf("DOI:%s AND (SRC:PPR)", doi))
	query.Set("format", "json")
	query.Set("resultType", "core")
	query.Set("pageSize", "1")
	baseURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, domain.NewExternalAPIError(
			c.config.Server,
			resp.StatusCode,
			string(body),
			nil,
		)
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(searchResp.ResultList.Result) == 0 {
		return nil, domain.NewNotFoundError("paper", id)
	}

	paper := c.articleToPaper(&searchResp.ResultList.Result[0])
	if paper == nil {
		return nil, domain.NewNotFoundError("paper", id)
	}

	return paper, nil
}

// SourceType returns the source type identifier.
func (c *Client) SourceType() domain.SourceType {
	return c.config.SourceType
}

// Name returns the human-readable name for this source.
func (c *Client) Name() string {
	return c.config.Server
}

// IsEnabled returns whether this source is enabled.
func (c *Client) IsEnabled() bool {
	return c.config.Enabled
}

// buildSearchURL constructs the Europe PMC search API URL.
func (c *Client) buildSearchURL(params papersources.SearchParams, cursorMark string) (string, error) {
	baseURL, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return "", fmt.Errorf("parsing base URL: %w", err)
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/search"

	// Build query: {query} AND (SRC:PPR) AND (PUBLISHER:"{server}")
	queryParts := []string{
		params.Query,
		"(SRC:PPR)",
		fmt.Sprintf(`(PUBLISHER:"%s")`, c.config.Server),
	}

	// Add date filter
	if params.DateFrom != nil || params.DateTo != nil {
		dateFilter := c.buildDateFilter(params.DateFrom, params.DateTo)
		if dateFilter != "" {
			queryParts = append(queryParts, dateFilter)
		}
	}

	urlQuery := url.Values{}
	urlQuery.Set("query", strings.Join(queryParts, " AND "))
	urlQuery.Set("format", "json")
	urlQuery.Set("resultType", "core")

	maxResults := params.MaxResults
	if maxResults == 0 {
		maxResults = c.config.MaxResults
	}
	urlQuery.Set("pageSize", strconv.Itoa(maxResults))
	urlQuery.Set("cursorMark", cursorMark)

	baseURL.RawQuery = urlQuery.Encode()
	return baseURL.String(), nil
}

// buildDateFilter constructs the Europe PMC date filter string.
func (c *Client) buildDateFilter(from, to *time.Time) string {
	var fromStr, toStr string

	if from != nil {
		fromStr = from.Format("2006-01-02")
	} else {
		fromStr = "*"
	}

	if to != nil {
		toStr = to.Format("2006-01-02")
	} else {
		toStr = "*"
	}

	return fmt.Sprintf("(FIRST_PDATE:[%s TO %s])", fromStr, toStr)
}

// articleToPaper converts a Europe PMC Article to a domain Paper.
func (c *Client) articleToPaper(article *Article) *domain.Paper {
	if article == nil {
		return nil
	}

	doi := strings.TrimSpace(article.DOI)
	pmid := strings.TrimSpace(article.PMID)
	pmcid := strings.TrimSpace(article.PMCID)

	// Generate canonical ID
	canonicalID := domain.GenerateCanonicalID(domain.PaperIdentifiers{
		DOI:      doi,
		PubMedID: pmid,
		PMCID:    pmcid,
	})

	if canonicalID == "" {
		return nil
	}

	// Parse publication date
	var pubDate *time.Time
	var pubYear int
	if article.FirstPublicationDate != "" {
		if t, err := time.Parse("2006-01-02", article.FirstPublicationDate); err == nil {
			pubDate = &t
			pubYear = t.Year()
		}
	}
	if pubYear == 0 && article.PubYear != "" {
		pubYear, _ = strconv.Atoi(article.PubYear)
	}

	// Parse authors from authorString (comma-separated)
	authors := parseAuthorString(article.AuthorString)

	// Determine open access status (preprints are generally open access)
	openAccess := true
	if article.IsOpenAccess == "N" {
		openAccess = false
	}

	// Build PDF URL based on server
	var pdfURL string
	if doi != "" {
		switch strings.ToLower(c.config.Server) {
		case "medrxiv":
			pdfURL = "https://www.medrxiv.org/content/" + doi + ".full.pdf"
		default:
			pdfURL = "https://www.biorxiv.org/content/" + doi + ".full.pdf"
		}
	}

	rawMetadata := map[string]interface{}{
		"source":    article.Source,
		"publisher": article.PublisherName,
	}
	if doi != "" {
		rawMetadata["doi"] = doi
	}
	if pmid != "" {
		rawMetadata["pmid"] = pmid
	}
	if pmcid != "" {
		rawMetadata["pmcid"] = pmcid
	}

	return &domain.Paper{
		CanonicalID:     canonicalID,
		Title:           strings.TrimSpace(article.Title),
		Abstract:        strings.TrimSpace(article.AbstractText),
		Authors:         authors,
		PublicationDate: pubDate,
		PublicationYear: pubYear,
		Journal:         strings.TrimSpace(article.JournalTitle),
		Venue:           strings.TrimSpace(article.JournalTitle),
		Volume:          strings.TrimSpace(article.JournalVolume),
		CitationCount:   article.CitedByCount,
		PDFURL:          pdfURL,
		OpenAccess:      openAccess,
		RawMetadata:     rawMetadata,
	}
}

// parseAuthorString parses the Europe PMC authorString field.
// The format is "Author A, Author B, Author C" where each entry is a single author name.
// Authors are separated by ", " but author names themselves may contain commas
// in "Surname, GivenName" format. Europe PMC uses "GivenName Surname" format
// with authors separated by ", ".
func parseAuthorString(authorString string) []domain.Author {
	authorString = strings.TrimSpace(authorString)
	// Remove trailing period if present
	authorString = strings.TrimSuffix(authorString, ".")
	if authorString == "" {
		return nil
	}

	parts := strings.Split(authorString, ", ")
	authors := make([]domain.Author, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		authors = append(authors, domain.Author{Name: name})
	}

	return authors
}
