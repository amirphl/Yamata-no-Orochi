-- Create table for SMS status results returned by provider
BEGIN;

CREATE TABLE IF NOT EXISTS sms_status_results (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES sms_status_jobs(id) ON DELETE CASCADE,
    processed_campaign_id BIGINT NOT NULL REFERENCES processed_campaigns(id) ON DELETE CASCADE,
    customer_id TEXT NOT NULL,
    server_id TEXT NULL,
    total_parts BIGINT NULL,
    total_delivered_parts BIGINT NULL,
    total_undelivered_parts BIGINT NULL,
    total_unknown_parts BIGINT NULL,
    status TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);

CREATE INDEX IF NOT EXISTS idx_sms_status_results_job_id ON sms_status_results(job_id);
CREATE INDEX IF NOT EXISTS idx_sms_status_results_processed_campaign_id ON sms_status_results(processed_campaign_id);
CREATE INDEX IF NOT EXISTS idx_sms_status_results_customer_id ON sms_status_results(customer_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sms_status_results_processed_campaign_customer ON sms_status_results(processed_campaign_id, customer_id);

COMMIT;
