-- Migration: 000008_performance_indexes (rollback)
-- Description: Remove composite performance indexes

DROP INDEX IF EXISTS idx_keyword_paper_mappings_keyword_created;
DROP INDEX IF EXISTS idx_review_requests_id_org_project;
DROP INDEX IF EXISTS idx_review_requests_org_project_created;
DROP INDEX IF EXISTS idx_keyword_searches_keyword_source_searched;
