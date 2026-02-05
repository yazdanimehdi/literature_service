package pubmed

import (
	"context"
	"encoding/xml"
	"errors"
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
	// DefaultBaseURL is the base URL for NCBI E-utilities API.
	DefaultBaseURL = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"

	// DefaultRateLimit is the rate limit without an API key (3 requests/second).
	// With an API key, the limit increases to 10 requests/second.
	DefaultRateLimit = 3.0

	// DefaultBurstSize is the default burst size for rate limiting.
	DefaultBurstSize = 3

	// DefaultTimeout is the default request timeout.
	DefaultTimeout = 30 * time.Second

	// DefaultMaxResults is the default maximum results per search.
	DefaultMaxResults = 100

	// MaxResultsLimit is the maximum results allowed per request by the API.
	MaxResultsLimit = 10000

	// sourceName is the human-readable name for this source.
	sourceName = "PubMed"
)

// Config holds the configuration for the PubMed client.
type Config struct {
	// BaseURL is the base URL for the E-utilities API.
	// Defaults to DefaultBaseURL if empty.
	BaseURL string

	// APIKey is the NCBI API key for higher rate limits.
	// Optional but recommended for production use.
	APIKey string

	// Timeout is the request timeout.
	// Defaults to DefaultTimeout if zero.
	Timeout time.Duration

	// RateLimit is the maximum requests per second.
	// Defaults to DefaultRateLimit (3 req/sec) if zero.
	// With an API key, you can increase this to 10 req/sec.
	RateLimit float64

	// BurstSize is the maximum burst of requests allowed.
	// Defaults to DefaultBurstSize if zero.
	BurstSize int

	// MaxResults is the default maximum results per search.
	// Defaults to DefaultMaxResults if zero.
	MaxResults int

	// Enabled indicates whether this source is enabled.
	// When false, Search and GetByID return errors.
	Enabled bool
}

// applyDefaults applies default values to the config.
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

// Client implements the papersources.PaperSource interface for PubMed.
type Client struct {
	config     Config
	httpClient *papersources.HTTPClient
}

// Compile-time check that Client implements PaperSource.
var _ papersources.PaperSource = (*Client)(nil)

// New creates a new PubMed client with the given configuration.
func New(cfg Config) *Client {
	cfg.applyDefaults()

	httpCfg := papersources.HTTPClientConfig{
		Timeout:   cfg.Timeout,
		RateLimit: cfg.RateLimit,
		BurstSize: cfg.BurstSize,
		UserAgent: "Helixir-LiteratureService/1.0 (mailto:support@helixir.io)",
	}

	return &Client{
		config:     cfg,
		httpClient: papersources.NewHTTPClient(httpCfg),
	}
}

// NewWithHTTPClient creates a new PubMed client with a custom HTTP client.
// This is useful for testing with mock servers.
func NewWithHTTPClient(cfg Config, httpClient *papersources.HTTPClient) *Client {
	cfg.applyDefaults()
	return &Client{
		config:     cfg,
		httpClient: httpClient,
	}
}

// Search queries PubMed for papers matching the given parameters.
// It performs a two-step search:
// 1. esearch.fcgi - retrieves PMIDs matching the query
// 2. efetch.fcgi - retrieves full article metadata for the PMIDs
func (c *Client) Search(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
	if !c.config.Enabled {
		return nil, errors.New("pubmed source is disabled")
	}

	startTime := time.Now()

	// Step 1: Search for PMIDs
	searchResult, err := c.esearch(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("esearch failed: %w", err)
	}

	// Check for errors in the response
	if searchResult.ErrorList != nil {
		if len(searchResult.ErrorList.PhraseNotFound) > 0 {
			// Return empty results for phrases not found (not an error)
			return &papersources.SearchResult{
				Papers:         []*domain.Paper{},
				TotalResults:   0,
				HasMore:        false,
				NextOffset:     0,
				Source:         domain.SourceTypePubMed,
				SearchDuration: time.Since(startTime),
			}, nil
		}
	}

	// If no results, return early
	if len(searchResult.IDList.IDs) == 0 {
		return &papersources.SearchResult{
			Papers:         []*domain.Paper{},
			TotalResults:   searchResult.Count,
			HasMore:        searchResult.Count > params.Offset,
			NextOffset:     params.Offset,
			Source:         domain.SourceTypePubMed,
			SearchDuration: time.Since(startTime),
		}, nil
	}

	// Step 2: Fetch full article metadata
	articles, err := c.efetch(ctx, searchResult.IDList.IDs)
	if err != nil {
		return nil, fmt.Errorf("efetch failed: %w", err)
	}

	// Convert articles to domain papers
	papers := make([]*domain.Paper, 0, len(articles.Articles))
	for _, article := range articles.Articles {
		paper := c.articleToPaper(article)
		papers = append(papers, paper)
	}

	// Calculate pagination info
	nextOffset := params.Offset + len(papers)
	hasMore := nextOffset < searchResult.Count

	return &papersources.SearchResult{
		Papers:         papers,
		TotalResults:   searchResult.Count,
		HasMore:        hasMore,
		NextOffset:     nextOffset,
		Source:         domain.SourceTypePubMed,
		SearchDuration: time.Since(startTime),
	}, nil
}

