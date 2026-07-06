-- Scope campaign status jobs to their owning scheduler and retain provider responses.
BEGIN;

ALTER TABLE campaign_status_jobs
    ADD COLUMN IF NOT EXISTS platform VARCHAR(20),
    ADD COLUMN IF NOT EXISTS raw_provider_response TEXT;

-- Existing jobs predate the platform column. The processed campaign snapshot is
-- the most reliable source because it is the exact campaign each job belongs to.
UPDATE campaign_status_jobs AS status_job
SET platform = CASE
    WHEN LOWER(TRIM(processed.campaign_json ->> 'platform')) IN ('sms', 'bale', 'splus', 'rubika')
        THEN LOWER(TRIM(processed.campaign_json ->> 'platform'))
    ELSE 'sms'
END
FROM processed_campaigns AS processed
WHERE processed.id = status_job.processed_campaign_id
  AND status_job.platform IS NULL;

-- SMS was the table's original owner, so it is the safest fallback for any
-- orphaned legacy row whose processed campaign no longer exists.
UPDATE campaign_status_jobs
SET platform = 'sms'
WHERE platform IS NULL;

ALTER TABLE campaign_status_jobs
    ALTER COLUMN platform SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_campaign_status_jobs_platform'
          AND conrelid = 'campaign_status_jobs'::regclass
    ) THEN
        ALTER TABLE campaign_status_jobs
            ADD CONSTRAINT chk_campaign_status_jobs_platform
            CHECK (platform IN ('sms', 'bale', 'splus', 'rubika'));
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_campaign_status_jobs_platform_scheduled_retry
    ON campaign_status_jobs(platform, scheduled_at, retry_count)
    WHERE executed_at IS NULL;

COMMIT;
