-- Migration: 000007_triggers (rollback)
-- Description: Drop all triggers and trigger functions in reverse order

-- Drop triggers first (must be dropped before their functions)
DROP TRIGGER IF EXISTS trg_paper_sources_updated_at ON paper_sources;
DROP TRIGGER IF EXISTS trg_request_paper_mappings_updated_at ON request_paper_mappings;
DROP TRIGGER IF EXISTS trg_review_requests_updated_at ON literature_review_requests;
DROP TRIGGER IF EXISTS trg_papers_updated_at ON papers;
DROP TRIGGER IF EXISTS trg_review_progress_notify ON review_progress_events;

-- Drop trigger functions
DROP FUNCTION IF EXISTS update_paper_sources_timestamp();
DROP FUNCTION IF EXISTS update_request_paper_mapping_timestamp();
DROP FUNCTION IF EXISTS update_review_request_timestamp();
DROP FUNCTION IF EXISTS update_papers_timestamp();
DROP FUNCTION IF EXISTS notify_review_progress();
