-- Drop Splus status results table
BEGIN;

DROP INDEX IF EXISTS idx_splus_status_results_processed_campaign_tracking;
DROP INDEX IF EXISTS idx_splus_status_results_tracking_id;
DROP INDEX IF EXISTS idx_splus_status_results_processed_campaign_id;
DROP INDEX IF EXISTS idx_splus_status_results_job_id;
DROP TABLE IF EXISTS splus_status_results;

COMMIT;
