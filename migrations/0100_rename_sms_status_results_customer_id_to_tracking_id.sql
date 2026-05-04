-- Rename sms_status_results.customer_id to tracking_id for naming consistency
BEGIN;

ALTER TABLE sms_status_results
	RENAME COLUMN customer_id TO tracking_id;

ALTER INDEX IF EXISTS idx_sms_status_results_customer_id
	RENAME TO idx_sms_status_results_tracking_id;

ALTER INDEX IF EXISTS idx_sms_status_results_processed_campaign_customer
	RENAME TO idx_sms_status_results_processed_campaign_tracking;

COMMIT;
