-- Add fields to track PDF file storage and ingestion runs
ALTER TABLE papers ADD COLUMN file_id UUID;
ALTER TABLE papers ADD COLUMN ingestion_run_id TEXT;

-- Partial index for efficient lookups on papers that have been ingested
CREATE INDEX idx_papers_file_id ON papers (file_id) WHERE file_id IS NOT NULL;

COMMENT ON COLUMN papers.file_id IS 'File service UUID for the downloaded PDF';
COMMENT ON COLUMN papers.ingestion_run_id IS 'Ingestion service run ID';
