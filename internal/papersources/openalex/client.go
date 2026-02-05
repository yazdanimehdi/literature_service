package openalex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/papersources"
)

const (
	// DefaultBaseURL is the default OpenAlex API base URL.
	DefaultBaseURL = "https://api.openalex.org"

	// DefaultRateLimit is the default rate limit for requests per second.
	// OpenAlex polite pool (with email) allows higher rates.
	DefaultRateLimit = 10.0

	// DefaultBurstSize is the default burst size for rate limiting.
	DefaultBurstSize = 10

	// DefaultTimeout is the default request timeout.
	DefaultTimeout = 30 * time.Second

	// DefaultMaxResults is the default maximum results per request.
	DefaultMaxResults = 25

	// doiPrefix is the URL prefix that OpenAlex uses for DOIs.
	doiPrefix = "https://doi.org/"

	// openAlexIDPrefix is the URL prefix for OpenAlex IDs.
	openAlexIDPrefix = "https://openalex.org/"
)

// Config holds configuration for the OpenAlex client.
type Config struct {
	// BaseURL is the OpenAlex API base URL.
	// Defaults to https://api.openalex.org
	BaseURL string

	// Email is the contact email for the polite pool.
	// Providing an email grants access to higher rate limits.
	// See: https://docs.openalex.org/how-to-use-the-api/rate-limits-and-authentication
	Email string

	// Timeout is the request timeout.
	// Defaults to 30 seconds.
	Timeout time.Duration

	// RateLimit is the maximum requests per second.
	// Defaults to 10 req/sec (polite pool with email gets higher).
	RateLimit float64

	// BurstSize is the maximum burst of requests allowed.
	// Defaults to 10.
	BurstSize int

	// MaxResults is the maximum results to return per search request.
	// Defaults to 25, maximum is 200 per OpenAlex API.
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

// Client implements the papersources.PaperSource interface for OpenAlex.
type Client struct {
	config     Config
	httpClient *papersources.HTTPClient
}

// Ensure Client implements PaperSource interface.
var _ papersources.PaperSource = (*Client)(nil)

// New creates a new OpenAlex client with the given configuration.
func New(cfg Config) *Client {
	cfg.applyDefaults()

	httpClient := papersources.NewHTTPClient(papersources.HTTPClientConfig{
		Timeout:   cfg.Timeout,
		RateLimit: cfg.RateLimit,
		BurstSize: cfg.BurstSize,
		UserAgent: "Helixir-LiteratureService/1.0 (mailto:" + cfg.Email + ")",
	})

	return &Client{
		config:     cfg,
		httpClient: httpClient,
	}
}

// NewWithHTTPClient creates a new OpenAlex client with a custom HTTP client.
// This is useful for testing with mock servers.
func NewWithHTTPClient(cfg Config, httpClient *papersources.HTTPClient) *Client {
	cfg.applyDefaults()

	return &Client{
		config:     cfg,
		httpClient: httpClient,
	}
}

// Search queries OpenAlex for papers matching the given parameters.
func (c *Client) Search(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
	startTime := time.Now()

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

	// Handle non-success status codes
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, domain.NewExternalAPIError(
			"OpenAlex",
			resp.StatusCode,
			string(body),
			nil,
		)
	}

	// Parse the response (limit body to 10MB to prevent resource exhaustion).
	var searchResp SearchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Convert works to domain papers
	papers := make([]*domain.Paper, 0, len(searchResp.Results))
	for _, work := range searchResp.Results {
		paper := c.workToPaper(&work)
		if paper != nil {
			papers = append(papers, paper)
		}
	}

	// Calculate pagination info
	maxResults := params.MaxResults
	if maxResults == 0 {
		maxResults = c.config.MaxResults
	}
	nextOffset := params.Offset + len(papers)
	hasMore := nextOffset < searchResp.Meta.Count

	return &papersources.SearchResult{
		Papers:         papers,
		TotalResults:   searchResp.Meta.Count,
		HasMore:        hasMore,
		NextOffset:     nextOffset,
		Source:         domain.SourceTypeOpenAlex,
		SearchDuration: time.Since(startTime),
	}, nil
}

// GetByID retrieves a specific paper by its OpenAlex ID or DOI.
func (c *Client) GetByID(ctx context.Context, id string) (*domain.Paper, error) {
	// Build the URL for fetching by ID
	fetchURL, err := c.buildGetByIDURL(id)
	if err != nil {
		return nil, fmt.Errorf("building fetch URL: %w", err)
	}

	// Create the request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Execute the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Handle non-success status codes
	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.NewNotFoundError("paper", id)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, domain.NewExternalAPIError(
			"OpenAlex",
			resp.StatusCode,
			string(body),
			nil,
		)
	}

	// Parse the response (single work, not a search response).
	// Limit body to 10MB to prevent resource exhaustion.
	var work Work
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&work); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	paper := c.workToPaper(&work)
	if paper == nil {
		return nil, domain.NewNotFoundError("paper", id)
	}

	return paper, nil
}

// SourceType returns the source type identifier.
func (c *Client) SourceType() domain.SourceType {
	return domain.SourceTypeOpenAlex
}

