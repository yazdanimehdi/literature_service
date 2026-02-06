-- Migration: 000011_add_pause_fields (rollback)
-- Description: Remove pause tracking fields from literature_review_requests

DROP INDEX IF EXISTS idx_review_requests_pause_reason;

ALTER TABLE literature_review_requests
DROP COLUMN IF EXISTS pause_reason,
DROP COLUMN IF EXISTS paused_at,
DROP COLUMN IF EXISTS paused_at_phase;
