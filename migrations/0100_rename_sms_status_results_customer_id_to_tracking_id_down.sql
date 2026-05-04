-- Revert sms_status_results.tracking_id back to customer_id
BEGIN;

ALTER TABLE sms_status_results
	RENAME COLUMN tracking_id TO customer_id;

ALTER INDEX IF EXISTS idx_sms_status_results_tracking_id
	RENAME TO idx_sms_status_results_customer_id;

ALTER INDEX IF EXISTS idx_sms_status_results_processed_campaign_tracking
	RENAME TO idx_sms_status_results_processed_campaign_customer;

COMMIT;
