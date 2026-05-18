-- Migration: 0105_create_rubika_status_results_down.sql
-- Description: Drop Rubika status results table

BEGIN;

DROP INDEX IF EXISTS idx_rubika_status_results_processed_campaign_tracking;
DROP INDEX IF EXISTS idx_rubika_status_results_tracking_id;
DROP INDEX IF EXISTS idx_rubika_status_results_processed_campaign_id;
DROP INDEX IF EXISTS idx_rubika_status_results_job_id;
DROP TABLE IF EXISTS rubika_status_results;

COMMIT;
