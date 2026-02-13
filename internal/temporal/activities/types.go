// Package activities provides Temporal activity implementations for the
// literature review service pipeline.
//
// Activity inputs and outputs are defined as serializable structs that cross the
// Temporal serialization boundary. Each activity receives an input struct and
// returns an output struct (or error). All fields must be exported for JSON
// serialization by the Temporal SDK's default data converter.
package activities

import (
	"github.com/google/uuid"
	"github.com/helixir/literature-review-service/internal/domain"
)

// ExtractKeywordsInput contains the parameters for the keyword extraction activity.
type ExtractKeywordsInput struct {
	// Text is the input text to extract keywords from (query or abstract).
	Text string

	// Mode specifies the type of text being processed ("query" or "abstract").
	Mode string

	// MaxKeywords is the maximum number of keywords to extract.
	MaxKeywords int

	// MinKeywords is the minimum number of keywords to extract.
	MinKeywords int

	// ExistingKeywords are keywords already found (to avoid duplicates).
	ExistingKeywords []string

	// Context provides additional context about the research domain.
	Context string

	// OrgID is the organization ID for budget tracking (optional).
	OrgID string

	// ProjectID is the project ID for budget tracking (optional).
	ProjectID string

	// LeaseID is the budget lease ID for usage reporting (optional).
	// When set, budget usage events are emitted via the outbox after successful extraction.
	LeaseID string
}

// ExtractKeywordsOutput contains the results of the keyword extraction activity.
type ExtractKeywordsOutput struct {
	// Keywords is the list of extracted keywords/phrases.
	Keywords []string

	// Reasoning is the LLM's explanation of its keyword choices.
	Reasoning string

	// Model is the LLM model used for extraction.
	Model string

	// InputTokens is the number of input tokens consumed.
	InputTokens int

	// OutputTokens is the number of output tokens consumed.
	OutputTokens int
}

// SearchPapersInput contains the parameters for the paper search activity.
type SearchPapersInput struct {
	// Query is the search query string.
	Query string

	// Sources lists the paper sources to search.
	Sources []domain.SourceType

	// MaxResults is the maximum number of results to return per source.
	MaxResults int

	// IncludePreprints indicates whether to include preprint results.
	IncludePreprints bool

	// OpenAccessOnly indicates whether to restrict results to open access papers.
	OpenAccessOnly bool

	// MinCitations filters papers by minimum citation count.
	MinCitations int
}

// SearchPapersOutput contains the results of the paper search activity.
type SearchPapersOutput struct {
	// Papers is the list of papers found across all sources.
	Papers []*domain.Paper

	// TotalFound is the total number of papers found across all sources.
	TotalFound int

	// BySource maps each source to the count of papers found from that source.
	BySource map[domain.SourceType]int

	// Errors contains any errors encountered from individual sources.
	Errors []SourceError
}

// SourceError represents an error from a specific paper source during search.
type SourceError struct {
	// Source is the paper source that produced the error.
	Source domain.SourceType

	// Error is the error message from the source.
	Error string
}

// UpdateStatusInput contains the parameters for the review status update activity.
type UpdateStatusInput struct {
	// OrgID is the organization identifier.
	OrgID string

	// ProjectID is the project identifier.
	ProjectID string

	// RequestID is the review request identifier.
	RequestID uuid.UUID

	// Status is the new review status to set.
	Status domain.ReviewStatus

	// ErrorMsg contains an error message when transitioning to a failed state.
	ErrorMsg string
}

// SaveKeywordsInput contains the parameters for the keyword persistence activity.
type SaveKeywordsInput struct {
	// RequestID is the review request identifier.
	RequestID uuid.UUID

	// Keywords is the list of keyword strings to save.
	Keywords []string

	// ExtractionRound is the round number in which these keywords were extracted.
	ExtractionRound int

	// SourcePaperID is the ID of the paper from which keywords were extracted (nil for query-based extraction).
	SourcePaperID *uuid.UUID

	// SourceType indicates how the keywords were discovered (e.g., "query", "paper_keywords", "llm_extraction").
	SourceType string
}