// Name returns the human-readable name for this source.
func (c *Client) Name() string {
	return "OpenAlex"
}

// IsEnabled returns whether this source is enabled.
func (c *Client) IsEnabled() bool {
	return c.config.Enabled
}

// buildSearchURL constructs the search API URL with query parameters.
func (c *Client) buildSearchURL(params papersources.SearchParams) (string, error) {
	baseURL, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return "", fmt.Errorf("parsing base URL: %w", err)
	}

	baseURL.Path = "/works"

	query := url.Values{}

	// Add search query
	if params.Query != "" {
		query.Set("search", params.Query)
	}

	// Build filter string
	filters := c.buildFilters(params)
	if len(filters) > 0 {
		query.Set("filter", strings.Join(filters, ","))
	}

	// Add pagination
	maxResults := params.MaxResults
	if maxResults == 0 {
		maxResults = c.config.MaxResults
	}
	if maxResults > 200 {
		maxResults = 200 // OpenAlex API limit
	}
	query.Set("per_page", strconv.Itoa(maxResults))

	// OpenAlex uses page-based pagination (1-indexed)
	if params.Offset > 0 {
		page := (params.Offset / maxResults) + 1
		query.Set("page", strconv.Itoa(page))
	}

	// Add mailto for polite pool
	if c.config.Email != "" {
		query.Set("mailto", c.config.Email)
	}

	baseURL.RawQuery = query.Encode()
	return baseURL.String(), nil
}

// buildFilters constructs the filter query string components.
func (c *Client) buildFilters(params papersources.SearchParams) []string {
	var filters []string

	// Date range filters
	if params.DateFrom != nil {
		filters = append(filters, fmt.Sprintf("from_publication_date:%s", params.DateFrom.Format("2006-01-02")))
	}
	if params.DateTo != nil {
		filters = append(filters, fmt.Sprintf("to_publication_date:%s", params.DateTo.Format("2006-01-02")))
	}

	// Open access filter
	if params.OpenAccessOnly {
		filters = append(filters, "is_oa:true")
	}

	// Minimum citations filter
	if params.MinCitations > 0 {
		filters = append(filters, fmt.Sprintf("cited_by_count:>%d", params.MinCitations-1))
	}

	// Exclude preprints if not requested
	// OpenAlex type for preprints is "preprint"
	if !params.IncludePreprints {
		filters = append(filters, "type:!preprint")
	}

	return filters
}

// buildGetByIDURL constructs the URL for fetching a work by ID.
func (c *Client) buildGetByIDURL(id string) (string, error) {
	baseURL, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return "", fmt.Errorf("parsing base URL: %w", err)
	}

	// Determine the ID format and construct the path
	// OpenAlex accepts: OpenAlex ID, DOI, MAG ID, PubMed ID, PMC ID
	var workID string
	switch {
	case strings.HasPrefix(id, openAlexIDPrefix):
		// Full OpenAlex URL - extract the ID part
		workID = strings.TrimPrefix(id, openAlexIDPrefix)
	case strings.HasPrefix(id, "W"):
		// Short OpenAlex ID (e.g., W2741809807)
		workID = id
	case strings.HasPrefix(id, doiPrefix):
		// Full DOI URL
		workID = id
	case strings.HasPrefix(id, "10."):
		// Short DOI format - prefix with https://doi.org/
		workID = doiPrefix + id
	case strings.HasPrefix(id, "doi:"):
		// Canonical DOI format from our system
		workID = doiPrefix + strings.TrimPrefix(id, "doi:")
	default:
		// Assume it is an OpenAlex ID or other supported format
		workID = id
	}

	// Use direct path concatenation - OpenAlex expects the DOI as-is in the path
	// and handles URL decoding on their side
	baseURL.Path = "/works/" + workID

	// Add mailto for polite pool
	if c.config.Email != "" {
		query := url.Values{}
		query.Set("mailto", c.config.Email)
		baseURL.RawQuery = query.Encode()
	}

	return baseURL.String(), nil
}

