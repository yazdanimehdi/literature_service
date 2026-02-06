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
    owner_org_id TEXT,
    owner_project_id TEXT,

    -- Metadata (tracing context, correlation IDs)
    metadata JSONB,

    -- Processing state
    status TEXT NOT NULL DEFAULT 'pending',
    sequence_number BIGSERIAL,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    last_error TEXT,
    next_attempt_at TIMESTAMPTZ,

    -- Lease-based locking for distributed processing
    locked_at TIMESTAMPTZ,
    locked_until TIMESTAMPTZ,
    locked_by TEXT,
    lock_token TEXT,

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

CREATE INDEX idx_outbox_events_locked_until
    ON outbox_events (locked_until)
    WHERE status = 'in_progress' AND locked_until IS NOT NULL;

CREATE INDEX idx_outbox_events_cleanup
    ON outbox_events (processed_at)
    WHERE status = 'published';

CREATE INDEX idx_outbox_events_scope
    ON outbox_events (scope, owner_org_id, owner_project_id);

COMMENT ON TABLE outbox_events IS 'Transactional outbox for reliable event publishing with lease-based distributed processing';
COMMENT ON COLUMN outbox_events.event_id IS 'Stable event ID for consumer deduplication';
COMMENT ON COLUMN outbox_events.sequence_number IS 'Monotonically increasing sequence for ordering';
COMMENT ON COLUMN outbox_events.lock_token IS 'UUID token for lease validation';
COMMENT ON COLUMN outbox_events.locked_until IS 'Lease expiration time for distributed processing';
COMMENT ON COLUMN outbox_events.next_attempt_at IS 'Scheduled time for retry with exponential backoff';

-- Outbox Dead Letter table
-- Stores events that have permanently failed after exhausting retries
CREATE TABLE outbox_deadletter (
    id BIGSERIAL PRIMARY KEY,

    -- Original event identification
    event_id TEXT NOT NULL,
    event_version INTEGER NOT NULL DEFAULT 1,

    -- Aggregate information
    aggregate_id TEXT NOT NULL,
    aggregate_type TEXT,
    event_type TEXT NOT NULL,

    -- Payload (JSON)
    payload BYTEA NOT NULL,

    -- Scope and ownership
    scope TEXT NOT NULL DEFAULT 'public',
    owner_org_id TEXT,
    owner_project_id TEXT,

    -- Metadata (JSON with tracing context)
    metadata JSONB,
    sequence_number BIGINT,

    -- Processing history
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    last_error TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL,
    first_attempt_at TIMESTAMPTZ,
    last_attempt_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Retry support
    retried_at TIMESTAMPTZ,
    retry_count INTEGER NOT NULL DEFAULT 0
);

-- Indexes for dead letter queue
CREATE INDEX idx_outbox_deadletter_event_id
    ON outbox_deadletter (event_id);

CREATE INDEX idx_outbox_deadletter_event_type
    ON outbox_deadletter (event_type);

CREATE INDEX idx_outbox_deadletter_aggregate
    ON outbox_deadletter (aggregate_id, aggregate_type);

CREATE INDEX idx_outbox_deadletter_failed_at
    ON outbox_deadletter (failed_at DESC);

CREATE INDEX idx_outbox_deadletter_pending_retry
    ON outbox_deadletter (failed_at)
    WHERE retried_at IS NULL;

COMMENT ON TABLE outbox_deadletter IS 'Dead letter queue for events that permanently failed after exhausting retries';
COMMENT ON COLUMN outbox_deadletter.event_id IS 'Event ID from the original outbox_events entry';
COMMENT ON COLUMN outbox_deadletter.last_error IS 'Final error message that caused the event to be dead-lettered';
