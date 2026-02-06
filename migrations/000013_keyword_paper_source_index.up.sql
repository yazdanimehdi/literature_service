-- Migration: 000013_keyword_paper_source_index
-- Description: Add composite index for source-filtered keyword paper lookups
-- Created: 2026-02-06

-- Composite index for GetPapersForKeywordAndSource:
-- WHERE keyword_id = $1 AND source_type = $2 ORDER BY created_at DESC
-- The existing idx_keyword_paper_mappings_keyword_created only covers keyword_id,
-- requiring a filter step for source_type. This index eliminates the filter.
CREATE INDEX IF NOT EXISTS idx_keyword_paper_mappings_keyword_source_created
    ON keyword_paper_mappings (keyword_id, source_type, created_at DESC);