// GetByID retrieves a specific paper by its PubMed ID (PMID).
func (c *Client) GetByID(ctx context.Context, id string) (*domain.Paper, error) {
	if !c.config.Enabled {
		return nil, errors.New("pubmed source is disabled")
	}

	// Fetch the article
	articles, err := c.efetch(ctx, []string{id})
	if err != nil {
		return nil, fmt.Errorf("efetch failed: %w", err)
	}

	if len(articles.Articles) == 0 {
		return nil, domain.NewNotFoundError("paper", id)
	}

	return c.articleToPaper(articles.Articles[0]), nil
}

// SourceType returns the source type identifier.
func (c *Client) SourceType() domain.SourceType {
	return domain.SourceTypePubMed
}

// Name returns the human-readable name for this source.
func (c *Client) Name() string {
	return sourceName
}

// IsEnabled returns whether the source is enabled.
func (c *Client) IsEnabled() bool {
	return c.config.Enabled
}

// esearch performs a search query and returns matching PMIDs.
func (c *Client) esearch(ctx context.Context, params papersources.SearchParams) (*ESearchResult, error) {
	// Build URL with query parameters
	u, err := url.Parse(c.config.BaseURL + "/esearch.fcgi")
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	q := u.Query()
	q.Set("db", "pubmed")
	q.Set("term", params.Query)
	q.Set("retmode", "xml")
	q.Set("usehistory", "n")

	// Set result limits
	maxResults := params.MaxResults
	if maxResults <= 0 {
		maxResults = c.config.MaxResults
	}
	if maxResults > MaxResultsLimit {
		maxResults = MaxResultsLimit
	}
	q.Set("retmax", strconv.Itoa(maxResults))

	if params.Offset > 0 {
		q.Set("retstart", strconv.Itoa(params.Offset))
	}

	// Add date filters if provided
	if params.DateFrom != nil || params.DateTo != nil {
		q.Set("datetype", "pdat") // Publication date

		if params.DateFrom != nil {
			q.Set("mindate", params.DateFrom.Format("2006/01/02"))
		}
		if params.DateTo != nil {
			q.Set("maxdate", params.DateTo.Format("2006/01/02"))
		}
	}

	// Add API key if configured
	if c.config.APIKey != "" {
		q.Set("api_key", c.config.APIKey)
	}

	u.RawQuery = q.Encode()

	// Create and execute request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		return nil, domain.NewExternalAPIError(sourceName, resp.StatusCode, string(body), nil)
	}

	// Parse XML response
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result ESearchResult
	if err := xml.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse XML response: %w", err)
	}

	return &result, nil
}

// efetch retrieves full article metadata for the given PMIDs.
func (c *Client) efetch(ctx context.Context, pmids []string) (*PubmedArticleSet, error) {
	if len(pmids) == 0 {
		return &PubmedArticleSet{}, nil
	}

	// Build URL with query parameters
	u, err := url.Parse(c.config.BaseURL + "/efetch.fcgi")
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	q := u.Query()
	q.Set("db", "pubmed")
	q.Set("id", strings.Join(pmids, ","))
	q.Set("retmode", "xml")
	q.Set("rettype", "abstract")

	// Add API key if configured
	if c.config.APIKey != "" {
		q.Set("api_key", c.config.APIKey)
	}

	u.RawQuery = q.Encode()

	// Create and execute request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		return nil, domain.NewExternalAPIError(sourceName, resp.StatusCode, string(body), nil)
	}

	// Parse XML response
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result PubmedArticleSet
	if err := xml.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse XML response: %w", err)
	}

	return &result, nil
}

