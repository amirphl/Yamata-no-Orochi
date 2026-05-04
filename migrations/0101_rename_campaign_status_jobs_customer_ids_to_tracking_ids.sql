-- Rename campaign_status_jobs.customer_ids to tracking_ids for naming consistency
BEGIN;

ALTER TABLE campaign_status_jobs
	RENAME COLUMN customer_ids TO tracking_ids;

COMMIT;
