-- Migration: 000004_tenant_tables
-- Description: Create tenant-scoped tables for literature review requests and progress tracking
-- Created: 2026-02-04

-- Literature Review Requests table
-- Multi-tenant table storing user review requests
CREATE TABLE literature_review_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Tenant context (multi-tenancy)
    org_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    user_id TEXT NOT NULL,

    -- Request details
    original_query TEXT NOT NULL,

    -- Temporal workflow tracking
    temporal_workflow_id TEXT,
    temporal_run_id TEXT,

    -- Status and progress
    status review_status NOT NULL DEFAULT 'pending',
    keywords_found_count INTEGER DEFAULT 0,
    papers_found_count INTEGER DEFAULT 0,
    papers_ingested_count INTEGER DEFAULT 0,
    papers_failed_count INTEGER DEFAULT 0,

    -- Configuration
    expansion_depth INTEGER NOT NULL DEFAULT 1,
    config_snapshot JSONB,
    source_filters JSONB,
    date_from DATE,
    date_to DATE,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    -- Constraints
    CONSTRAINT valid_expansion_depth CHECK (expansion_depth >= 0 AND expansion_depth <= 5)
);

-- Indexes for request queries
CREATE INDEX idx_review_requests_org_id ON literature_review_requests (org_id);
CREATE INDEX idx_review_requests_project_id ON literature_review_requests (project_id);
CREATE INDEX idx_review_requests_user_id ON literature_review_requests (user_id);
CREATE INDEX idx_review_requests_org_project ON literature_review_requests (org_id, project_id);
CREATE INDEX idx_review_requests_status ON literature_review_requests (status);
CREATE INDEX idx_review_requests_created_at ON literature_review_requests (created_at DESC);
CREATE INDEX idx_review_requests_workflow_id ON literature_review_requests (temporal_workflow_id) WHERE temporal_workflow_id IS NOT NULL;

COMMENT ON TABLE literature_review_requests IS 'Multi-tenant literature review requests initiated by users';
COMMENT ON COLUMN literature_review_requests.org_id IS 'Organization ID for multi-tenancy';
COMMENT ON COLUMN literature_review_requests.project_id IS 'Project ID within organization';
COMMENT ON COLUMN literature_review_requests.original_query IS 'Natural language query submitted by user';
COMMENT ON COLUMN literature_review_requests.expansion_depth IS 'How many levels of recursive keyword expansion (0-5)';
COMMENT ON COLUMN literature_review_requests.config_snapshot IS 'Snapshot of config at request time for reproducibility';
COMMENT ON COLUMN literature_review_requests.source_filters IS 'Which paper sources to include/exclude';

-- Request Keyword Mappings table
-- Links review requests to discovered keywords with provenance
CREATE TABLE request_keyword_mappings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_id UUID NOT NULL REFERENCES literature_review_requests(id) ON DELETE CASCADE,
    keyword_id UUID NOT NULL REFERENCES keywords(id) ON DELETE CASCADE,
    extraction_round INTEGER NOT NULL DEFAULT 0,
    source_paper_id UUID REFERENCES papers(id) ON DELETE SET NULL,
    source_type TEXT NOT NULL DEFAULT 'query',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One keyword per request per round from same source
    CONSTRAINT uq_request_keyword_mappings UNIQUE (request_id, keyword_id, extraction_round, source_paper_id)
);

-- Indexes for request-keyword queries
CREATE INDEX idx_request_keyword_mappings_request_id ON request_keyword_mappings (request_id);
CREATE INDEX idx_request_keyword_mappings_keyword_id ON request_keyword_mappings (keyword_id);
CREATE INDEX idx_request_keyword_mappings_round ON request_keyword_mappings (extraction_round);
CREATE INDEX idx_request_keyword_mappings_source_paper ON request_keyword_mappings (source_paper_id) WHERE source_paper_id IS NOT NULL;

COMMENT ON TABLE request_keyword_mappings IS 'Links review requests to keywords with provenance';
COMMENT ON COLUMN request_keyword_mappings.extraction_round IS 'Which expansion round (0=initial query, 1+=expanded)';
COMMENT ON COLUMN request_keyword_mappings.source_paper_id IS 'Paper that yielded this keyword (NULL for initial query)';
COMMENT ON COLUMN request_keyword_mappings.source_type IS 'How keyword was obtained: query, paper_keywords, llm_extraction';

-- Request Paper Mappings table
-- Links review requests to discovered papers with discovery context
CREATE TABLE request_paper_mappings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_id UUID NOT NULL REFERENCES literature_review_requests(id) ON DELETE CASCADE,
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    discovered_via_keyword_id UUID REFERENCES keywords(id) ON DELETE SET NULL,
    discovered_via_source source_type,
    expansion_depth INTEGER NOT NULL DEFAULT 0,

    -- Ingestion tracking
    ingestion_status ingestion_status NOT NULL DEFAULT 'pending',
    ingestion_job_id TEXT,
    ingestion_error TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One paper per request (may be discovered multiple ways, keep first)
    CONSTRAINT uq_request_paper_mappings UNIQUE (request_id, paper_id)
);

-- Indexes for request-paper queries
CREATE INDEX idx_request_paper_mappings_request_id ON request_paper_mappings (request_id);
CREATE INDEX idx_request_paper_mappings_paper_id ON request_paper_mappings (paper_id);
CREATE INDEX idx_request_paper_mappings_keyword_id ON request_paper_mappings (discovered_via_keyword_id) WHERE discovered_via_keyword_id IS NOT NULL;
CREATE INDEX idx_request_paper_mappings_ingestion_status ON request_paper_mappings (ingestion_status);
CREATE INDEX idx_request_paper_mappings_depth ON request_paper_mappings (expansion_depth);
CREATE INDEX idx_request_paper_mappings_pending ON request_paper_mappings (request_id, ingestion_status) WHERE ingestion_status = 'pending';

COMMENT ON TABLE request_paper_mappings IS 'Links review requests to papers with discovery context';
COMMENT ON COLUMN request_paper_mappings.discovered_via_keyword_id IS 'Keyword search that found this paper';
COMMENT ON COLUMN request_paper_mappings.expansion_depth IS 'Which expansion round discovered this paper';
COMMENT ON COLUMN request_paper_mappings.ingestion_job_id IS 'Ingestion service job ID for tracking';

-- Review Progress Events table
-- Real-time progress tracking for UI updates
CREATE TABLE review_progress_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_id UUID NOT NULL REFERENCES literature_review_requests(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    event_data JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for progress event queries
CREATE INDEX idx_review_progress_events_request_id ON review_progress_events (request_id);
CREATE INDEX idx_review_progress_events_type ON review_progress_events (event_type);
CREATE INDEX idx_review_progress_events_created_at ON review_progress_events (created_at);
CREATE INDEX idx_review_progress_events_request_created ON review_progress_events (request_id, created_at);

COMMENT ON TABLE review_progress_events IS 'Real-time progress events for UI updates via NOTIFY/LISTEN';
COMMENT ON COLUMN review_progress_events.event_type IS 'Event type: keyword_extracted, paper_found, search_completed, etc.';
COMMENT ON COLUMN review_progress_events.event_data IS 'Event-specific data payload';