// articleToPaper converts a PubmedArticle to a domain.Paper.
func (c *Client) articleToPaper(article PubmedArticle) *domain.Paper {
	citation := article.MedlineCitation
	pubmedData := article.PubmedData

	// Extract DOI from ELocationID or ArticleIdList
	doi := extractDOI(citation.Article, pubmedData)

	// Build canonical ID
	ids := domain.PaperIdentifiers{
		DOI:      doi,
		PubMedID: citation.PMID.Value,
	}

	// Extract PMCID if available
	for _, aid := range pubmedData.ArticleIdList.ArticleIds {
		if aid.IdType == "pmc" {
			ids.PMCID = aid.Value
			break
		}
	}

	canonicalID := domain.GenerateCanonicalID(ids)

	// Extract publication date and year
	pubDate, pubYear := extractPublicationDate(citation.Article)

	// Extract abstract (concatenate multiple sections)
	abstract := extractAbstract(citation.Article.Abstract)

	// Extract authors
	authors := extractAuthors(citation.Article.AuthorList)

	// Extract venue/journal information
	journal := citation.Article.Journal.Title
	if journal == "" {
		journal = citation.Article.Journal.ISOAbbreviation
	}

	volume := citation.Article.Journal.JournalIssue.Volume
	issue := citation.Article.Journal.JournalIssue.Issue
	pages := extractPages(citation.Article.Pagination)

	// Build raw metadata
	rawMetadata := map[string]interface{}{
		"pmid":   citation.PMID.Value,
		"source": "pubmed",
	}
	if doi != "" {
		rawMetadata["doi"] = doi
	}
	if ids.PMCID != "" {
		rawMetadata["pmcid"] = ids.PMCID
	}

	// Add MeSH terms if available
	if citation.MeshHeadingList != nil {
		meshTerms := make([]string, 0, len(citation.MeshHeadingList.MeshHeadings))
		for _, mh := range citation.MeshHeadingList.MeshHeadings {
			meshTerms = append(meshTerms, mh.DescriptorName.Value)
		}
		rawMetadata["mesh_terms"] = meshTerms
	}

	// Add keywords if available
	if citation.KeywordList != nil {
		keywords := make([]string, 0, len(citation.KeywordList.Keywords))
		for _, kw := range citation.KeywordList.Keywords {
			keywords = append(keywords, kw.Value)
		}
		rawMetadata["keywords"] = keywords
	}

	return &domain.Paper{
		CanonicalID:     canonicalID,
		Title:           citation.Article.ArticleTitle,
		Abstract:        abstract,
		Authors:         authors,
		PublicationDate: pubDate,
		PublicationYear: pubYear,
		Venue:           journal,
		Journal:         journal,
		Volume:          volume,
		Issue:           issue,
		Pages:           pages,
		RawMetadata:     rawMetadata,
	}
}

// extractDOI extracts the DOI from article metadata.
// It checks ELocationID first (more reliable), then ArticleIdList.
func extractDOI(article Article, pubmedData PubmedData) string {
	// Check ELocationID first
	for _, eloc := range article.ELocationID {
		if eloc.EIdType == "doi" && (eloc.Valid == "" || eloc.Valid == "Y") {
			return eloc.Value
		}
	}

	// Check ArticleIdList
	for _, aid := range pubmedData.ArticleIdList.ArticleIds {
		if aid.IdType == "doi" {
			return aid.Value
		}
	}

	return ""
}

// extractPublicationDate extracts the publication date from the article.
// Returns the parsed date and year. Uses ArticleDate if available, otherwise PubDate.
func extractPublicationDate(article Article) (*time.Time, int) {
	// Try ArticleDate first (more precise)
	for _, ad := range article.ArticleDate {
		if ad.DateType == "epublish" || ad.DateType == "Electronic" || ad.DateType == "" {
			if t := parseDate(ad.Year, ad.Month, ad.Day); t != nil {
				return t, t.Year()
			}
		}
	}

	// Fall back to PubDate from JournalIssue
	pubDate := article.Journal.JournalIssue.PubDate

	// Handle MedlineDate format (e.g., "2020 Jan-Feb")
	if pubDate.MedlineDate != "" {
		year := extractYearFromMedlineDate(pubDate.MedlineDate)
		if year > 0 {
			t := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
			return &t, year
		}
	}

	// Standard date format
	if pubDate.Year != "" {
		t := parseDate(pubDate.Year, pubDate.Month, pubDate.Day)
		if t != nil {
			return t, t.Year()
		}
		// If we have a year but couldn't parse a full date, return year only
		if year, err := strconv.Atoi(pubDate.Year); err == nil {
			t := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
			return &t, year
		}
	}

	return nil, 0
}

