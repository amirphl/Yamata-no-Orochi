-- Revert campaign_status_jobs.tracking_ids back to customer_ids
BEGIN;

ALTER TABLE campaign_status_jobs
	RENAME COLUMN tracking_ids TO customer_ids;

COMMIT;