// SaveKeywordsOutput contains the results of the keyword persistence activity.
type SaveKeywordsOutput struct {
	// KeywordIDs contains the UUIDs of the saved keyword records.
	KeywordIDs []uuid.UUID

	// KeywordIDMap maps keyword strings to their database UUIDs.
	// The keys are the original (non-normalized) keyword strings.
	KeywordIDMap map[string]uuid.UUID

	// NewCount is the number of newly created keywords (excludes duplicates).
	NewCount int
}

// SavePapersInput contains the parameters for the paper persistence activity.
type SavePapersInput struct {
	// RequestID is the review request identifier.
	RequestID uuid.UUID

	// OrgID is the organization identifier.
	OrgID string

	// ProjectID is the project identifier.
	ProjectID string

	// Papers is the list of papers to save.
	Papers []*domain.Paper

	// DiscoveredViaKeywordID is the keyword that led to discovering these papers.
	DiscoveredViaKeywordID *uuid.UUID

	// DiscoveredViaSource is the source that provided these papers.
	DiscoveredViaSource domain.SourceType

	// ExpansionDepth is the recursive expansion depth at which these papers were discovered.
	ExpansionDepth int
}

// SavePapersOutput contains the results of the paper persistence activity.
type SavePapersOutput struct {
	// SavedCount is the number of papers successfully saved.
	SavedCount int

	// DuplicateCount is the number of papers that were already present.
	DuplicateCount int

	// PaperIDs contains the database UUIDs of all upserted papers (both new and existing).
	// These IDs are needed by the workflow to create child workflow batches, since
	// papers from search sources arrive with uuid.Nil and get their IDs assigned
	// during BulkUpsert inside this activity.
	PaperIDs []uuid.UUID `json:"paper_ids"`
}

// IncrementCountersInput contains the parameters for the counter increment activity.
type IncrementCountersInput struct {
	// OrgID is the organization identifier.
	OrgID string

	// ProjectID is the project identifier.
	ProjectID string

	// RequestID is the review request identifier.
	RequestID uuid.UUID

	// PapersFound is the number of papers found to add to the counter.
	PapersFound int

	// PapersIngested is the number of papers ingested to add to the counter.
	PapersIngested int
}

// SubmitPaperForIngestionInput contains the parameters for submitting a single paper to the ingestion service.
type SubmitPaperForIngestionInput struct {
	// OrgID is the organization identifier.
	OrgID string

	// ProjectID is the project identifier.
	ProjectID string

	// RequestID is the literature review request identifier.
	RequestID uuid.UUID

	// PaperID is the paper's internal UUID.
	PaperID uuid.UUID

	// PDFURL is the URL to the paper's PDF.
	PDFURL string

	// IdempotencyKey is a unique key for deduplication.
	IdempotencyKey string

	// AccessTag is the access control tag for the ingested document (e.g., "lit_public", "lit_paywall").
	AccessTag string
}

// SubmitPaperForIngestionOutput contains the result of submitting a paper for ingestion.
type SubmitPaperForIngestionOutput struct {
	// RunID is the ingestion run identifier returned by the ingestion service.
	RunID string

	// IsExisting indicates whether this was an idempotent hit (run already existed).
	IsExisting bool

	// Status is the current status of the ingestion run.
	Status string
}

// CheckIngestionStatusInput contains the parameters for checking ingestion run status.
type CheckIngestionStatusInput struct {
	// RunID is the ingestion run identifier to check.
	RunID string
}

// CheckIngestionStatusOutput contains the result of checking ingestion status.
type CheckIngestionStatusOutput struct {
	// RunID is the ingestion run identifier.
	RunID string

	// Status is the current status string (e.g., "COMPLETED", "FAILED", "EXECUTING").
	Status string

	// IsTerminal indicates whether the status is a final state.
	IsTerminal bool

	// ErrorMessage contains the error message if the run failed.
	ErrorMessage string
}