// parseDate parses year, month, day strings into a time.Time.
func parseDate(year, month, day string) *time.Time {
	if year == "" {
		return nil
	}

	y, err := strconv.Atoi(year)
	if err != nil {
		return nil
	}

	m := parseMonth(month)
	d := 1
	if day != "" {
		if parsed, err := strconv.Atoi(day); err == nil {
			d = parsed
		}
	}

	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

// monthNames maps lowercase month name strings (abbreviation and full) to time.Month.
// This is a package-level variable to avoid re-allocating on every call to parseMonth.
var monthNames = map[string]time.Month{
	"jan": time.January, "january": time.January,
	"feb": time.February, "february": time.February,
	"mar": time.March, "march": time.March,
	"apr": time.April, "april": time.April,
	"may": time.May,
	"jun": time.June, "june": time.June,
	"jul": time.July, "july": time.July,
	"aug": time.August, "august": time.August,
	"sep": time.September, "september": time.September,
	"oct": time.October, "october": time.October,
	"nov": time.November, "november": time.November,
	"dec": time.December, "december": time.December,
}

// parseMonth parses a month string (numeric or name) into time.Month.
func parseMonth(month string) time.Month {
	if month == "" {
		return time.January
	}

	// Try numeric
	if m, err := strconv.Atoi(month); err == nil && m >= 1 && m <= 12 {
		return time.Month(m)
	}

	if m, ok := monthNames[strings.ToLower(month)]; ok {
		return m
	}

	return time.January
}

// extractYearFromMedlineDate extracts the year from a MedlineDate string.
func extractYearFromMedlineDate(medlineDate string) int {
	// MedlineDate can be "2020 Jan-Feb", "2020 Spring", "2020-2021", etc.
	parts := strings.Fields(medlineDate)
	if len(parts) > 0 {
		// Try the first part as a year
		yearStr := strings.Split(parts[0], "-")[0]
		if year, err := strconv.Atoi(yearStr); err == nil {
			return year
		}
	}
	return 0
}

// extractAbstract concatenates multiple abstract sections into a single string.
func extractAbstract(abstract *Abstract) string {
	if abstract == nil || len(abstract.AbstractTexts) == 0 {
		return ""
	}

	// If only one section without label, return it directly
	if len(abstract.AbstractTexts) == 1 && abstract.AbstractTexts[0].Label == "" {
		return strings.TrimSpace(abstract.AbstractTexts[0].Value)
	}

	// Concatenate multiple sections with labels
	var parts []string
	for _, at := range abstract.AbstractTexts {
		text := strings.TrimSpace(at.Value)
		if text == "" {
			continue
		}
		if at.Label != "" {
			parts = append(parts, at.Label+": "+text)
		} else {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, " ")
}

// extractAuthors converts PubMed authors to domain authors.
func extractAuthors(authorList *AuthorList) []domain.Author {
	if authorList == nil || len(authorList.Authors) == 0 {
		return nil
	}

	authors := make([]domain.Author, 0, len(authorList.Authors))
	for _, a := range authorList.Authors {
		// Skip invalid authors
		if a.ValidYN == "N" {
			continue
		}

		// Build name
		var name string
		if a.CollectiveName != "" {
			name = a.CollectiveName
		} else {
			// Combine ForeName and LastName
			nameParts := make([]string, 0, 2)
			if a.ForeName != "" {
				nameParts = append(nameParts, a.ForeName)
			}
			if a.LastName != "" {
				nameParts = append(nameParts, a.LastName)
			}
			name = strings.Join(nameParts, " ")
		}

		if name == "" {
			continue
		}

		// Extract ORCID if available
		var orcid string
		for _, id := range a.Identifiers {
			if strings.ToUpper(id.Source) == "ORCID" {
				orcid = id.Value
				break
			}
		}

		// Extract first affiliation
		var affiliation string
		if len(a.AffiliationInfo) > 0 {
			affiliation = a.AffiliationInfo[0].Affiliation
		}

		authors = append(authors, domain.Author{
			Name:        name,
			Affiliation: affiliation,
			ORCID:       orcid,
		})
	}

	return authors
}

// extractPages formats the page information.
func extractPages(pagination *Pagination) string {
	if pagination == nil {
		return ""
	}

	if pagination.MedlinePgn != "" {
		return pagination.MedlinePgn
	}

	if pagination.StartPage != "" {
		if pagination.EndPage != "" && pagination.EndPage != pagination.StartPage {
			return pagination.StartPage + "-" + pagination.EndPage
		}
		return pagination.StartPage
	}

	return ""
}
