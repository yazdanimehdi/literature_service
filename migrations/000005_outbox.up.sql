-- Migration: 000005_outbox
-- Description: Create outbox tables for transactional event publishing (matches shared outbox package schema)
-- Created: 2026-02-04

-- Outbox Events table
-- Stores events pending publication with lease-based distributed processing support
CREATE TABLE outbox_events (
    id BIGSERIAL PRIMARY KEY,

    -- Event identification
    event_id TEXT NOT NULL DEFAULT gen_random_uuid()::text,
    event_version INTEGER NOT NULL DEFAULT 1,

    -- Aggregate information
    aggregate_id TEXT NOT NULL,
    aggregate_type TEXT,
    event_type TEXT NOT NULL,

    -- Payload (binary for efficiency)
    payload BYTEA NOT NULL,

    -- Scope and ownership (multi-tenancy)
    scope TEXT NOT NULL DEFAULT 'public',
    org_id TEXT,
    project_id TEXT,

    -- Metadata (tracing context, correlation IDs)
    metadata JSONB,

    -- Processing state
    status TEXT NOT NULL DEFAULT 'pending',
    sequence_num BIGSERIAL,
    publish_attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    last_error TEXT,
    next_attempt_at TIMESTAMPTZ,

    -- Lease-based locking for distributed processing
    locked_by TEXT,
    locked_at TIMESTAMPTZ,
    lock_expires_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,

    -- Constraints
    CONSTRAINT valid_outbox_status CHECK (status IN ('pending', 'in_progress', 'published', 'failed')),
    CONSTRAINT valid_outbox_scope CHECK (scope IN ('public', 'org', 'project'))
);

-- Indexes for efficient event processing
CREATE INDEX idx_outbox_events_status_next_attempt
    ON outbox_events (status, next_attempt_at)
    WHERE status = 'pending';

CREATE INDEX idx_outbox_events_status_created
    ON outbox_events (status, created_at)
    WHERE status = 'pending';

CREATE INDEX idx_outbox_events_aggregate
    ON outbox_events (aggregate_id, aggregate_type);

CREATE INDEX idx_outbox_events_event_id
    ON outbox_events (event_id);

CREATE INDEX idx_outbox_events_lock_expires
    ON outbox_events (lock_expires_at)
    WHERE status = 'in_progress' AND lock_expires_at IS NOT NULL;

CREATE INDEX idx_outbox_events_cleanup
    ON outbox_events (processed_at)
    WHERE status = 'published';

CREATE INDEX idx_outbox_events_scope
    ON outbox_events (scope, org_id, project_id);

COMMENT ON TABLE outbox_events IS 'Transactional outbox for reliable event publishing with lease-based distributed processing';
COMMENT ON COLUMN outbox_events.event_id IS 'Stable event ID for consumer deduplication';
COMMENT ON COLUMN outbox_events.sequence_num IS 'Monotonically increasing sequence for ordering';
COMMENT ON COLUMN outbox_events.locked_by IS 'Worker ID that holds the lease';
COMMENT ON COLUMN outbox_events.lock_expires_at IS 'Lease expiration time for distributed processing';
COMMENT ON COLUMN outbox_events.next_attempt_at IS 'Scheduled time for retry with exponential backoff';

-- Outbox Dead Letter table
-- Stores events that have permanently failed after exhausting retries
CREATE TABLE outbox_dead_letter (
    id BIGSERIAL PRIMARY KEY,

    -- Original event identification
    original_event_id TEXT NOT NULL,
    event_version INTEGER NOT NULL DEFAULT 1,

    -- Aggregate information
    aggregate_type TEXT,
    aggregate_id TEXT NOT NULL,
    event_type TEXT NOT NULL,

    -- Scope and ownership
    org_id TEXT,
    project_id TEXT,

    -- Payload and metadata
    payload BYTEA NOT NULL,
    metadata JSONB,

    -- Failure information
    original_created_at TIMESTAMPTZ NOT NULL,
    failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    failure_reason TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,

    -- Reprocessing support
    reprocessed_at TIMESTAMPTZ,
    reprocess_count INTEGER NOT NULL DEFAULT 0
);

-- Indexes for dead letter queue
CREATE INDEX idx_outbox_dead_letter_event_id
    ON outbox_dead_letter (original_event_id);

CREATE INDEX idx_outbox_dead_letter_event_type
    ON outbox_dead_letter (event_type);

CREATE INDEX idx_outbox_dead_letter_aggregate
    ON outbox_dead_letter (aggregate_id, aggregate_type);

CREATE INDEX idx_outbox_dead_letter_failed_at
    ON outbox_dead_letter (failed_at DESC);

CREATE INDEX idx_outbox_dead_letter_pending_reprocess
    ON outbox_dead_letter (failed_at)
    WHERE reprocessed_at IS NULL;

COMMENT ON TABLE outbox_dead_letter IS 'Dead letter queue for events that permanently failed after exhausting retries';
COMMENT ON COLUMN outbox_dead_letter.original_event_id IS 'Event ID from the original outbox_events entry';
COMMENT ON COLUMN outbox_dead_letter.failure_reason IS 'Final error message that caused the event to be dead-lettered';
