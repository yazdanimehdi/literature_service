package domain

import (
	"time"

	"github.com/google/uuid"
)

// ReviewConfiguration holds the configuration parameters for a literature review.
// This struct is stored as JSONB in PostgreSQL for flexibility and auditability.
type ReviewConfiguration struct {
	// MaxPapers is the maximum total number of papers to retrieve.
	MaxPapers int `json:"max_papers"`

	// MaxExpansionDepth is the maximum recursive search depth.
	MaxExpansionDepth int `json:"max_expansion_depth"`

	// MaxKeywordsPerRound limits keywords extracted in the initial round.
	MaxKeywordsPerRound int `json:"max_keywords_per_round,omitempty"`

	// PaperKeywordCount limits keywords extracted per paper during expansion rounds.
	// If zero, defaults to MaxKeywordsPerRound.
	PaperKeywordCount int `json:"paper_keyword_count,omitempty"`

	// Sources lists the paper sources to search.
	Sources []SourceType `json:"sources,omitempty"`

	// DateFrom is the earliest publication date to include.
	DateFrom *time.Time `json:"date_from,omitempty"`

	// DateTo is the latest publication date to include.
	DateTo *time.Time `json:"date_to,omitempty"`

	// IncludePreprints indicates whether to include preprints.
	IncludePreprints bool `json:"include_preprints"`

	// RequireOpenAccess indicates whether to only include open access papers.
	RequireOpenAccess bool `json:"require_open_access"`

	// MinCitations filters papers by minimum citation count.
	MinCitations int `json:"min_citations,omitempty"`

	// EnableRelevanceGate enables embedding-based keyword drift prevention
	// during expansion rounds.
	EnableRelevanceGate bool `json:"enable_relevance_gate"`

	// RelevanceThreshold is the minimum cosine similarity (0.0-1.0) for a
	// keyword to pass the relevance gate. Default: 0.3.
	RelevanceThreshold float64 `json:"relevance_threshold,omitempty"`

	// EnableCoverageReview enables LLM-based corpus coverage assessment
	// after expansion rounds complete.
	EnableCoverageReview bool `json:"enable_coverage_review"`

	// CoverageThreshold is the minimum coverage score (0.0-1.0) below which
	// the workflow will auto-trigger one more expansion round. Default: 0.7.
	CoverageThreshold float64 `json:"coverage_threshold,omitempty"`

	// LLMModel specifies the LLM to use for keyword extraction.
	LLMModel string `json:"llm_model,omitempty"`

	// Custom holds any additional custom configuration.
	Custom map[string]interface{} `json:"custom,omitempty"`
}

// DefaultReviewConfiguration returns a ReviewConfiguration with default values.
func DefaultReviewConfiguration() ReviewConfiguration {
	return ReviewConfiguration{
		MaxPapers:           100,
		MaxExpansionDepth:   2,
		MaxKeywordsPerRound: 10,
		Sources: []SourceType{
			SourceTypeSemanticScholar,
			SourceTypeOpenAlex,
			SourceTypePubMed,
		},
		IncludePreprints:     true,
		RequireOpenAccess:    false,
		MinCitations:         0,
		EnableRelevanceGate:  true,
		RelevanceThreshold:   0.3,
		EnableCoverageReview: false,
		CoverageThreshold:    0.7,
	}
}

// SourceFilters holds optional filters for specific paper sources.
// Stored as JSONB in PostgreSQL for flexibility.
type SourceFilters struct {
	// SemanticScholar holds Semantic Scholar-specific filters.
	SemanticScholar *SemanticScholarFilters `json:"semantic_scholar,omitempty"`

	// OpenAlex holds OpenAlex-specific filters.
	OpenAlex *OpenAlexFilters `json:"openalex,omitempty"`

	// PubMed holds PubMed-specific filters.
	PubMed *PubMedFilters `json:"pubmed,omitempty"`
}

// SemanticScholarFilters holds Semantic Scholar-specific search filters.
type SemanticScholarFilters struct {
	FieldsOfStudy []string `json:"fields_of_study,omitempty"`
	OpenAccessPDF bool     `json:"open_access_pdf,omitempty"`
}

// OpenAlexFilters holds OpenAlex-specific search filters.
type OpenAlexFilters struct {
	Concepts []string `json:"concepts,omitempty"`
	Types    []string `json:"types,omitempty"`
}

// PubMedFilters holds PubMed-specific search filters.
type PubMedFilters struct {
	MeshTerms     []string `json:"mesh_terms,omitempty"`
	PublicationType []string `json:"publication_type,omitempty"`
}

