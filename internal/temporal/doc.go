// Package temporal provides Temporal workflow client integration for the
// literature review service.
//
// This package handles workflow client initialization, workflow/activity
// registration, and worker lifecycle management.
//
// # Overview
//
// The temporal package provides:
//
//   - Client: Temporal client wrapper for starting/managing workflows
//   - Worker: Worker process for executing workflows and activities
//   - Workflow definitions for literature review orchestration
//   - Activity implementations for review pipeline steps
//
// # Client Setup
//
// Create a Temporal client:
//
//	cfg := temporal.ClientConfig{
//	    HostPort:  "localhost:7233",
//	    Namespace: "literature-review",
//	    TaskQueue: "literature-review-tasks",
//	}
//
//	client, err := temporal.NewClient(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
// # Starting Workflows
//
// Start a literature review workflow:
//
//	workflowID, runID, err := client.StartReviewWorkflow(ctx, temporal.ReviewWorkflowRequest{
//	    RequestID:  requestID,
//	    OrgID:      orgID,
//	    ProjectID:  projectID,
//	    Query:      query,
//	    MaxPapers:  100,
//	})
//
// # Worker Setup
//
// Create and start a worker:
//
//	deps := temporal.ActivityDependencies{
//	    LLMActivities:     llmActivities,
//	    SearchActivities:  searchActivities,
//	    StatusActivities:  statusActivities,
//	}
//
//	worker, err := temporal.NewWorker(client, cfg, deps)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	if err := temporal.StartWorker(ctx, worker); err != nil {
//	    log.Fatal(err)
//	}
//
// # Workflow Types
//
// The package defines several workflow types:
//
//   - LiteratureReviewWorkflow: Main orchestration workflow
//   - KeywordExtractionWorkflow: Child workflow for keyword extraction
//   - PaperSearchWorkflow: Fan-out workflow for paper search across sources
//
// # Activity Types
//
// Activities are grouped by responsibility:
//
//   - LLM activities: Keyword extraction, query parsing
//   - Search activities: Paper source API calls
//   - Status activities: Review status updates, event emission
//   - Ingestion activities: Paper ingestion requests
//
// # Error Handling
//
// Workflows use standard Temporal error handling:
//
//	if temporal.IsWorkflowNotFound(err) {
//	    // Workflow doesn't exist or already completed
//	}
//
//	if temporal.IsWorkflowAlreadyStarted(err) {
//	    // Workflow with same ID is already running
//	}
//
// # Thread Safety
//
// The Temporal client is safe for concurrent use. Workers manage their
// own goroutines for activity execution.
package temporal
