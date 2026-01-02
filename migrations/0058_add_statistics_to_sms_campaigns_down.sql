-- Remove statistics column from sms_campaigns
BEGIN;

ALTER TABLE sms_campaigns
    DROP COLUMN IF EXISTS statistics;

COMMIT;
