// Package workflows defines Temporal workflow implementations for the
// literature review service pipeline.
package workflows

import (
	"time"

	"github.com/google/uuid"
)

// Signal names for parent-child workflow communication.
const (
	// SignalClaimPapers is sent by child to claim papers for processing.
	SignalClaimPapers = "claim_papers"
	// SignalRegisterEmbeddings is sent by child to register embeddings.
	SignalRegisterEmbeddings = "register_embeddings"
	// SignalBatchComplete is sent by child when batch processing completes.
	SignalBatchComplete = "batch_complete"
)

// ClaimPapersRequest is sent by child workflow to claim papers for processing.
type ClaimPapersRequest struct {
	// BatchID identifies the child workflow batch.
	BatchID string `json:"batch_id"`
	// CanonicalIDs are the paper canonical IDs to claim.
	CanonicalIDs []string `json:"canonical_ids"`
}

// ClaimPapersResponse is returned to child with papers it can process.
type ClaimPapersResponse struct {
	// ClaimedIDs are canonical IDs successfully claimed (new, not seen before).
	ClaimedIDs []string `json:"claimed_ids"`
}

// RegisterEmbeddingsRequest is sent by child to register paper embeddings.
type RegisterEmbeddingsRequest struct {
	// BatchID identifies the child workflow batch.
	BatchID string `json:"batch_id"`
	// Embeddings maps canonical ID to embedding vector.
	Embeddings map[string][]float32 `json:"embeddings"`
}

// BatchCompleteSignal is sent by child when batch processing completes.
type BatchCompleteSignal struct {
	// BatchID identifies the child workflow batch.
	BatchID string `json:"batch_id"`
	// Processed is total papers processed in this batch.
	Processed int `json:"processed"`
	// Duplicates is papers filtered as duplicates.
	Duplicates int `json:"duplicates"`
	// Ingested is papers successfully ingested.
	Ingested int `json:"ingested"`
	// Failed is papers that failed processing.
	Failed int `json:"failed"`
	// Results contains per-paper results for DB updates.
	Results []PaperResult `json:"results"`
}

// PaperResult contains the result of processing a single paper.
type PaperResult struct {
	// PaperID is the paper's internal UUID.
	PaperID uuid.UUID `json:"paper_id"`
	// CanonicalID is the paper's canonical identifier.
	CanonicalID string `json:"canonical_id"`
	// FileID is the file_service UUID (empty if not ingested).
	FileID string `json:"file_id"`
	// IngestionRunID is the ingestion run ID (empty if not ingested).
	IngestionRunID string `json:"ingestion_run_id"`
	// Status is the processing status.
	Status string `json:"status"`
	// Error contains error message if processing failed.
	Error string `json:"error"`
}

// PipelineProgress tracks the internal progress of the pipeline workflow.
type PipelineProgress struct {
	// Status is the overall workflow status.
	Status string `json:"status"`
	// Phase is the current phase (searching, batching, processing, completing).
	Phase string `json:"phase"`

	// Search phase metrics
	PapersFound      int `json:"papers_found"`
	SearchesComplete int `json:"searches_complete"`
	SearchesTotal    int `json:"searches_total"`

	// Processing phase metrics
	BatchesSpawned   int `json:"batches_spawned"`
	BatchesCompleted int `json:"batches_completed"`
	PapersProcessed  int `json:"papers_processed"`
	DuplicatesFound  int `json:"duplicates_found"`
	PapersIngested   int `json:"papers_ingested"`
	PapersFailed     int `json:"papers_failed"`

	// Timing
	StartTime     time.Time `json:"start_time"`
	SearchEndTime time.Time `json:"search_end_time"`
}

// PaperBatch represents a batch of papers to be processed by a child workflow.
type PaperBatch struct {
	// BatchID is a unique identifier for this batch.
	BatchID string `json:"batch_id"`
	// Papers contains the papers in this batch.
	Papers []PaperForProcessing `json:"papers"`
}

// PaperForProcessing contains paper data needed for the processing pipeline.
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
}
