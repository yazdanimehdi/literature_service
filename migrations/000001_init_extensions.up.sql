-- Migration: 000001_init_extensions
-- Description: Initialize required PostgreSQL extensions for Literature Review Service
-- Created: 2026-02-04

-- Enable UUID generation (uuid_generate_v4)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Enable trigram search for text matching (fuzzy search on titles, abstracts)
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- Enable cryptographic functions (gen_random_uuid, digest for hash generation)
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
