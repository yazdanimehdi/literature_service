package httpserver

import (
	"time"

	"github.com/helixir/literature-review-service/internal/domain"
)

// Review response types for JSON serialization.

type startReviewResponse struct {
	ReviewID   string    `json:"review_id"`
	WorkflowID string   `json:"workflow_id"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	Message    string    `json:"message"`
}

type reviewStatusResponse struct {
	ReviewID     string            `json:"review_id"`
	Status       string            `json:"status"`
	Progress     *progressResponse `json:"progress,omitempty"`
	ErrorMessage string            `json:"error_message,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	StartedAt    *time.Time        `json:"started_at,omitempty"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty"`
	Duration     string            `json:"duration,omitempty"`
	Config       *configResponse   `json:"configuration,omitempty"`
}

type progressResponse struct {
	InitialKeywordsCount   int `json:"initial_keywords_count"`
	TotalKeywordsProcessed int `json:"total_keywords_processed"`
	PapersFound            int `json:"papers_found"`
	PapersNew              int `json:"papers_new"`
	PapersIngested         int `json:"papers_ingested"`
	PapersFailed           int `json:"papers_failed"`
	MaxExpansionDepth      int `json:"max_expansion_depth"`
}

type configResponse struct {
	InitialKeywordCount int      `json:"initial_keyword_count"`
	PaperKeywordCount   int      `json:"paper_keyword_count"`
	MaxExpansionDepth   int      `json:"max_expansion_depth"`
	EnabledSources      []string `json:"enabled_sources"`
	DateFrom            string   `json:"date_from,omitempty"`
	DateTo              string   `json:"date_to,omitempty"`
}

type reviewSummaryResponse struct {
	ReviewID       string     `json:"review_id"`
	Title          string     `json:"title"`
	Status         string     `json:"status"`
	PapersFound    int        `json:"papers_found"`
	PapersIngested int        `json:"papers_ingested"`
	KeywordsUsed   int        `json:"keywords_used"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	Duration       string     `json:"duration,omitempty"`
}

type listReviewsResponse struct {
	Reviews       []reviewSummaryResponse `json:"reviews"`
	NextPageToken string                  `json:"next_page_token,omitempty"`
	TotalCount    int                     `json:"total_count"`
}

type cancelReviewResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	FinalStatus string `json:"final_status"`
}

type paperResponse struct {
	ID              string           `json:"id"`
	CanonicalID     string           `json:"canonical_id,omitempty"`
	Title           string           `json:"title"`
	Abstract        string           `json:"abstract,omitempty"`
	Authors         []authorResponse `json:"authors,omitempty"`
	PublicationDate *time.Time       `json:"publication_date,omitempty"`
	PublicationYear int              `json:"publication_year,omitempty"`
	Venue           string           `json:"venue,omitempty"`
	Journal         string           `json:"journal,omitempty"`
	CitationCount   int              `json:"citation_count"`
	PdfURL          string           `json:"pdf_url,omitempty"`
	OpenAccess      bool             `json:"open_access"`
}

type authorResponse struct {
	Name        string `json:"name"`
	Affiliation string `json:"affiliation,omitempty"`
	ORCID       string `json:"orcid,omitempty"`
}

type listPapersResponse struct {
	Papers        []paperResponse `json:"papers"`
	NextPageToken string          `json:"next_page_token,omitempty"`
	TotalCount    int             `json:"total_count"`
}

type keywordResponse struct {
	ID                string `json:"id"`
	Keyword           string `json:"keyword"`
	NormalizedKeyword string `json:"normalized_keyword"`
}

type listKeywordsResponse struct {
	Keywords      []keywordResponse `json:"keywords"`
	NextPageToken string            `json:"next_page_token,omitempty"`
	TotalCount    int               `json:"total_count"`
}

// Converter functions

func domainReviewToStatusResponse(r *domain.LiteratureReviewRequest) reviewStatusResponse {
	resp := reviewStatusResponse{
		ReviewID:     r.ID.String(),
		Status:       string(r.Status),
		ErrorMessage: r.ErrorMessage,
		CreatedAt:    r.CreatedAt,
		StartedAt:    r.StartedAt,
		CompletedAt:  r.CompletedAt,
		Progress: &progressResponse{
			InitialKeywordsCount:   r.Configuration.MaxKeywordsPerRound,
			TotalKeywordsProcessed: r.KeywordsFoundCount,
			PapersFound:            r.PapersFoundCount,
			PapersNew:              r.PapersFoundCount,
			PapersIngested:         r.PapersIngestedCount,
			PapersFailed:           r.PapersFailedCount,
			MaxExpansionDepth:      r.Configuration.MaxExpansionDepth,
		},
		Config: domainConfigToResponse(r.Configuration),
	}
	if d := r.Duration(); d > 0 {
		resp.Duration = d.String()
	}
	return resp
}

func domainConfigToResponse(c domain.ReviewConfiguration) *configResponse {
	sources := make([]string, len(c.Sources))
	for i, s := range c.Sources {
		sources[i] = string(s)
	}
	resp := &configResponse{
		InitialKeywordCount: c.MaxKeywordsPerRound,
		PaperKeywordCount:   c.PaperKeywordCount,
		MaxExpansionDepth:   c.MaxExpansionDepth,
		EnabledSources:      sources,
	}
	if c.DateFrom != nil {
		resp.DateFrom = c.DateFrom.Format(time.RFC3339)
	}
	if c.DateTo != nil {
		resp.DateTo = c.DateTo.Format(time.RFC3339)
	}
	return resp
}

func domainReviewToSummary(r *domain.LiteratureReviewRequest) reviewSummaryResponse {
	resp := reviewSummaryResponse{
		ReviewID:       r.ID.String(),
		Title:          r.Title,
		Status:         string(r.Status),
		PapersFound:    r.PapersFoundCount,
		PapersIngested: r.PapersIngestedCount,
		KeywordsUsed:   r.KeywordsFoundCount,
		CreatedAt:      r.CreatedAt,
		CompletedAt:    r.CompletedAt,
	}
	if d := r.Duration(); d > 0 {
		resp.Duration = d.String()
	}
	return resp
}

func domainPaperToResponse(p *domain.Paper) paperResponse {
	authors := make([]authorResponse, len(p.Authors))
	for i, a := range p.Authors {
		authors[i] = authorResponse{
			Name:        a.Name,
			Affiliation: a.Affiliation,
			ORCID:       a.ORCID,
		}
	}
	return paperResponse{
		ID:              p.ID.String(),
		CanonicalID:     p.CanonicalID,
		Title:           p.Title,
		Abstract:        p.Abstract,
		Authors:         authors,
		PublicationDate: p.PublicationDate,
		PublicationYear: p.PublicationYear,
		Venue:           p.Venue,
		Journal:         p.Journal,
		CitationCount:   p.CitationCount,
		PdfURL:          p.PDFURL,
		OpenAccess:      p.OpenAccess,
	}
}

func domainKeywordToResponse(k *domain.Keyword) keywordResponse {
	return keywordResponse{
		ID:                k.ID.String(),
		Keyword:           k.Keyword,
		NormalizedKeyword: k.NormalizedKeyword,
	}
}
