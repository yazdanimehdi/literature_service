package arxiv

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/papersources"
)

const (
	// DefaultBaseURL is the default arXiv API base URL.
	DefaultBaseURL = "https://export.arxiv.org/api"

	// DefaultRateLimit is the default rate limit (3 requests per second).
	DefaultRateLimit = 3.0

	// DefaultBurstSize is the default burst size for rate limiting.
	DefaultBurstSize = 3

	// DefaultTimeout is the default request timeout.
	DefaultTimeout = 30 * time.Second

	// DefaultMaxResults is the default maximum results per request.
	DefaultMaxResults = 100

	// sourceName is the human-readable name for this source.
	sourceName = "arXiv"
)

// arxivIDRegex extracts the arXiv ID from the full URL.
// Matches patterns like "http://arxiv.org/abs/2301.12345v1" or "http://arxiv.org/abs/hep-th/9901001v1".
var arxivIDRegex = regexp.MustCompile(`arxiv\.org/abs/(.+?)(?:v\d+)?$`)

// Config holds configuration for the arXiv client.
type Config struct {
	// BaseURL is the arXiv API base URL.
	BaseURL string

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

// Client implements the papersources.PaperSource interface for arXiv.
type Client struct {
	config     Config
	httpClient *papersources.HTTPClient
}

// Ensure Client implements PaperSource interface.
var _ papersources.PaperSource = (*Client)(nil)

// New creates a new arXiv client with the given configuration.
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

// NewWithHTTPClient creates a new arXiv client with a custom HTTP client.
// This is useful for testing with mock servers.
func NewWithHTTPClient(cfg Config, httpClient *papersources.HTTPClient) *Client {
	cfg.applyDefaults()

	return &Client{
		config:     cfg,
		httpClient: httpClient,
	}
}

// Search queries arXiv for papers matching the given parameters.
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

	// Parse the Atom XML response (limit body to 10MB).
	var feed Feed
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&feed); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	papers := make([]*domain.Paper, 0, len(feed.Entries))
	for i := range feed.Entries {
		paper := c.entryToPaper(&feed.Entries[i])
		if paper != nil {
			papers = append(papers, paper)
		}
	}

	maxResults := params.MaxResults
	if maxResults == 0 {
		maxResults = c.config.MaxResults
	}
	nextOffset := params.Offset + len(papers)
	hasMore := nextOffset < feed.TotalResults

	return &papersources.SearchResult{
		Papers:         papers,
		TotalResults:   feed.TotalResults,
		HasMore:        hasMore,
		NextOffset:     nextOffset,
		Source:         domain.SourceTypeArXiv,
		SearchDuration: time.Since(startTime),
	}, nil
}

// GetByID retrieves a specific paper by its arXiv ID.
func (c *Client) GetByID(ctx context.Context, id string) (*domain.Paper, error) {
	baseURL, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing base URL: %w", err)
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/query"
	query := url.Values{}
	query.Set("id_list", id)
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
			sourceName,
			resp.StatusCode,
			string(body),
			nil,
		)
	}

	var feed Feed
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&feed); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(feed.Entries) == 0 {
		return nil, domain.NewNotFoundError("paper", id)
	}

	paper := c.entryToPaper(&feed.Entries[0])
	if paper == nil {
		return nil, domain.NewNotFoundError("paper", id)
	}

	return paper, nil
}

// SourceType returns the source type identifier.
func (c *Client) SourceType() domain.SourceType {
	return domain.SourceTypeArXiv
}

// Name returns the human-readable name for this source.
func (c *Client) Name() string {
	return sourceName
}

// IsEnabled returns whether this source is enabled.
func (c *Client) IsEnabled() bool {
	return c.config.Enabled
}

// buildSearchURL constructs the arXiv search API URL.
func (c *Client) buildSearchURL(params papersources.SearchParams) (string, error) {
	baseURL, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return "", fmt.Errorf("parsing base URL: %w", err)
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/query"

	query := url.Values{}

	// Build search query
	searchQuery := "all:" + params.Query

	// Add date filter if specified
	if params.DateFrom != nil || params.DateTo != nil {
		dateFilter := c.buildDateFilter(params.DateFrom, params.DateTo)
		if dateFilter != "" {
			searchQuery = searchQuery + " AND " + dateFilter
		}
	}

	query.Set("search_query", searchQuery)

	// Pagination
	maxResults := params.MaxResults
	if maxResults == 0 {
		maxResults = c.config.MaxResults
	}
	query.Set("max_results", strconv.Itoa(maxResults))

	if params.Offset > 0 {
		query.Set("start", strconv.Itoa(params.Offset))
	}

	// Sort by submission date (newest first)
	query.Set("sortBy", "submittedDate")
	query.Set("sortOrder", "descending")

	baseURL.RawQuery = query.Encode()
	return baseURL.String(), nil
}

