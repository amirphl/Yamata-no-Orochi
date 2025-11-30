-- Migration: Remove unused indexes from short_links table
-- Up migration

BEGIN;

-- Drop indexes that are no longer needed
DROP INDEX IF EXISTS idx_short_links_campaign_id;
DROP INDEX IF EXISTS idx_short_links_client_id;
DROP INDEX IF EXISTS idx_short_links_created_at;
DROP INDEX IF EXISTS idx_short_links_phone_number;

COMMIT;