// LiteratureReviewRequest represents a user's request for a literature review.
type LiteratureReviewRequest struct {
	ID uuid.UUID `json:"id"`

	// Tenant context (multi-tenancy)
	OrgID     string `json:"org_id"`
	ProjectID string `json:"project_id"`
	UserID    string `json:"user_id"`

	// Title is the research topic title (required).
	Title string `json:"title"`

	// Description is an extended description of the review scope (optional).
	Description string `json:"description,omitempty"`

	// SeedKeywords are user-provided starting keywords (optional).
	SeedKeywords []string `json:"seed_keywords,omitempty"`

	// Temporal workflow tracking
	TemporalWorkflowID string `json:"temporal_workflow_id,omitempty"`
	TemporalRunID      string `json:"temporal_run_id,omitempty"`

	// Status and progress
	Status              ReviewStatus `json:"status"`
	ErrorMessage        string       `json:"error_message,omitempty"`
	KeywordsFoundCount  int          `json:"keywords_found_count"`
	PapersFoundCount    int          `json:"papers_found_count"`
	PapersIngestedCount int          `json:"papers_ingested_count"`
	PapersFailedCount   int          `json:"papers_failed_count"`

	// CoverageScore is the LLM-assessed corpus coverage (0.0-1.0).
	// Nil/zero if coverage review was not enabled.
	CoverageScore *float64 `json:"coverage_score,omitempty"`

	// CoverageReasoning is the LLM's assessment of coverage strengths and gaps.
	CoverageReasoning string `json:"coverage_reasoning,omitempty"`

	// Configuration (stored as JSONB)
	Configuration ReviewConfiguration `json:"configuration"`
	SourceFilters *SourceFilters      `json:"source_filters,omitempty"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Pause state
	PauseReason   PauseReason `json:"pause_reason,omitempty"`
	PausedAt      *time.Time  `json:"paused_at,omitempty"`
	PausedAtPhase string      `json:"paused_at_phase,omitempty"`
}

// Duration returns the duration of the review request.
// Returns zero if the request has not started.
// Returns elapsed time from start if still running.
// Returns total duration if completed.
func (r *LiteratureReviewRequest) Duration() time.Duration {
	if r.StartedAt == nil {
		return 0
	}

	if r.CompletedAt != nil {
		return r.CompletedAt.Sub(*r.StartedAt)
	}

	return time.Since(*r.StartedAt)
}

// IsActive returns true if the review request is still in progress.
func (r *LiteratureReviewRequest) IsActive() bool {
	return !r.Status.IsTerminal()
}

// GetTenant returns the tenant context for this request.
func (r *LiteratureReviewRequest) GetTenant() Tenant {
	return Tenant{
		OrgID:     r.OrgID,
		ProjectID: r.ProjectID,
		UserID:    r.UserID,
	}
}

// RequestKeywordMapping links a review request to a discovered keyword.
type RequestKeywordMapping struct {
	ID              uuid.UUID  `json:"id"`
	RequestID       uuid.UUID  `json:"request_id"`
	KeywordID       uuid.UUID  `json:"keyword_id"`
	ExtractionRound int        `json:"extraction_round"`
	SourcePaperID   *uuid.UUID `json:"source_paper_id,omitempty"`
	SourceType      string     `json:"source_type"` // "query", "paper_keywords", "llm_extraction"
	CreatedAt       time.Time  `json:"created_at"`
}

// RequestPaperMapping links a review request to a discovered paper.
type RequestPaperMapping struct {
	ID                     uuid.UUID  `json:"id"`
	RequestID              uuid.UUID  `json:"request_id"`
	PaperID                uuid.UUID  `json:"paper_id"`
	DiscoveredViaKeywordID *uuid.UUID `json:"discovered_via_keyword_id,omitempty"`
	DiscoveredViaSource    SourceType `json:"discovered_via_source"`
	ExpansionDepth         int        `json:"expansion_depth"`

	// Ingestion tracking
	IngestionStatus IngestionStatus `json:"ingestion_status"`
	IngestionJobID  string          `json:"ingestion_job_id,omitempty"`
	IngestionError  string          `json:"ingestion_error,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ReviewProgressEvent represents a real-time progress event for UI updates.
// These events are streamed to clients via the gRPC streaming endpoint.
type ReviewProgressEvent struct {
	// ID is the unique identifier for this progress event.
	ID uuid.UUID `json:"id"`

	// RequestID references the literature review request this event belongs to.
	RequestID uuid.UUID `json:"request_id"`

	// EventType describes the kind of progress event (e.g., "stream_started", "progress_update", "completed").
	EventType string `json:"event_type"`

	// EventData holds the event-specific payload as a flexible JSON object.
	EventData map[string]interface{} `json:"event_data"`

	// CreatedAt records when this progress event was generated.
	CreatedAt time.Time `json:"created_at"`
}

// ReviewProgress represents the current progress of a literature review.
// This is a snapshot of the review's state used for progress reporting and streaming.
type ReviewProgress struct {
	// RequestID references the literature review request.
	RequestID uuid.UUID `json:"request_id"`

	// Status is the current review lifecycle status.
	Status ReviewStatus `json:"status"`

	// CurrentPhase describes the active processing phase in human-readable form.
	CurrentPhase string `json:"current_phase"`

	// KeywordsFound is the total number of keywords extracted so far.
	KeywordsFound int `json:"keywords_found"`

	// KeywordsSearched is the number of keywords that have been searched across sources.
	KeywordsSearched int `json:"keywords_searched"`

	// PapersFound is the total number of unique papers discovered.
	PapersFound int `json:"papers_found"`

	// PapersIngested is the number of papers successfully submitted for ingestion.
	PapersIngested int `json:"papers_ingested"`

	// PapersFailed is the number of papers that failed during ingestion.
	PapersFailed int `json:"papers_failed"`

	// SourceProgress provides per-source search progress, keyed by source type.
	SourceProgress map[SourceType]*SourceProgress `json:"source_progress,omitempty"`

	// StartedAt records when the review processing began.
	StartedAt time.Time `json:"started_at"`

	// EstimatedEndAt is the estimated completion time, if available.
	EstimatedEndAt *time.Time `json:"estimated_end_at,omitempty"`

	// LastUpdatedAt records when the progress was last updated.
	LastUpdatedAt time.Time `json:"last_updated_at"`
}

// SourceProgress represents the progress of searches for a specific paper source API.
type SourceProgress struct {
	// Source identifies the paper source API.
	Source SourceType `json:"source"`

	// Searched is the number of keyword searches completed against this source.
	Searched int `json:"searched"`

	// PapersFound is the number of papers discovered from this source.
	PapersFound int `json:"papers_found"`

	// Status is the overall search status for this source.
	Status SearchStatus `json:"status"`

	// ErrorMessage contains error details if the source encountered a failure; empty otherwise.
	ErrorMessage string `json:"error_message,omitempty"`
}
