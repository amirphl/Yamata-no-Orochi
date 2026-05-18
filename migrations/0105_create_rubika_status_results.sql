-- Migration: 0105_create_rubika_status_results.sql
-- Description: Create table for future Rubika status results returned by provider

BEGIN;

CREATE TABLE IF NOT EXISTS rubika_status_results (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES campaign_status_jobs(id) ON DELETE CASCADE,
    processed_campaign_id BIGINT NOT NULL REFERENCES processed_campaigns(id) ON DELETE CASCADE,
    tracking_id TEXT NOT NULL,
    server_id TEXT NULL,
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

CREATE INDEX IF NOT EXISTS idx_rubika_status_results_job_id ON rubika_status_results(job_id);
CREATE INDEX IF NOT EXISTS idx_rubika_status_results_processed_campaign_id ON rubika_status_results(processed_campaign_id);
CREATE INDEX IF NOT EXISTS idx_rubika_status_results_tracking_id ON rubika_status_results(tracking_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_rubika_status_results_processed_campaign_tracking ON rubika_status_results(processed_campaign_id, tracking_id);

COMMIT;
