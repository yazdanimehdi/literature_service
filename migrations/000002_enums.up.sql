-- Migration: 000002_enums
-- Description: Create all enum types for the Literature Review Service
-- Created: 2026-02-04

-- Identifier Type Enum
-- Represents the type of academic paper identifier
CREATE TYPE identifier_type AS ENUM (
    'doi',              -- Digital Object Identifier
    'arxiv_id',         -- arXiv identifier
    'pubmed_id',        -- PubMed identifier
    'semantic_scholar_id', -- Semantic Scholar Paper ID
    'openalex_id',      -- OpenAlex Work ID
    'scopus_id'         -- Scopus EID
);

COMMENT ON TYPE identifier_type IS 'Types of academic paper identifiers from various sources';

-- Review Status Enum
-- Represents the lifecycle states of a literature review request
CREATE TYPE review_status AS ENUM (
    'pending',          -- Request created, not yet started
    'extracting_keywords', -- LLM extracting keywords from query
    'searching',        -- Searching paper sources
    'expanding',        -- Recursive keyword expansion in progress
    'ingesting',        -- Papers being sent to ingestion service
    'completed',        -- Review completed successfully
    'partial',          -- Review completed with some failures
    'failed',           -- Review failed
    'cancelled'         -- Review cancelled by user
);

COMMENT ON TYPE review_status IS 'Lifecycle states for literature review requests';

-- Search Status Enum
-- Represents the state of a keyword search operation
CREATE TYPE search_status AS ENUM (
    'pending',          -- Search queued
    'in_progress',      -- Search currently executing
    'completed',        -- Search completed successfully
    'failed',           -- Search failed
    'rate_limited'      -- Search hit rate limit, will retry
);

COMMENT ON TYPE search_status IS 'States for keyword search operations';

-- Ingestion Status Enum
-- Represents the ingestion state of a paper within a review
CREATE TYPE ingestion_status AS ENUM (
    'pending',          -- Not yet submitted for ingestion
    'submitted',        -- Submitted to ingestion service
    'processing',       -- Ingestion in progress
    'completed',        -- Ingestion completed successfully
    'failed',           -- Ingestion failed
    'skipped'           -- Skipped (duplicate, excluded, etc.)
);

COMMENT ON TYPE ingestion_status IS 'Ingestion states for papers within a review';

-- Mapping Type Enum
-- Represents how a keyword was associated with a paper
CREATE TYPE mapping_type AS ENUM (
    'author_keyword',   -- Keyword from author-provided keywords
    'mesh_term',        -- MeSH term from PubMed
    'extracted',        -- Extracted from title/abstract by LLM
    'query_match'       -- Direct match from user query
);

COMMENT ON TYPE mapping_type IS 'How a keyword was associated with a paper';

-- Source Type Enum
-- Represents the source API that provided paper data
CREATE TYPE source_type AS ENUM (
    'semantic_scholar', -- Semantic Scholar API
    'openalex',         -- OpenAlex API
    'scopus',           -- Scopus API
    'pubmed',           -- PubMed/Entrez API
    'biorxiv',          -- bioRxiv API
    'arxiv'             -- arXiv API
);

COMMENT ON TYPE source_type IS 'Paper source APIs';
