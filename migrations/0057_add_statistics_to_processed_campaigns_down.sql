-- Remove statistics field from processed_campaigns
BEGIN;

ALTER TABLE processed_campaigns
    DROP COLUMN IF EXISTS statistics;

COMMIT;
