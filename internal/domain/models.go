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
	ReviewStatusPending            ReviewStatus = "pending"
	ReviewStatusExtractingKeywords ReviewStatus = "extracting_keywords"
	ReviewStatusSearching          ReviewStatus = "searching"
	ReviewStatusExpanding          ReviewStatus = "expanding"
	ReviewStatusIngesting          ReviewStatus = "ingesting"
	ReviewStatusCompleted          ReviewStatus = "completed"
	ReviewStatusPartial            ReviewStatus = "partial"
	ReviewStatusFailed             ReviewStatus = "failed"
	ReviewStatusCancelled          ReviewStatus = "cancelled"
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

// SearchStatus represents the state of a keyword search operation.
// These values must match the database enum search_status.
type SearchStatus string

const (
	SearchStatusPending     SearchStatus = "pending"
	SearchStatusInProgress  SearchStatus = "in_progress"
	SearchStatusCompleted   SearchStatus = "completed"
	SearchStatusFailed      SearchStatus = "failed"
	SearchStatusRateLimited SearchStatus = "rate_limited"
)

// IngestionStatus represents the ingestion state of a paper within a review.
// These values must match the database enum ingestion_status.
type IngestionStatus string

const (
	IngestionStatusPending    IngestionStatus = "pending"
	IngestionStatusSubmitted  IngestionStatus = "submitted"
	IngestionStatusProcessing IngestionStatus = "processing"
	IngestionStatusCompleted  IngestionStatus = "completed"
	IngestionStatusFailed     IngestionStatus = "failed"
	IngestionStatusSkipped    IngestionStatus = "skipped"
)

// MappingType represents how a keyword was associated with a paper.
// These values must match the database enum mapping_type.
type MappingType string

const (
	MappingTypeAuthorKeyword MappingType = "author_keyword"
	MappingTypeMeshTerm      MappingType = "mesh_term"
	MappingTypeExtracted     MappingType = "extracted"
	MappingTypeQueryMatch    MappingType = "query_match"
)

// SourceType represents the source API that provided paper data.
// These values must match the database enum source_type.
type SourceType string

const (
	SourceTypeSemanticScholar SourceType = "semantic_scholar"
	SourceTypeOpenAlex        SourceType = "openalex"
	SourceTypeScopus          SourceType = "scopus"
	SourceTypePubMed          SourceType = "pubmed"
	SourceTypeBioRxiv         SourceType = "biorxiv"
	SourceTypeArXiv           SourceType = "arxiv"
)

// IdentifierType represents the type of academic paper identifier.
// These values must match the database enum identifier_type.
type IdentifierType string

const (
	IdentifierTypeDOI               IdentifierType = "doi"
	IdentifierTypeArXivID           IdentifierType = "arxiv_id"
	IdentifierTypePubMedID          IdentifierType = "pubmed_id"
	IdentifierTypeSemanticScholarID IdentifierType = "semantic_scholar_id"
	IdentifierTypeOpenAlexID        IdentifierType = "openalex_id"
	IdentifierTypeScopusID          IdentifierType = "scopus_id"
)

// Tenant represents the multi-tenancy context for a request.
type Tenant struct {
	OrgID     string
	ProjectID string
	UserID    string
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
type PaperIdentifier struct {
	ID              uuid.UUID
	PaperID         uuid.UUID
	IdentifierType  IdentifierType
	IdentifierValue string
	SourceAPI       SourceType
	DiscoveredAt    time.Time
}

// PaperSource represents a source that provided data for a paper.
type PaperSource struct {
	ID             uuid.UUID
	PaperID        uuid.UUID
	SourceAPI      SourceType
	SourceMetadata map[string]interface{}
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
