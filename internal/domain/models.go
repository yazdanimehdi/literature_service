// Package domain provides domain models and business logic for the Literature Review Service.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// ReviewStatus represents the lifecycle states of a literature review request.
// These values must match the database enum review_status.
type ReviewStatus string

const (
	// ReviewStatusPending indicates the review has been created but not yet started.
	ReviewStatusPending ReviewStatus = "pending"

	// ReviewStatusExtractingKeywords indicates the LLM is extracting search keywords from the query.
	ReviewStatusExtractingKeywords ReviewStatus = "extracting_keywords"

	// ReviewStatusSearching indicates paper sources are being queried with extracted keywords.
	ReviewStatusSearching ReviewStatus = "searching"

	// ReviewStatusExpanding indicates a recursive expansion round is in progress,
	// extracting new keywords from discovered papers.
	ReviewStatusExpanding ReviewStatus = "expanding"

	// ReviewStatusIngesting indicates discovered papers are being submitted to the ingestion service.
	ReviewStatusIngesting ReviewStatus = "ingesting"

	// ReviewStatusReviewing indicates the LLM is assessing corpus coverage
	// against the original research intent.
	ReviewStatusReviewing ReviewStatus = "reviewing"

	// ReviewStatusCompleted indicates the review finished successfully with all papers processed.
	ReviewStatusCompleted ReviewStatus = "completed"

	// ReviewStatusPartial indicates the review finished but some operations failed or were skipped.
	ReviewStatusPartial ReviewStatus = "partial"

	// ReviewStatusFailed indicates the review encountered an unrecoverable error.
	ReviewStatusFailed ReviewStatus = "failed"

	// ReviewStatusCancelled indicates the review was cancelled by a user or system action.
	ReviewStatusCancelled ReviewStatus = "cancelled"

	// ReviewStatusPaused indicates the review has been paused by user or budget exhaustion.
	ReviewStatusPaused ReviewStatus = "paused"
)

// IsTerminal returns true if the status represents a final state that will not change.
func (s ReviewStatus) IsTerminal() bool {
	switch s {
	case ReviewStatusCompleted, ReviewStatusPartial, ReviewStatusFailed, ReviewStatusCancelled:
		return true
	default:
		return false
	}
}

// PauseReason indicates why a literature review workflow was paused.
// These values must match the database enum pause_reason.
type PauseReason string

const (
	// PauseReasonUser indicates the workflow was manually paused by a user.
	PauseReasonUser PauseReason = "user"

	// PauseReasonBudgetExhausted indicates the workflow was paused because
	// the project ran out of LLM credits or budget allocation.
	PauseReasonBudgetExhausted PauseReason = "budget_exhausted"
)

// SearchStatus represents the state of a keyword search operation.
// These values must match the database enum search_status.
type SearchStatus string

const (
	// SearchStatusPending indicates the search has not yet been executed.
	SearchStatusPending SearchStatus = "pending"

	// SearchStatusInProgress indicates the search is currently running against a paper source.
	SearchStatusInProgress SearchStatus = "in_progress"

	// SearchStatusCompleted indicates the search finished successfully.
	SearchStatusCompleted SearchStatus = "completed"

	// SearchStatusFailed indicates the search encountered an error.
	SearchStatusFailed SearchStatus = "failed"

	// SearchStatusRateLimited indicates the search was throttled by the paper source API.
	SearchStatusRateLimited SearchStatus = "rate_limited"
)

// IngestionStatus represents the ingestion state of a paper within a review.
// These values must match the database enum ingestion_status.
type IngestionStatus string

const (
	// IngestionStatusPending indicates the paper has not yet been submitted for ingestion.
	IngestionStatusPending IngestionStatus = "pending"

	// IngestionStatusSubmitted indicates the paper has been submitted to the ingestion service.
	IngestionStatusSubmitted IngestionStatus = "submitted"

	// IngestionStatusProcessing indicates the ingestion service is actively processing the paper.
	IngestionStatusProcessing IngestionStatus = "processing"

	// IngestionStatusCompleted indicates the paper was successfully ingested.
	IngestionStatusCompleted IngestionStatus = "completed"

	// IngestionStatusFailed indicates the ingestion service failed to process the paper.
	IngestionStatusFailed IngestionStatus = "failed"

	// IngestionStatusSkipped indicates the paper was skipped (e.g., no PDF URL available).
	IngestionStatusSkipped IngestionStatus = "skipped"
)

// MappingType represents how a keyword was associated with a paper.
// These values must match the database enum mapping_type.
type MappingType string

const (
	// MappingTypeAuthorKeyword indicates the keyword was provided by the paper's authors.
	MappingTypeAuthorKeyword MappingType = "author_keyword"

	// MappingTypeMeshTerm indicates the keyword is a MeSH (Medical Subject Headings) term from PubMed.
	MappingTypeMeshTerm MappingType = "mesh_term"

	// MappingTypeExtracted indicates the keyword was extracted by the LLM from paper content.
	MappingTypeExtracted MappingType = "extracted"

	// MappingTypeQueryMatch indicates the keyword matched the original user query.
	MappingTypeQueryMatch MappingType = "query_match"
)

