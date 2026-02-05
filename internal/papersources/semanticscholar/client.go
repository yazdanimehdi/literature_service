package semanticscholar

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
	// DefaultBaseURL is the default base URL for the Semantic Scholar Graph API.
	DefaultBaseURL = "https://api.semanticscholar.org/graph/v1"

	// DefaultRateLimit is the default rate limit for unauthenticated requests (100 req/5 min).
	// With an API key, this can be increased.
	DefaultRateLimit = 10.0

	// DefaultBurstSize is the default burst size for rate limiting.
	DefaultBurstSize = 10

	// DefaultTimeout is the default HTTP request timeout.
	DefaultTimeout = 30 * time.Second

	// DefaultMaxResults is the default maximum number of results per request.
	DefaultMaxResults = 100

	// apiKeyHeader is the header name for the Semantic Scholar API key.
	apiKeyHeader = "x-api-key"

	// paperFields is the list of fields to request from the API.
	paperFields = "paperId,externalIds,title,abstract,year,publicationDate,venue,journal,authors,citationCount,referenceCount,isOpenAccess,openAccessPdf"

	// sourceName is the human-readable name for this source.
	sourceName = "Semantic Scholar"
)

// Config contains configuration options for the Semantic Scholar client.
type Config struct {
	// BaseURL is the base URL for the API.
	// Defaults to DefaultBaseURL if empty.
	BaseURL string

	// APIKey is the optional API key for authenticated requests.
	// Authenticated requests have higher rate limits.
	APIKey string

	// Timeout is the HTTP request timeout.
	// Defaults to DefaultTimeout if zero.
	Timeout time.Duration

	// RateLimit is the maximum requests per second.
	// Defaults to DefaultRateLimit if zero.
	RateLimit float64

	// BurstSize is the maximum burst of requests allowed.
	// Defaults to DefaultBurstSize if zero.
	BurstSize int

	// MaxResults is the maximum number of results to return per search.
	// Defaults to DefaultMaxResults if zero.
	MaxResults int

	// Enabled indicates whether this source is enabled.
	// Defaults to true if not explicitly set.
	Enabled bool
}

// Client implements the papersources.PaperSource interface for Semantic Scholar.
type Client struct {
	httpClient *papersources.HTTPClient
	config     Config
}

// Compile-time check that Client implements papersources.PaperSource.
var _ papersources.PaperSource = (*Client)(nil)

// NewClient creates a new Semantic Scholar client with the given configuration.
// If httpClient is nil, a new one will be created with the configuration settings.
func NewClient(cfg Config, httpClient *papersources.HTTPClient) *Client {
	// Apply defaults
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = DefaultRateLimit
	}
	if cfg.BurstSize == 0 {
		cfg.BurstSize = DefaultBurstSize
	}
	if cfg.MaxResults == 0 {
		cfg.MaxResults = DefaultMaxResults
	}

	// Create HTTP client if not provided
	if httpClient == nil {
		httpClient = papersources.NewHTTPClient(papersources.HTTPClientConfig{
			Timeout:      cfg.Timeout,
			RateLimit:    cfg.RateLimit,
			BurstSize:    cfg.BurstSize,
			APIKey:       cfg.APIKey,
			APIKeyHeader: apiKeyHeader,
		})
	}

	return &Client{
		httpClient: httpClient,
		config:     cfg,
	}
}

// Search queries Semantic Scholar for papers matching the given parameters.
func (c *Client) Search(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
	start := time.Now()

	// Build the search URL
	searchURL, err := c.buildSearchURL(params)
	if err != nil {
		return nil, fmt.Errorf("building search URL: %w", err)
	}

	// Create the request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Execute the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Handle error responses
	if err := c.handleErrorResponse(resp); err != nil {
		return nil, err
	}

	// Parse the response (limit body to 10MB to prevent resource exhaustion).
	var searchResp SearchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Convert to domain papers
	papers := c.convertToPapers(searchResp.Data)

	// Apply date filtering client-side if needed
	if params.DateFrom != nil || params.DateTo != nil {
		papers = filterByDate(papers, params.DateFrom, params.DateTo)
	}

	// Determine if there are more results
	hasMore := searchResp.Next > 0

	return &papersources.SearchResult{
		Papers:         papers,
		TotalResults:   searchResp.Total,
		HasMore:        hasMore,
		NextOffset:     searchResp.Next,
		Source:         domain.SourceTypeSemanticScholar,
		SearchDuration: time.Since(start),
	}, nil
}