// SubmitPapersForIngestionInput contains the parameters for batch paper ingestion submission.
type SubmitPapersForIngestionInput struct {
	// OrgID is the organization identifier.
	OrgID string

	// ProjectID is the project identifier.
	ProjectID string

	// RequestID is the literature review request identifier.
	RequestID uuid.UUID

	// Papers contains the papers to submit (only those with PDF URLs).
	Papers []PaperForIngestion

	// AccessTag is the access control tag for all papers in this batch (e.g., "lit_public", "lit_paywall").
	AccessTag string
}

// PaperForIngestion contains the minimum paper data needed for ingestion submission.
type PaperForIngestion struct {
	// PaperID is the paper's internal UUID.
	PaperID uuid.UUID

	// PDFURL is the URL to the paper's PDF.
	PDFURL string

	// CanonicalID is the paper's canonical identifier (used for idempotency).
	CanonicalID string

	// AccessTag is the per-paper access control tag (e.g., "lit_public", "lit_paywall").
	// Determined by whether the paper is open access.
	AccessTag string
}

// SubmitPapersForIngestionOutput contains the results of batch paper ingestion submission.
type SubmitPapersForIngestionOutput struct {
	// Submitted is the number of papers successfully submitted.
	Submitted int

	// Skipped is the number of papers skipped (already ingested or no PDF).
	Skipped int

	// Failed is the number of papers that failed submission.
	Failed int

	// RunIDs maps paper IDs to their ingestion run IDs.
	RunIDs map[string]string
}

// DedupPapersInput contains the parameters for the batch dedup activity.
type DedupPapersInput struct {
	// Papers to check for duplicates.
	Papers []*domain.Paper
}

// DedupPapersOutput contains the dedup results.
type DedupPapersOutput struct {
	// NonDuplicateIDs are paper IDs that passed the dedup check.
	NonDuplicateIDs []uuid.UUID

	// DuplicateCount is the number of duplicates found.
	DuplicateCount int

	// SkippedCount is papers skipped (no abstract).
	SkippedCount int
}

// DownloadAndIngestInput contains the parameters for the download and ingest activity.
type DownloadAndIngestInput struct {
	// OrgID is the organization identifier.
	OrgID string

	// ProjectID is the project identifier.
	ProjectID string

	// RequestID is the literature review request identifier.
	RequestID uuid.UUID

	// Papers contains the papers to download and ingest.
	// Each paper carries its own AccessTag for per-paper access classification.
	Papers []PaperForIngestion
}

// DownloadAndIngestOutput contains the results of the download and ingest activity.
type DownloadAndIngestOutput struct {
	// Successful is the count of successfully processed papers.
	Successful int

	// Failed is the count of papers that failed processing.
	Failed int

	// Skipped is the count of papers without PDF URLs.
	Skipped int

	// Results contains per-paper results.
	Results []PaperIngestionResult
}

// PaperIngestionResult contains the result of downloading and ingesting a single paper.
type PaperIngestionResult struct {
	// PaperID is the paper's internal UUID.
	PaperID uuid.UUID

	// FileID is the file_service UUID (empty if failed).
	FileID string

	// IngestionRunID is the ingestion service run ID (empty if failed).
	IngestionRunID string

	// Status is the ingestion run status.
	Status string

	// Error contains the error message if processing failed.
	Error string
}

// UpdatePaperIngestionResultsInput contains the parameters for updating paper ingestion results.
type UpdatePaperIngestionResultsInput struct {
	// Results contains the per-paper ingestion results to save.
	Results []PaperIngestionResult
}

// UpdatePaperIngestionResultsOutput contains the results of updating paper ingestion results.
type UpdatePaperIngestionResultsOutput struct {
	// Updated is the count of papers successfully updated.
	Updated int

	// Skipped is the count of papers skipped (no file_id or ingestion_run_id).
	Skipped int

	// Failed is the count of papers that failed to update.
	Failed int
}

// EmbedPapersInput contains the parameters for the batch embedding activity.
type EmbedPapersInput struct {
	// Papers contains the papers to embed.
	Papers []PaperForEmbedding `json:"papers"`
}