// buildDateFilter constructs the arXiv date filter string.
func (c *Client) buildDateFilter(from, to *time.Time) string {
	var fromStr, toStr string

	if from != nil {
		fromStr = from.Format("20060102") + "0000"
	} else {
		fromStr = "*"
	}

	if to != nil {
		toStr = to.Format("20060102") + "2359"
	} else {
		toStr = "*"
	}

	return fmt.Sprintf("submittedDate:[%s TO %s]", fromStr, toStr)
}

// entryToPaper converts an arXiv Atom entry to a domain Paper.
func (c *Client) entryToPaper(entry *Entry) *domain.Paper {
	if entry == nil {
		return nil
	}

	// Extract arXiv ID from the entry URL
	arxivID := extractArXivID(entry.ID)
	if arxivID == "" {
		return nil
	}

	// Extract DOI
	doi := strings.TrimSpace(entry.DOI)

	// Generate canonical ID
	canonicalID := domain.GenerateCanonicalID(domain.PaperIdentifiers{
		ArXivID: arxivID,
		DOI:     doi,
	})

	if canonicalID == "" {
		return nil
	}

	// Parse publication date
	var pubDate *time.Time
	var pubYear int
	if entry.Published != "" {
		if t, err := time.Parse(time.RFC3339, entry.Published); err == nil {
			pubDate = &t
			pubYear = t.Year()
		}
	}

	// Extract authors
	authors := make([]domain.Author, 0, len(entry.Authors))
	for _, a := range entry.Authors {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
		authors = append(authors, domain.Author{
			Name:        name,
			Affiliation: strings.TrimSpace(a.Affiliation),
		})
	}

	// Normalize title and abstract (arXiv includes leading/trailing whitespace and newlines)
	title := normalizeWhitespace(entry.Title)
	abstract := normalizeWhitespace(entry.Summary)

	// Extract PDF URL from links
	pdfURL := ""
	for _, link := range entry.Links {
		if link.Title == "pdf" || link.Type == "application/pdf" {
			pdfURL = link.Href
			break
		}
	}
	if pdfURL == "" && arxivID != "" {
		pdfURL = "http://arxiv.org/pdf/" + arxivID
	}

	// Extract categories for metadata
	categories := make([]string, 0, len(entry.Categories))
	for _, cat := range entry.Categories {
		if cat.Term != "" {
			categories = append(categories, cat.Term)
		}
	}

	rawMetadata := map[string]interface{}{
		"arxiv_id":   arxivID,
		"categories": categories,
	}
	if entry.DOI != "" {
		rawMetadata["doi"] = doi
	}
	if entry.JournalRef != "" {
		rawMetadata["journal_ref"] = strings.TrimSpace(entry.JournalRef)
	}
	if entry.Comment != "" {
		rawMetadata["comment"] = strings.TrimSpace(entry.Comment)
	}
	if entry.PrimaryCategory.Term != "" {
		rawMetadata["primary_category"] = entry.PrimaryCategory.Term
	}

	return &domain.Paper{
		CanonicalID:     canonicalID,
		Title:           title,
		Abstract:        abstract,
		Authors:         authors,
		PublicationDate: pubDate,
		PublicationYear: pubYear,
		PDFURL:          pdfURL,
		OpenAccess:      true, // arXiv papers are always open access
		RawMetadata:     rawMetadata,
	}
}

// extractArXivID extracts the arXiv ID from the full entry URL.
// Input: "http://arxiv.org/abs/2301.12345v1" → "2301.12345"
func extractArXivID(entryURL string) string {
	matches := arxivIDRegex.FindStringSubmatch(entryURL)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// normalizeWhitespace trims and collapses multiple whitespace characters.
func normalizeWhitespace(s string) string {
	s = strings.TrimSpace(s)
	// Collapse multiple whitespace (including newlines) into single spaces
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