// GetByID retrieves a specific paper by its Semantic Scholar ID or other identifier.
func (c *Client) GetByID(ctx context.Context, id string) (*domain.Paper, error) {
	// Build the paper URL
	paperURL := fmt.Sprintf("%s/paper/%s?fields=%s", c.config.BaseURL, url.PathEscape(id), paperFields)

	// Create the request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, paperURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Execute the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Handle 404 as not found
	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.NewNotFoundError("paper", id)
	}

	// Handle other error responses
	if err := c.handleErrorResponse(resp); err != nil {
		return nil, err
	}

	// Parse the response (limit body to 10MB to prevent resource exhaustion).
	var paperResult PaperResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&paperResult); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return c.convertToPaper(paperResult), nil
}

// SourceType returns the source type identifier.
func (c *Client) SourceType() domain.SourceType {
	return domain.SourceTypeSemanticScholar
}

// Name returns the human-readable name for this source.
func (c *Client) Name() string {
	return sourceName
}

// IsEnabled returns whether this source is currently enabled.
func (c *Client) IsEnabled() bool {
	return c.config.Enabled
}

// buildSearchURL constructs the search API URL with query parameters.
func (c *Client) buildSearchURL(params papersources.SearchParams) (string, error) {
	baseURL, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return "", fmt.Errorf("parsing base URL: %w", err)
	}

	// Build the search endpoint
	searchURL := baseURL.JoinPath("paper", "search")

	// Build query parameters
	q := searchURL.Query()
	q.Set("query", params.Query)
	q.Set("fields", paperFields)

	// Set limit
	limit := params.MaxResults
	if limit <= 0 || limit > c.config.MaxResults {
		limit = c.config.MaxResults
	}
	q.Set("limit", strconv.Itoa(limit))

	// Set offset
	if params.Offset > 0 {
		q.Set("offset", strconv.Itoa(params.Offset))
	}

	// Set open access filter
	if params.OpenAccessOnly {
		q.Set("openAccessPdf", "")
	}

	// Set year range for date filtering (Semantic Scholar uses year-based filtering)
	if params.DateFrom != nil {
		q.Set("year", fmt.Sprintf("%d-", params.DateFrom.Year()))
	}
	if params.DateTo != nil {
		existingYear := q.Get("year")
		if existingYear != "" {
			// Already has a start year, add end year
			q.Set("year", fmt.Sprintf("%s%d", existingYear, params.DateTo.Year()))
		} else {
			q.Set("year", fmt.Sprintf("-%d", params.DateTo.Year()))
		}
	}

	// Set minimum citation count
	if params.MinCitations > 0 {
		q.Set("minCitationCount", strconv.Itoa(params.MinCitations))
	}

	searchURL.RawQuery = q.Encode()
	return searchURL.String(), nil
}

// handleErrorResponse checks for API errors and returns appropriate error types.
func (c *Client) handleErrorResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	// Read the error body (limit to 1MB to prevent resource exhaustion)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return domain.NewExternalAPIError(sourceName, resp.StatusCode, "failed to read error response", err)
	}

	// Try to parse as JSON error
	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil {
		message := errResp.Error
		if message == "" {
			message = errResp.Message
		}
		if message == "" {
			message = string(body)
		}
		return domain.NewExternalAPIError(sourceName, resp.StatusCode, message, nil)
	}

	// Return raw body as error message
	return domain.NewExternalAPIError(sourceName, resp.StatusCode, string(body), nil)
}

// convertToPapers converts a slice of API paper results to domain papers.
func (c *Client) convertToPapers(results []PaperResult) []*domain.Paper {
	papers := make([]*domain.Paper, 0, len(results))
	for _, result := range results {
		papers = append(papers, c.convertToPaper(result))
	}
	return papers
}

