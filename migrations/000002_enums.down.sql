-- Migration: 000002_enums (rollback)
-- Description: Drop all enum types in reverse order of creation

DROP TYPE IF EXISTS source_type;
DROP TYPE IF EXISTS mapping_type;
DROP TYPE IF EXISTS ingestion_status;
DROP TYPE IF EXISTS search_status;
DROP TYPE IF EXISTS review_status;
DROP TYPE IF EXISTS identifier_type;
