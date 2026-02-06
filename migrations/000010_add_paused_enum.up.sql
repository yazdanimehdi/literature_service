-- Migration: 000010_add_paused_enum
-- Description: Add 'paused' value to the review_status enum
-- Created: 2026-02-05
-- NOTE: This is a separate migration because ALTER TYPE ... ADD VALUE
-- creates a new enum value that is not visible within the same transaction,
-- so it must commit before subsequent migrations can reference it.

ALTER TYPE review_status ADD VALUE IF NOT EXISTS 'paused' BEFORE 'completed';