// PaperForEmbedding contains the minimum paper data needed for embedding.
type PaperForEmbedding struct {
	// PaperID is the paper's internal UUID.
	PaperID uuid.UUID `json:"paper_id"`

	// CanonicalID is the paper's canonical identifier.
	CanonicalID string `json:"canonical_id"`

	// Abstract is the text to embed.
	Abstract string `json:"abstract"`
}

// EmbedPapersOutput contains the results of the batch embedding activity.
type EmbedPapersOutput struct {
	// Embeddings maps canonical ID to embedding vector.
	Embeddings map[string][]float32 `json:"embeddings"`

	// Skipped is the count of papers skipped (no abstract).
	Skipped int `json:"skipped"`

	// Failed is the count of papers that failed embedding.
	Failed int `json:"failed"`
}

// SearchSingleSourceInput contains the parameters for searching a single paper source.
type SearchSingleSourceInput struct {
	// Source is the paper source to search.
	Source domain.SourceType `json:"source"`
	// Query is the search query string.
	Query string `json:"query"`
	// MaxResults is the maximum number of results to return.
	MaxResults int `json:"max_results"`
	// IncludePreprints indicates whether to include preprint results.
	IncludePreprints bool `json:"include_preprints"`
	// OpenAccessOnly indicates whether to restrict results to open access papers.
	OpenAccessOnly bool `json:"open_access_only"`
	// MinCitations filters papers by minimum citation count.
	MinCitations int `json:"min_citations"`
	// DateFrom filters papers published on or after this date (YYYY-MM-DD or RFC3339).
	// Used for incremental searches after cached results.
	DateFrom *string `json:"date_from,omitempty"`
	// DateTo filters papers published on or before this date (YYYY-MM-DD or RFC3339).
	DateTo *string `json:"date_to,omitempty"`
}

// SearchSingleSourceOutput contains the results of searching a single paper source.
type SearchSingleSourceOutput struct {
	// Source is the paper source that was searched.
	Source domain.SourceType `json:"source"`
	// Papers is the list of papers found.
	Papers []*domain.Paper `json:"papers"`
	// TotalFound is the count of papers found.
	TotalFound int `json:"total_found"`
	// Error contains error message if the search failed.
	Error string `json:"error"`
}

// PaperWithEmbedding contains paper data with its embedding for dedup.
type PaperWithEmbedding struct {
	// PaperID is the paper's internal UUID.
	PaperID uuid.UUID `json:"paper_id"`
	// CanonicalID is the paper's canonical identifier.
	CanonicalID string `json:"canonical_id"`
	// Embedding is the paper's abstract embedding.
	Embedding []float32 `json:"embedding"`
}

// BatchDedupInput contains the parameters for batch dedup activity.
type BatchDedupInput struct {
	// Papers contains papers with embeddings to check for duplicates.
	Papers []*PaperWithEmbedding `json:"papers"`
}

// BatchDedupOutput contains the results of batch dedup activity.
type BatchDedupOutput struct {
	// NonDuplicateIDs are paper IDs that passed the dedup check.
	NonDuplicateIDs []uuid.UUID `json:"non_duplicate_ids"`
	// DuplicateCount is the number of duplicates found.
	DuplicateCount int `json:"duplicate_count"`
}

// UpdatePauseStateInput contains the data needed to update a review's pause state.
type UpdatePauseStateInput struct {
	// OrgID is the organization identifier.
	OrgID string

	// ProjectID is the project identifier.
	ProjectID string

	// RequestID is the review request identifier.
	RequestID uuid.UUID

	// Status is the new review status (typically ReviewStatusPaused).
	Status domain.ReviewStatus

	// PauseReason indicates why the workflow was paused.
	PauseReason domain.PauseReason

	// PausedAtPhase records which workflow phase was active when paused.
	PausedAtPhase string
}

// --- Relevance Gate & Coverage Review Types ---

