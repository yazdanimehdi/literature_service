-- Migration: 000003_core_tables
-- Description: Create core tables for keywords, papers, identifiers, sources, searches, and mappings
-- Created: 2026-02-04

-- Keywords table
-- Stores unique keywords used for paper searches
CREATE TABLE keywords (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    keyword TEXT NOT NULL,
    normalized_keyword TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Ensure uniqueness on normalized form
    CONSTRAINT uq_keywords_normalized UNIQUE (normalized_keyword)
);

-- Index for keyword lookups
CREATE INDEX idx_keywords_keyword ON keywords (keyword);
CREATE INDEX idx_keywords_normalized ON keywords (normalized_keyword);
-- Trigram index for fuzzy keyword search
CREATE INDEX idx_keywords_keyword_trgm ON keywords USING gin (keyword gin_trgm_ops);

COMMENT ON TABLE keywords IS 'Unique keywords used for academic paper searches';
COMMENT ON COLUMN keywords.keyword IS 'Original keyword text';
COMMENT ON COLUMN keywords.normalized_keyword IS 'Lowercased, trimmed keyword for deduplication';

-- Papers table
-- Central repository of discovered academic papers
CREATE TABLE papers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    canonical_id TEXT NOT NULL,
    title TEXT NOT NULL,
    abstract TEXT,
    authors JSONB,
    publication_date DATE,
    publication_year INTEGER,
    venue TEXT,
    journal TEXT,
    volume TEXT,
    issue TEXT,
    pages TEXT,
    citation_count INTEGER DEFAULT 0,
    reference_count INTEGER DEFAULT 0,
    pdf_url TEXT,
    open_access BOOLEAN DEFAULT FALSE,
    keywords_extracted BOOLEAN DEFAULT FALSE,
    raw_metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Ensure uniqueness on canonical identifier
    CONSTRAINT uq_papers_canonical_id UNIQUE (canonical_id)
);

-- Indexes for paper queries
CREATE INDEX idx_papers_canonical_id ON papers (canonical_id);
CREATE INDEX idx_papers_publication_date ON papers (publication_date);
CREATE INDEX idx_papers_publication_year ON papers (publication_year);
CREATE INDEX idx_papers_citation_count ON papers (citation_count DESC);
CREATE INDEX idx_papers_created_at ON papers (created_at);
CREATE INDEX idx_papers_keywords_extracted ON papers (keywords_extracted) WHERE NOT keywords_extracted;
-- Trigram indexes for fuzzy text search
CREATE INDEX idx_papers_title_trgm ON papers USING gin (title gin_trgm_ops);
CREATE INDEX idx_papers_abstract_trgm ON papers USING gin (abstract gin_trgm_ops) WHERE abstract IS NOT NULL;

COMMENT ON TABLE papers IS 'Central repository of discovered academic papers';
COMMENT ON COLUMN papers.canonical_id IS 'Derived identifier based on priority: DOI > arXiv > PubMed > etc.';
COMMENT ON COLUMN papers.authors IS 'JSON array of author objects with name, affiliation, etc.';
COMMENT ON COLUMN papers.raw_metadata IS 'Original metadata from source APIs for future re-processing';
COMMENT ON COLUMN papers.keywords_extracted IS 'Whether keywords have been extracted from this paper';

-- Paper Identifiers table
-- Maps papers to their various identifiers across sources
CREATE TABLE paper_identifiers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    identifier_type identifier_type NOT NULL,
    identifier_value TEXT NOT NULL,
    source_api source_type,
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Ensure uniqueness of identifier type+value combination
    CONSTRAINT uq_paper_identifiers UNIQUE (identifier_type, identifier_value)
);

-- Indexes for identifier lookups
CREATE INDEX idx_paper_identifiers_paper_id ON paper_identifiers (paper_id);
CREATE INDEX idx_paper_identifiers_type_value ON paper_identifiers (identifier_type, identifier_value);
CREATE INDEX idx_paper_identifiers_value ON paper_identifiers (identifier_value);

