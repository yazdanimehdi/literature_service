-- Migration: 000003_core_tables (rollback)
-- Description: Drop core tables in reverse order of creation

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS keyword_paper_mappings;
DROP TABLE IF EXISTS keyword_searches;
DROP TABLE IF EXISTS paper_sources;
DROP TABLE IF EXISTS paper_identifiers;
DROP TABLE IF EXISTS papers;
DROP TABLE IF EXISTS keywords;
