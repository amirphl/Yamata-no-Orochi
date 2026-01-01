-- Drop SMS status check jobs table
BEGIN;

DROP INDEX IF EXISTS idx_sms_status_jobs_corr_id;
DROP INDEX IF EXISTS idx_sms_status_jobs_scheduled_retry;
DROP INDEX IF EXISTS idx_sms_status_jobs_processed_campaign_id;
DROP TABLE IF EXISTS sms_status_jobs;

COMMIT;
