BEGIN;

DROP INDEX IF EXISTS idx_campaign_status_jobs_platform_scheduled_retry;

ALTER TABLE campaign_status_jobs
    DROP CONSTRAINT IF EXISTS chk_campaign_status_jobs_platform,
    DROP COLUMN IF EXISTS raw_provider_response,
    DROP COLUMN IF EXISTS platform;

COMMIT;