// EmbedTextInput contains the parameters for embedding a single text string.
type EmbedTextInput struct {
	// Text is the text to embed.
	Text string `json:"text"`
}

// EmbedTextOutput contains the embedding result for a single text.
type EmbedTextOutput struct {
	// Embedding is the vector representation of the text.
	Embedding []float32 `json:"embedding"`
}

// ScoreKeywordRelevanceInput contains the parameters for scoring keyword relevance.
type ScoreKeywordRelevanceInput struct {
	// Keywords are the keywords to score.
	Keywords []string `json:"keywords"`
	// QueryEmbedding is the embedding of the original query (anchor).
	QueryEmbedding []float32 `json:"query_embedding"`
	// Threshold is the minimum cosine similarity to accept a keyword.
	Threshold float64 `json:"threshold"`
}

// ScoredKeyword contains a keyword with its relevance score.
type ScoredKeyword struct {
	// Keyword is the keyword text.
	Keyword string `json:"keyword"`
	// Score is the cosine similarity to the query embedding.
	Score float64 `json:"score"`
}

// ScoreKeywordRelevanceOutput contains the scored and filtered keywords.
type ScoreKeywordRelevanceOutput struct {
	// Accepted are keywords above the threshold.
	Accepted []ScoredKeyword `json:"accepted"`
	// Rejected are keywords below the threshold.
	Rejected []ScoredKeyword `json:"rejected"`
}

// PaperSummary contains the minimum paper data for coverage assessment.
type PaperSummary struct {
	// Title is the paper title.
	Title string `json:"title"`
	// Abstract is the paper abstract (may be truncated).
	Abstract string `json:"abstract"`
}

// AssessCoverageInput contains the parameters for LLM coverage assessment.
type AssessCoverageInput struct {
	// Title is the research topic title.
	Title string `json:"title"`
	// Description is the extended review scope description.
	Description string `json:"description"`
	// SeedKeywords are the user-provided starting keywords.
	SeedKeywords []string `json:"seed_keywords"`
	// AllKeywords are all keywords extracted across all rounds.
	AllKeywords []string `json:"all_keywords"`
	// PaperSummaries are title+abstract pairs for up to 20 papers.
	PaperSummaries []PaperSummary `json:"paper_summaries"`
	// TotalPapers is the total number of papers found.
	TotalPapers int `json:"total_papers"`
	// ExpansionRounds is the number of expansion rounds completed.
	ExpansionRounds int `json:"expansion_rounds"`
	// OrgID for budget tracking.
	OrgID string `json:"org_id"`
	// ProjectID for budget tracking.
	ProjectID string `json:"project_id"`
	// LeaseID for budget usage reporting.
	LeaseID string `json:"lease_id"`
}

// AssessCoverageOutput contains the LLM coverage assessment results.
type AssessCoverageOutput struct {
	// CoverageScore is the assessed coverage level (0.0-1.0).
	CoverageScore float64 `json:"coverage_score"`
	// Reasoning is the LLM's explanation of its assessment.
	Reasoning string `json:"reasoning"`
	// GapTopics are research subtopics not well-represented in the corpus.
	GapTopics []string `json:"gap_topics"`
	// IsSufficient indicates whether the corpus supports a reasonable review.
	IsSufficient bool `json:"is_sufficient"`
	// Model is the LLM model used.
	Model string `json:"model"`
	// InputTokens consumed.
	InputTokens int `json:"input_tokens"`
	// OutputTokens consumed.
	OutputTokens int `json:"output_tokens"`
}

// --- Search Deduplication Types ---

// CheckSearchCompletedInput contains the parameters for checking if a search was already done.
type CheckSearchCompletedInput struct {
	// ReviewID is the literature review request UUID for tenant isolation.
	ReviewID uuid.UUID `json:"review_id"`

	// KeywordID is the keyword's database UUID.
	KeywordID uuid.UUID `json:"keyword_id"`

	// Keyword is the keyword string (for logging).
	Keyword string `json:"keyword"`

	// Source is the paper source API to check.
	Source domain.SourceType `json:"source"`

	// DateFrom is the optional start of the date range filter.
	DateFrom *string `json:"date_from,omitempty"`

	// DateTo is the optional end of the date range filter.
	DateTo *string `json:"date_to,omitempty"`
}

