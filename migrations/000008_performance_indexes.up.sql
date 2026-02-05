-- Migration: 000008_performance_indexes
-- Description: Add composite indexes for frequently queried patterns to improve query performance
-- Created: 2026-02-04

-- Composite index for GetLastSearch: keyword_id + source_api + searched_at DESC
-- This covers the query: WHERE keyword_id = $1 AND source_api = $2 ORDER BY searched_at DESC LIMIT 1
-- and the NeedsSearch subquery pattern.
CREATE INDEX IF NOT EXISTS idx_keyword_searches_keyword_source_searched
    ON keyword_searches (keyword_id, source_api, searched_at DESC);

-- Composite index for review List with tenant isolation and pagination:
-- WHERE org_id = $1 AND project_id = $2 ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_review_requests_org_project_created
    ON literature_review_requests (org_id, project_id, created_at DESC);

-- Composite index for review Get with tenant isolation:
-- WHERE id = $1 AND org_id = $2 AND project_id = $3
CREATE INDEX IF NOT EXISTS idx_review_requests_id_org_project
    ON literature_review_requests (id, org_id, project_id);

-- Composite index for keyword_paper_mappings used in GetPapersForKeyword:
-- WHERE keyword_id = $1 ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_keyword_paper_mappings_keyword_created
    ON keyword_paper_mappings (keyword_id, created_at DESC);