// SourceType represents the source API that provided paper data.
// These values must match the database enum source_type.
type SourceType string

const (
	// SourceTypeSemanticScholar identifies the Semantic Scholar API as the paper source.
	SourceTypeSemanticScholar SourceType = "semantic_scholar"

	// SourceTypeOpenAlex identifies the OpenAlex API as the paper source.
	SourceTypeOpenAlex SourceType = "openalex"

	// SourceTypeScopus identifies the Scopus (Elsevier) API as the paper source.
	SourceTypeScopus SourceType = "scopus"

	// SourceTypePubMed identifies the PubMed/NCBI API as the paper source.
	SourceTypePubMed SourceType = "pubmed"

	// SourceTypeBioRxiv identifies the bioRxiv preprint server as the paper source.
	SourceTypeBioRxiv SourceType = "biorxiv"

	// SourceTypeArXiv identifies the arXiv preprint repository as the paper source.
	SourceTypeArXiv SourceType = "arxiv"
)

// validSourceTypes is the set of known source types for validation.
var validSourceTypes = map[SourceType]bool{
	SourceTypeSemanticScholar: true,
	SourceTypeOpenAlex:        true,
	SourceTypeScopus:          true,
	SourceTypePubMed:          true,
	SourceTypeBioRxiv:         true,
	SourceTypeArXiv:           true,
}

// IsValidSourceType returns true if st is a recognized source type.
func IsValidSourceType(st SourceType) bool {
	return validSourceTypes[st]
}

// IdentifierType represents the type of academic paper identifier.
// These values must match the database enum identifier_type.
type IdentifierType string

const (
	// IdentifierTypeDOI represents a Digital Object Identifier.
	IdentifierTypeDOI IdentifierType = "doi"

	// IdentifierTypeArXivID represents an arXiv paper identifier.
	IdentifierTypeArXivID IdentifierType = "arxiv_id"

	// IdentifierTypePubMedID represents a PubMed identifier (PMID).
	IdentifierTypePubMedID IdentifierType = "pubmed_id"

	// IdentifierTypeSemanticScholarID represents a Semantic Scholar corpus identifier.
	IdentifierTypeSemanticScholarID IdentifierType = "semantic_scholar_id"

	// IdentifierTypeOpenAlexID represents an OpenAlex work identifier.
	IdentifierTypeOpenAlexID IdentifierType = "openalex_id"

	// IdentifierTypeScopusID represents a Scopus document identifier.
	IdentifierTypeScopusID IdentifierType = "scopus_id"
)

// Tenant represents the multi-tenancy context for a request.
// Every operation in the service is scoped to a specific org, project, and user.
type Tenant struct {
	// OrgID is the organization identifier from the authentication context.
	OrgID string

	// ProjectID is the project identifier within the organization.
	ProjectID string

	// UserID is the identifier of the user who initiated the request.
	UserID string
}

// TenantFromIDs creates a Tenant from string IDs.
func TenantFromIDs(orgID, projectID, userID string) Tenant {
	return Tenant{
		OrgID:     orgID,
		ProjectID: projectID,
		UserID:    userID,
	}
}

// PaperIdentifier represents a paper identifier stored in the paper_identifiers table.
// Each paper may have multiple identifiers from different sources (DOI, arXiv, PubMed, etc.).
type PaperIdentifier struct {
	// ID is the primary key for this identifier record.
	ID uuid.UUID

	// PaperID references the paper that owns this identifier.
	PaperID uuid.UUID

	// IdentifierType indicates the kind of identifier (e.g., DOI, arXiv, PubMed).
	IdentifierType IdentifierType

	// IdentifierValue is the actual identifier string (e.g., "10.1234/example").
	IdentifierValue string

	// SourceAPI indicates which paper source API provided this identifier.
	SourceAPI SourceType

	// DiscoveredAt records when this identifier was first discovered.
	DiscoveredAt time.Time
}

// PaperSource represents a source that provided data for a paper.
// A paper may be discovered by multiple sources; each discovery is recorded separately.
type PaperSource struct {
	// ID is the primary key for this source record.
	ID uuid.UUID

	// PaperID references the paper that was discovered by this source.
	PaperID uuid.UUID

	// SourceAPI identifies which paper source API provided the data.
	SourceAPI SourceType

	// SourceMetadata holds source-specific metadata as a JSON object (e.g., relevance score, result rank).
	SourceMetadata map[string]interface{}

	// CreatedAt records when this source record was created.
	CreatedAt time.Time

	// UpdatedAt records when this source record was last updated.
	UpdatedAt time.Time
}
