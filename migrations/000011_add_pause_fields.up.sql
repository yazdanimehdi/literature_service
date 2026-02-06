-- Migration: 000011_add_pause_fields
-- Description: Add pause tracking fields to literature_review_requests for resumable jobs
-- Created: 2026-02-05
-- Depends on: 000010_add_paused_enum (adds 'paused' to review_status enum)

-- Add pause tracking fields to literature_review_requests
ALTER TABLE literature_review_requests
ADD COLUMN pause_reason TEXT,
ADD COLUMN paused_at TIMESTAMPTZ,
ADD COLUMN paused_at_phase TEXT;

-- Add index for querying paused reviews by reason
-- Partial index scoped to org_id for multi-tenant queries
CREATE INDEX idx_review_requests_pause_reason
ON literature_review_requests (org_id, status, pause_reason)
WHERE status = 'paused';

COMMENT ON COLUMN literature_review_requests.pause_reason IS 'Why the review was paused: user or budget_exhausted';
COMMENT ON COLUMN literature_review_requests.paused_at IS 'When the review was paused';
COMMENT ON COLUMN literature_review_requests.paused_at_phase IS 'Which workflow phase the review was in when paused';