// CheckSearchCompletedOutput contains the result of checking for a completed search.
type CheckSearchCompletedOutput struct {
	// AlreadyCompleted is true if a completed search record exists for this keyword+source+dateRange.
	AlreadyCompleted bool `json:"already_completed"`

	// PreviouslyFoundPapers are papers from the cached search result.
	PreviouslyFoundPapers []*domain.Paper `json:"previously_found_papers,omitempty"`

	// PapersFoundCount is the number of papers found in the previous search.
	PapersFoundCount int `json:"papers_found_count"`

	// SearchedAt is when the previous search was performed.
	SearchedAt string `json:"searched_at,omitempty"`
}

// RecordSearchResultInput contains the parameters for recording a completed search.
type RecordSearchResultInput struct {
	// KeywordID is the keyword's database UUID.
	KeywordID uuid.UUID `json:"keyword_id"`

	// Source is the paper source API that was searched.
	Source domain.SourceType `json:"source"`

	// DateFrom is the optional start of the date range filter.
	DateFrom *string `json:"date_from,omitempty"`

	// DateTo is the optional end of the date range filter.
	DateTo *string `json:"date_to,omitempty"`

	// PapersFound is the number of papers found.
	PapersFound int `json:"papers_found"`

	// PaperIDs are the IDs of papers found (for keyword_paper_mappings).
	PaperIDs []uuid.UUID `json:"paper_ids"`

	// Status is the search status (completed or failed).
	Status domain.SearchStatus `json:"status"`

	// ErrorMessage is set when the search failed.
	ErrorMessage string `json:"error_message,omitempty"`
}

// FetchPaperBatchInput contains the parameters for fetching a batch of papers by ID.
type FetchPaperBatchInput struct {
	// PaperIDs are the UUIDs of papers to fetch from the database.
	PaperIDs []uuid.UUID `json:"paper_ids"`
}

// FetchPaperBatchOutput contains the fetched papers.
type FetchPaperBatchOutput struct {
	// Papers are the fetched paper details for processing.
	Papers []PaperForProcessing `json:"papers"`
}

// PaperForProcessing contains paper data needed for the processing pipeline.
// This is the canonical type for paper data in the processing pipeline.
type PaperForProcessing struct {
	// PaperID is the paper's internal UUID.
	PaperID uuid.UUID `json:"paper_id"`
	// CanonicalID is the paper's canonical identifier (DOI, PMID, etc.).
	CanonicalID string `json:"canonical_id"`
	// Title is the paper title.
	Title string `json:"title"`
	// Abstract is the paper abstract (for embedding).
	Abstract string `json:"abstract"`
	// PDFURL is the URL to download the PDF.
	PDFURL string `json:"pdf_url"`
	// Authors is the list of author names.
	Authors []string `json:"authors"`
	// OpenAccess indicates whether the paper is freely accessible.
	// Used to determine the access_tag when submitting for ingestion.
	OpenAccess bool `json:"open_access"`
}

// KeywordPaperMappingEntry represents a batch of papers found for a keyword from a specific source.
type KeywordPaperMappingEntry struct {
	// KeywordID is the keyword's database UUID.
	KeywordID uuid.UUID `json:"keyword_id"`
	// Source is the paper source that discovered these papers.
	Source domain.SourceType `json:"source"`
	// PaperIDs are the UUIDs of papers found for this keyword+source combination.
	PaperIDs []uuid.UUID `json:"paper_ids"`
}

// BulkCreateKeywordPaperMappingsInput contains the parameters for bulk-creating keyword-paper mappings.
type BulkCreateKeywordPaperMappingsInput struct {
	// Entries are the keyword-source-papers mapping entries to create.
	Entries []KeywordPaperMappingEntry `json:"entries"`
}
