-- Migration: 000011_reviewing_status_and_structured_input
-- Description: Add 'reviewing' status, rename original_query to title, add structured input columns
-- Created: 2026-02-05

-- Add 'reviewing' enum value before 'completed'
-- NOTE: ALTER TYPE ... ADD VALUE cannot run inside a transaction.
-- If using golang-migrate with default transaction wrapping, this migration
-- must be applied with x-multi-statement mode or run outside a transaction.
ALTER TYPE review_status ADD VALUE IF NOT EXISTS 'reviewing' BEFORE 'completed';

-- Rename original_query to title for clearer semantics
ALTER TABLE literature_review_requests RENAME COLUMN original_query TO title;

-- Add structured input and coverage columns
ALTER TABLE literature_review_requests
ADD COLUMN description TEXT DEFAULT '',
ADD COLUMN seed_keywords JSONB DEFAULT '[]'::jsonb,
ADD COLUMN coverage_score DOUBLE PRECISION,
ADD COLUMN coverage_reasoning TEXT;

-- Update column comments
COMMENT ON COLUMN literature_review_requests.title IS 'User-provided title for the literature review';
COMMENT ON COLUMN literature_review_requests.description IS 'Optional description providing additional context for the review';
COMMENT ON COLUMN literature_review_requests.seed_keywords IS 'JSON array of user-provided seed keywords to guide the search';
COMMENT ON COLUMN literature_review_requests.coverage_score IS 'LLM-assessed coverage score (0.0-1.0) indicating search completeness';
COMMENT ON COLUMN literature_review_requests.coverage_reasoning IS 'LLM explanation of coverage assessment and any identified gaps';
