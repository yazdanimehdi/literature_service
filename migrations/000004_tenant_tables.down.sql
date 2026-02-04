-- Migration: 000004_tenant_tables (rollback)
-- Description: Drop tenant-scoped tables in reverse order of creation

DROP TABLE IF EXISTS review_progress_events;
DROP TABLE IF EXISTS request_paper_mappings;
DROP TABLE IF EXISTS request_keyword_mappings;
DROP TABLE IF EXISTS literature_review_requests;
