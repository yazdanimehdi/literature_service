-- Migration: 000006_functions
-- Description: Create helper functions for canonical ID generation, search hashing, and lookups
-- Created: 2026-02-04

-- generate_canonical_id function
-- Generates a canonical identifier for a paper based on priority order
-- Priority: DOI > arXiv ID > PubMed ID > Semantic Scholar ID > OpenAlex ID > Scopus ID
CREATE OR REPLACE FUNCTION generate_canonical_id(
    p_doi TEXT DEFAULT NULL,
    p_arxiv_id TEXT DEFAULT NULL,
    p_pubmed_id TEXT DEFAULT NULL,
    p_semantic_scholar_id TEXT DEFAULT NULL,
    p_openalex_id TEXT DEFAULT NULL,
    p_scopus_id TEXT DEFAULT NULL
)
RETURNS TEXT
LANGUAGE plpgsql
IMMUTABLE
AS $$
BEGIN
    -- Return first non-null identifier in priority order with prefix
    IF p_doi IS NOT NULL AND p_doi != '' THEN
        RETURN 'doi:' || LOWER(TRIM(p_doi));
    ELSIF p_arxiv_id IS NOT NULL AND p_arxiv_id != '' THEN
        RETURN 'arxiv:' || LOWER(TRIM(p_arxiv_id));
    ELSIF p_pubmed_id IS NOT NULL AND p_pubmed_id != '' THEN
        RETURN 'pubmed:' || TRIM(p_pubmed_id);
    ELSIF p_semantic_scholar_id IS NOT NULL AND p_semantic_scholar_id != '' THEN
        RETURN 's2:' || TRIM(p_semantic_scholar_id);
    ELSIF p_openalex_id IS NOT NULL AND p_openalex_id != '' THEN
        RETURN 'openalex:' || UPPER(TRIM(p_openalex_id));
    ELSIF p_scopus_id IS NOT NULL AND p_scopus_id != '' THEN
        RETURN 'scopus:' || TRIM(p_scopus_id);
    ELSE
        -- Fallback to a random UUID if no identifiers provided
        RETURN 'unknown:' || gen_random_uuid()::text;
    END IF;
END;
$$;

COMMENT ON FUNCTION generate_canonical_id IS 'Generates canonical paper ID with priority: DOI > arXiv > PubMed > S2 > OpenAlex > Scopus';

-- generate_search_window_hash function
-- Creates a SHA256 hash for idempotent search deduplication
CREATE OR REPLACE FUNCTION generate_search_window_hash(
    p_keyword_id UUID,
    p_source_api source_type,
    p_date_from DATE DEFAULT NULL,
    p_date_to DATE DEFAULT NULL
)
RETURNS TEXT
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
    hash_input TEXT;
BEGIN
    -- Construct deterministic hash input
    hash_input := p_keyword_id::text || '|'
                  || p_source_api::text || '|'
                  || COALESCE(p_date_from::text, 'NULL') || '|'
                  || COALESCE(p_date_to::text, 'NULL');

    -- Return SHA256 hash encoded as hex
    RETURN encode(digest(hash_input, 'sha256'), 'hex');
END;
$$;

COMMENT ON FUNCTION generate_search_window_hash IS 'Creates SHA256 hash for search window idempotency';

-- get_or_create_keyword function
-- Idempotently creates or retrieves a keyword, returning its ID
CREATE OR REPLACE FUNCTION get_or_create_keyword(
    p_keyword TEXT
)
RETURNS UUID
LANGUAGE plpgsql
AS $$
DECLARE
    v_normalized TEXT;
    v_keyword_id UUID;
BEGIN
    -- Normalize the keyword
    v_normalized := LOWER(TRIM(p_keyword));

    -- Try to find existing keyword
    SELECT id INTO v_keyword_id
    FROM keywords
    WHERE normalized_keyword = v_normalized;

    -- If not found, insert new keyword
    IF v_keyword_id IS NULL THEN
        INSERT INTO keywords (keyword, normalized_keyword)
        VALUES (TRIM(p_keyword), v_normalized)
        ON CONFLICT (normalized_keyword) DO UPDATE
            SET keyword = EXCLUDED.keyword  -- No-op to return the id
        RETURNING id INTO v_keyword_id;
    END IF;

    RETURN v_keyword_id;
