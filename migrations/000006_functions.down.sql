-- Migration: 000006_functions (rollback)
-- Description: Drop all helper functions in reverse order of creation

DROP FUNCTION IF EXISTS normalize_keyword(TEXT);
DROP FUNCTION IF EXISTS find_paper_by_any_identifier(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT);
DROP FUNCTION IF EXISTS find_paper_by_identifier(identifier_type, TEXT);
DROP FUNCTION IF EXISTS get_or_create_keyword(TEXT);
DROP FUNCTION IF EXISTS generate_search_window_hash(UUID, source_type, DATE, DATE);
DROP FUNCTION IF EXISTS generate_canonical_id(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT);
