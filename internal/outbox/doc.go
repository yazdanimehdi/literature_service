// Package outbox provides integration with the shared outbox pattern for publishing
// literature review service events.
//
// # Overview
//
// This package bridges the literature review service's domain events with the shared
// outbox package for reliable event publishing via the transactional outbox pattern.
//
// # Components
//
//   - Emitter: Creates shared outbox events from service-specific parameters
//   - Adapter: Bridges the shared outbox repository to service interfaces
//
// # Event Types
//
// The service publishes events for key review lifecycle stages:
//
//   - review.started: A new literature review has been initiated
//   - review.completed: A review finished successfully
//   - review.failed: A review failed with an error
//   - review.cancelled: A review was cancelled by user request
//   - review.keywords_extracted: Keywords have been extracted from a query or paper
//   - review.papers_discovered: Papers have been found from an API search
//   - review.search_completed: A keyword search against a source API completed
//   - review.ingestion_started: Paper ingestion has begun
//   - review.ingestion_completed: Paper ingestion has finished
//   - review.progress_updated: Review progress has been updated
//
// # Usage
//
// Create an emitter with service configuration:
//
//	emitter := outbox.NewEmitter(outbox.EmitterConfig{
//	    ServiceName: "literature-review-service",
//	})
//
// Emit events with context:
//
//	event, err := emitter.Emit(outbox.EmitParams{
//	    RequestID: requestID,
//	    OrgID:     orgID,
//	    ProjectID: projectID,
//	    EventType: domain.EventTypeReviewStarted,
//	    Payload:   payload,
//	})
//
// Insert into outbox within a transaction:
//
//	adapter := outbox.NewAdapter(sharedRepo)
//	err := adapter.Create(ctx, event)
package outbox
