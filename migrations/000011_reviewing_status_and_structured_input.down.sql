-- Migration: 000011_reviewing_status_and_structured_input (rollback)
-- Description: Reverse structured input columns and rename title back to original_query
-- NOTE: PostgreSQL does not support removing enum values. The 'reviewing' value
-- will remain in the review_status enum after rollback.

-- Remove coverage and structured input columns
ALTER TABLE literature_review_requests
DROP COLUMN IF EXISTS coverage_reasoning,
DROP COLUMN IF EXISTS coverage_score,
DROP COLUMN IF EXISTS seed_keywords,
DROP COLUMN IF EXISTS description;

-- Rename title back to original_query
ALTER TABLE literature_review_requests RENAME COLUMN title TO original_query;

-- Restore original column comment
COMMENT ON COLUMN literature_review_requests.original_query IS 'Natural language query submitted by user';