// convertToPaper converts a single API paper result to a domain paper.
func (c *Client) convertToPaper(result PaperResult) *domain.Paper {
	paper := &domain.Paper{
		Title:           result.Title,
		Abstract:        result.Abstract,
		PublicationYear: result.Year,
		Venue:           result.Venue,
		CitationCount:   result.CitationCount,
		ReferenceCount:  result.ReferenceCount,
		OpenAccess:      result.IsOpenAccess,
		RawMetadata: map[string]interface{}{
			"semantic_scholar_id": result.PaperID,
			"source":              sourceName,
		},
	}

	// Parse publication date
	if result.PublicationDate != "" {
		if pubDate, err := time.Parse("2006-01-02", result.PublicationDate); err == nil {
			paper.PublicationDate = &pubDate
		}
	}

	// Set journal information
	if result.Journal != nil {
		paper.Journal = result.Journal.Name
		paper.Volume = result.Journal.Volume
		paper.Pages = result.Journal.Pages
	}

	// Set PDF URL
	if result.OpenAccessPDF != nil && result.OpenAccessPDF.URL != "" {
		paper.PDFURL = result.OpenAccessPDF.URL
	}

	// Convert authors
	paper.Authors = convertAuthors(result.Authors)

	// Generate canonical ID from identifiers
	paper.CanonicalID = generateCanonicalID(result)

	return paper
}

// convertAuthors converts API authors to domain authors.
func convertAuthors(apiAuthors []Author) []domain.Author {
	authors := make([]domain.Author, 0, len(apiAuthors))
	for _, a := range apiAuthors {
		authors = append(authors, domain.Author{
			Name: a.Name,
		})
	}
	return authors
}

// generateCanonicalID generates a canonical ID from paper identifiers.
func generateCanonicalID(result PaperResult) string {
	ids := domain.PaperIdentifiers{
		SemanticScholarID: result.PaperID,
	}

	if result.ExternalIDs != nil {
		ids.DOI = result.ExternalIDs.DOI
		ids.ArXivID = result.ExternalIDs.ArXiv
		ids.PubMedID = result.ExternalIDs.PubMed
		if result.ExternalIDs.PubMedCentral != "" {
			ids.PMCID = result.ExternalIDs.PubMedCentral
		}
	}

	return domain.GenerateCanonicalID(ids)
}

// filterByDate filters papers by publication date.
// This is needed because Semantic Scholar only supports year-based filtering via the API.
func filterByDate(papers []*domain.Paper, dateFrom, dateTo *time.Time) []*domain.Paper {
	if dateFrom == nil && dateTo == nil {
		return papers
	}

	filtered := make([]*domain.Paper, 0, len(papers))
	for _, paper := range papers {
		// If no publication date, use year for comparison
		var paperDate time.Time
		if paper.PublicationDate != nil {
			paperDate = *paper.PublicationDate
		} else if paper.PublicationYear > 0 {
			// Use January 1 of the publication year as a proxy
			paperDate = time.Date(paper.PublicationYear, time.January, 1, 0, 0, 0, 0, time.UTC)
		} else {
			// No date information, include the paper
			filtered = append(filtered, paper)
			continue
		}

		// Check date bounds
		if dateFrom != nil && paperDate.Before(*dateFrom) {
			continue
		}
		if dateTo != nil && paperDate.After(*dateTo) {
			continue
		}

		filtered = append(filtered, paper)
	}

	return filtered
}

// BuildSearchQuery is a helper to construct boolean search queries.
// Semantic Scholar supports standard boolean operators: AND, OR, NOT.
func BuildSearchQuery(terms []string, operator string) string {
	if len(terms) == 0 {
		return ""
	}
	if len(terms) == 1 {
		return terms[0]
	}

	op := strings.ToUpper(operator)
	if op != "AND" && op != "OR" {
		op = "AND"
	}

	return strings.Join(terms, " "+op+" ")
}
