-- Migration: 000005_outbox (rollback)
-- Description: Drop outbox tables in reverse order of creation

DROP TABLE IF EXISTS outbox_dead_letter;
DROP TABLE IF EXISTS outbox_events;
