-- Add nullable statistics JSONB to sms_campaigns
BEGIN;

ALTER TABLE sms_campaigns
    ADD COLUMN IF NOT EXISTS statistics JSONB NULL;

COMMIT;
