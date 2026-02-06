-- Migration: 000013_keyword_paper_source_index (rollback)
DROP INDEX IF EXISTS idx_keyword_paper_mappings_keyword_source_created;
