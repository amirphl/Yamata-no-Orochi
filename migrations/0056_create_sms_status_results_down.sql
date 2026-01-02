-- Drop SMS status results table
BEGIN;

DROP INDEX IF EXISTS idx_sms_status_results_campaign_customer;
DROP INDEX IF EXISTS idx_sms_status_results_customer_id;
DROP INDEX IF EXISTS idx_sms_status_results_processed_campaign_id;
DROP INDEX IF EXISTS idx_sms_status_results_job_id;
DROP TABLE IF EXISTS sms_status_results;

COMMIT;