END;
$$;

COMMENT ON FUNCTION get_or_create_keyword IS 'Idempotently creates or retrieves a keyword by normalized value';

-- find_paper_by_identifier function
-- Looks up a paper by any identifier type and value
CREATE OR REPLACE FUNCTION find_paper_by_identifier(
    p_identifier_type identifier_type,
    p_identifier_value TEXT
)
RETURNS UUID
LANGUAGE plpgsql
STABLE
AS $$
DECLARE
    v_paper_id UUID;
BEGIN
    SELECT paper_id INTO v_paper_id
    FROM paper_identifiers
    WHERE identifier_type = p_identifier_type
      AND identifier_value = TRIM(p_identifier_value);

    RETURN v_paper_id;
END;
$$;

COMMENT ON FUNCTION find_paper_by_identifier IS 'Looks up a paper by any identifier type and value';

-- find_paper_by_any_identifier function
-- Looks up a paper by checking all provided identifiers in priority order
CREATE OR REPLACE FUNCTION find_paper_by_any_identifier(
    p_doi TEXT DEFAULT NULL,
    p_arxiv_id TEXT DEFAULT NULL,
    p_pubmed_id TEXT DEFAULT NULL,
    p_semantic_scholar_id TEXT DEFAULT NULL,
    p_openalex_id TEXT DEFAULT NULL,
    p_scopus_id TEXT DEFAULT NULL
)
RETURNS UUID
LANGUAGE plpgsql
STABLE
AS $$
DECLARE
    v_paper_id UUID;
BEGIN
    -- Check each identifier in priority order
    IF p_doi IS NOT NULL AND p_doi != '' THEN
        v_paper_id := find_paper_by_identifier('doi'::identifier_type, p_doi);
        IF v_paper_id IS NOT NULL THEN RETURN v_paper_id; END IF;
    END IF;

    IF p_arxiv_id IS NOT NULL AND p_arxiv_id != '' THEN
        v_paper_id := find_paper_by_identifier('arxiv_id'::identifier_type, p_arxiv_id);
        IF v_paper_id IS NOT NULL THEN RETURN v_paper_id; END IF;
    END IF;

    IF p_pubmed_id IS NOT NULL AND p_pubmed_id != '' THEN
        v_paper_id := find_paper_by_identifier('pubmed_id'::identifier_type, p_pubmed_id);
        IF v_paper_id IS NOT NULL THEN RETURN v_paper_id; END IF;
    END IF;

    IF p_semantic_scholar_id IS NOT NULL AND p_semantic_scholar_id != '' THEN
        v_paper_id := find_paper_by_identifier('semantic_scholar_id'::identifier_type, p_semantic_scholar_id);
        IF v_paper_id IS NOT NULL THEN RETURN v_paper_id; END IF;
    END IF;

    IF p_openalex_id IS NOT NULL AND p_openalex_id != '' THEN
        v_paper_id := find_paper_by_identifier('openalex_id'::identifier_type, p_openalex_id);
        IF v_paper_id IS NOT NULL THEN RETURN v_paper_id; END IF;
    END IF;

    IF p_scopus_id IS NOT NULL AND p_scopus_id != '' THEN
        v_paper_id := find_paper_by_identifier('scopus_id'::identifier_type, p_scopus_id);
        IF v_paper_id IS NOT NULL THEN RETURN v_paper_id; END IF;
    END IF;

    RETURN NULL;
END;
$$;

COMMENT ON FUNCTION find_paper_by_any_identifier IS 'Looks up a paper by checking all provided identifiers in priority order';

-- normalize_keyword function
-- Helper function to normalize keywords consistently
CREATE OR REPLACE FUNCTION normalize_keyword(
    p_keyword TEXT
)
RETURNS TEXT
LANGUAGE plpgsql
IMMUTABLE
AS $$
BEGIN
    RETURN LOWER(TRIM(p_keyword));
END;
$$;

COMMENT ON FUNCTION normalize_keyword IS 'Normalizes keyword text to lowercase trimmed form';