// workToPaper converts an OpenAlex Work to a domain Paper.
func (c *Client) workToPaper(work *Work) *domain.Paper {
	if work == nil {
		return nil
	}

	// Extract and normalize DOI
	doi := normalizeDOI(work.DOI)
	if doi == "" && work.IDs.DOI != "" {
		doi = normalizeDOI(work.IDs.DOI)
	}

	// Extract OpenAlex ID
	openAlexID := normalizeOpenAlexID(work.ID)
	if openAlexID == "" && work.IDs.OpenAlex != "" {
		openAlexID = normalizeOpenAlexID(work.IDs.OpenAlex)
	}

	// Generate canonical ID
	canonicalID := domain.GenerateCanonicalID(domain.PaperIdentifiers{
		DOI:        doi,
		PubMedID:   normalizePMID(work.IDs.PMID),
		PMCID:      work.IDs.PMCID,
		OpenAlexID: openAlexID,
	})

	// Skip papers without any identifier
	if canonicalID == "" {
		return nil
	}

	// Extract publication date
	var pubDate *time.Time
	if work.PublicationDate != "" {
		if t, err := time.Parse("2006-01-02", work.PublicationDate); err == nil {
			pubDate = &t
		}
	}

	// Extract authors
	authors := make([]domain.Author, 0, len(work.Authorships))
	for _, authorship := range work.Authorships {
		author := domain.Author{
			Name:  authorship.Author.DisplayName,
			ORCID: normalizeORCID(authorship.Author.Orcid),
		}
		// Get affiliation from first institution
		if len(authorship.Institutions) > 0 {
			author.Affiliation = authorship.Institutions[0].DisplayName
		}
		authors = append(authors, author)
	}

	// Get title - prefer display_name as it is usually cleaner
	title := work.DisplayName
	if title == "" {
		title = work.Title
	}

	// Get venue/journal from primary location
	var venue, journal string
	if work.PrimaryLocation != nil && work.PrimaryLocation.Source != nil {
		journal = work.PrimaryLocation.Source.DisplayName
		venue = journal
	}

	// Determine if open access
	isOpenAccess := work.IsOpenAccess
	if work.OpenAccess != nil {
		isOpenAccess = work.OpenAccess.IsOA
	}

	// Get PDF URL
	var pdfURL string
	if work.OpenAccess != nil && work.OpenAccess.OAURL != "" {
		pdfURL = work.OpenAccess.OAURL
	} else if work.PrimaryLocation != nil && work.PrimaryLocation.PDFURL != "" {
		pdfURL = work.PrimaryLocation.PDFURL
	}

	// Reconstruct abstract from inverted index
	abstract := reconstructAbstract(work.AbstractInvertedIndex)

	return &domain.Paper{
		CanonicalID:     canonicalID,
		Title:           title,
		Abstract:        abstract,
		Authors:         authors,
		PublicationDate: pubDate,
		PublicationYear: work.PublicationYear,
		Venue:           venue,
		Journal:         journal,
		CitationCount:   work.CitedByCount,
		ReferenceCount:  len(work.ReferencedWorks),
		PDFURL:          pdfURL,
		OpenAccess:      isOpenAccess,
		RawMetadata: map[string]interface{}{
			"openalex_id":     openAlexID,
			"doi":             doi,
			"type":            work.Type,
			"pmid":            work.IDs.PMID,
			"pmcid":           work.IDs.PMCID,
			"referenced_work": work.ReferencedWorks,
		},
	}
}

// normalizeDOI strips the https://doi.org/ prefix from DOIs and returns lowercase.
func normalizeDOI(doi string) string {
	if doi == "" {
		return ""
	}
	// Trim whitespace first
	doi = strings.TrimSpace(doi)
	// Strip the URL prefix if present
	doi = strings.TrimPrefix(doi, doiPrefix)
	doi = strings.TrimPrefix(doi, "http://doi.org/")
	doi = strings.TrimPrefix(doi, "doi:")
	return strings.ToLower(strings.TrimSpace(doi))
}

// normalizeOpenAlexID extracts the short ID from full OpenAlex URLs.
func normalizeOpenAlexID(id string) string {
	if id == "" {
		return ""
	}
	// Strip the URL prefix if present
	id = strings.TrimPrefix(id, openAlexIDPrefix)
	return strings.TrimSpace(id)
}

// normalizePMID strips any URL prefixes from PubMed IDs.
func normalizePMID(pmid string) string {
	if pmid == "" {
		return ""
	}
	pmid = strings.TrimPrefix(pmid, "https://pubmed.ncbi.nlm.nih.gov/")
	return strings.TrimSpace(pmid)
}

// normalizeORCID strips any URL prefixes from ORCID identifiers.
func normalizeORCID(orcid string) string {
	if orcid == "" {
		return ""
	}
	orcid = strings.TrimPrefix(orcid, "https://orcid.org/")
	return strings.TrimSpace(orcid)
}

// reconstructAbstract reconstructs the abstract text from OpenAlex's inverted index format.
// OpenAlex stores abstracts as inverted indices mapping words to their positions.
func reconstructAbstract(invertedIndex map[string][]int) string {
	if len(invertedIndex) == 0 {
		return ""
	}

	// Build a slice of (position, word) pairs.
	// Pre-calculate total capacity by summing all position slice lengths.
	type posWord struct {
		pos  int
		word string
	}
	const maxAbstractWords = 100_000
	totalPairs := 0
	for _, positions := range invertedIndex {
		totalPairs += len(positions)
	}
	// Guard against malicious payloads with excessive position entries.
	if totalPairs > maxAbstractWords {
		return ""
	}
	pairs := make([]posWord, 0, totalPairs)

	for word, positions := range invertedIndex {
		for _, pos := range positions {
			pairs = append(pairs, posWord{pos: pos, word: word})
		}
	}

	// Sort by position
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].pos < pairs[j].pos
	})

	// Reconstruct the text with pre-sized builder to reduce allocations.
	// Estimate average word length of 6 characters plus a space separator.
	var builder strings.Builder
	builder.Grow(totalPairs * 7)
	for i, pair := range pairs {
		if i > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(pair.word)
	}

	return builder.String()
}
