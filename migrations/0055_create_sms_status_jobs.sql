-- Create table for SMS status check jobs
BEGIN;

CREATE TABLE IF NOT EXISTS sms_status_jobs (
    id BIGSERIAL PRIMARY KEY,
    processed_campaign_id BIGINT NOT NULL REFERENCES processed_campaigns(id) ON DELETE CASCADE,
    correlation_id VARCHAR(64) NOT NULL,
    customer_ids TEXT[] NOT NULL,
    retry_count INT NOT NULL DEFAULT 0,
    scheduled_at TIMESTAMPTZ NOT NULL,
    executed_at TIMESTAMPTZ NULL,
    error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);

CREATE INDEX IF NOT EXISTS idx_sms_status_jobs_processed_campaign_id ON sms_status_jobs(processed_campaign_id);
CREATE INDEX IF NOT EXISTS idx_sms_status_jobs_scheduled_retry ON sms_status_jobs(scheduled_at, retry_count);
CREATE INDEX IF NOT EXISTS idx_sms_status_jobs_corr_id ON sms_status_jobs(correlation_id);

COMMIT;
