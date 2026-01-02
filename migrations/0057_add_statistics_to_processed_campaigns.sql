-- Add statistics JSONB field to processed_campaigns
BEGIN;

ALTER TABLE processed_campaigns
    ADD COLUMN IF NOT EXISTS statistics JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMIT;
