DROP INDEX IF EXISTS idx_papers_file_id;
ALTER TABLE papers DROP COLUMN IF EXISTS ingestion_run_id;
ALTER TABLE papers DROP COLUMN IF EXISTS file_id;