COMMENT ON TABLE paper_identifiers IS 'Maps papers to their various identifiers (DOI, arXiv ID, etc.)';
COMMENT ON COLUMN paper_identifiers.source_api IS 'Which API provided this identifier';

-- Paper Sources table
-- Tracks which APIs have provided data for a paper
CREATE TABLE paper_sources (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    source_api source_type NOT NULL,
    source_metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One record per paper per source
    CONSTRAINT uq_paper_sources UNIQUE (paper_id, source_api)
);

-- Indexes for source lookups
CREATE INDEX idx_paper_sources_paper_id ON paper_sources (paper_id);
CREATE INDEX idx_paper_sources_source_api ON paper_sources (source_api);

COMMENT ON TABLE paper_sources IS 'Tracks which APIs have provided data for each paper';
COMMENT ON COLUMN paper_sources.source_metadata IS 'Source-specific metadata (rankings, scores, etc.)';

-- Keyword Searches table
-- Records search operations for idempotency and audit
CREATE TABLE keyword_searches (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    keyword_id UUID NOT NULL REFERENCES keywords(id) ON DELETE CASCADE,
    source_api source_type NOT NULL,
    searched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    date_from DATE,
    date_to DATE,
    search_window_hash TEXT NOT NULL,
    papers_found INTEGER DEFAULT 0,
    status search_status NOT NULL DEFAULT 'pending',
    error_message TEXT,

    -- Ensure idempotency: one search per keyword/source/window
    CONSTRAINT uq_keyword_searches UNIQUE (search_window_hash)
);

-- Indexes for search lookups
CREATE INDEX idx_keyword_searches_keyword_id ON keyword_searches (keyword_id);
CREATE INDEX idx_keyword_searches_source_api ON keyword_searches (source_api);
CREATE INDEX idx_keyword_searches_status ON keyword_searches (status);
CREATE INDEX idx_keyword_searches_window_hash ON keyword_searches (search_window_hash);
CREATE INDEX idx_keyword_searches_searched_at ON keyword_searches (searched_at);

COMMENT ON TABLE keyword_searches IS 'Records of keyword search operations for idempotency';
COMMENT ON COLUMN keyword_searches.search_window_hash IS 'SHA256 hash of keyword_id+source_api+date_range for deduplication';
COMMENT ON COLUMN keyword_searches.date_from IS 'Start of date range filter (NULL for no filter)';
COMMENT ON COLUMN keyword_searches.date_to IS 'End of date range filter (NULL for no filter)';

-- Keyword Paper Mappings table
-- Links keywords to papers with provenance information
CREATE TABLE keyword_paper_mappings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    keyword_id UUID NOT NULL REFERENCES keywords(id) ON DELETE CASCADE,
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    mapping_type mapping_type NOT NULL,
    source_type source_type NOT NULL,
    confidence_score DECIMAL(5,4),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One mapping per keyword/paper/type combination
    CONSTRAINT uq_keyword_paper_mappings UNIQUE (keyword_id, paper_id, mapping_type)
);

-- Indexes for mapping queries
CREATE INDEX idx_keyword_paper_mappings_keyword_id ON keyword_paper_mappings (keyword_id);
CREATE INDEX idx_keyword_paper_mappings_paper_id ON keyword_paper_mappings (paper_id);
CREATE INDEX idx_keyword_paper_mappings_type ON keyword_paper_mappings (mapping_type);
CREATE INDEX idx_keyword_paper_mappings_confidence ON keyword_paper_mappings (confidence_score DESC) WHERE confidence_score IS NOT NULL;

COMMENT ON TABLE keyword_paper_mappings IS 'Links keywords to papers with provenance';
COMMENT ON COLUMN keyword_paper_mappings.mapping_type IS 'How the keyword was associated (author keyword, extracted, etc.)';
COMMENT ON COLUMN keyword_paper_mappings.confidence_score IS 'LLM confidence for extracted keywords (0.0-1.0)';
