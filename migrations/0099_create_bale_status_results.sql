-- Create table for Bale/Najva status results returned by provider
BEGIN;

CREATE TABLE IF NOT EXISTS bale_status_results (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES campaign_status_jobs(id) ON DELETE CASCADE,
    processed_campaign_id BIGINT NOT NULL REFERENCES processed_campaigns(id) ON DELETE CASCADE,
    tracking_id TEXT NOT NULL,
    server_id TEXT NULL,
    provider TEXT NULL,
    provider_status_code BIGINT NULL,
    provider_status_text TEXT NULL,
    total_parts BIGINT NULL,
    total_delivered_parts BIGINT NULL,
    total_undelivered_parts BIGINT NULL,
    total_unknown_parts BIGINT NULL,
    status TEXT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);

CREATE INDEX IF NOT EXISTS idx_bale_status_results_job_id ON bale_status_results(job_id);
CREATE INDEX IF NOT EXISTS idx_bale_status_results_processed_campaign_id ON bale_status_results(processed_campaign_id);
CREATE INDEX IF NOT EXISTS idx_bale_status_results_tracking_id ON bale_status_results(tracking_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bale_status_results_processed_campaign_tracking ON bale_status_results(processed_campaign_id, tracking_id);

COMMIT;
