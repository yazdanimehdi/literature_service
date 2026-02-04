-- Migration: 000001_init_extensions (rollback)
-- Description: Remove PostgreSQL extensions
-- Note: Extensions are usually not dropped as they may be shared across schemas/databases.
--       These statements are commented out by default for safety.

-- DROP EXTENSION IF EXISTS "pgcrypto";
-- DROP EXTENSION IF EXISTS "pg_trgm";
-- DROP EXTENSION IF EXISTS "uuid-ossp";
