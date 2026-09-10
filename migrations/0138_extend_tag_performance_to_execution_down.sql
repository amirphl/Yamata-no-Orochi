BEGIN;

DROP INDEX IF EXISTS idx_campaigns_smart_execution_performance;
DROP TABLE IF EXISTS tag_overall_performance_summaries;
DROP INDEX IF EXISTS idx_campaign_tag_performances_bundle_phase_tag;

-- Feature 5 cannot interpret Execution rows. Remove only those derived rows;
-- immutable attribution remains available for a later Feature 6 reapply.
DELETE FROM campaign_tag_test_performances
WHERE phase_type = 'execution';
DELETE FROM campaign_tag_test_reports AS report
USING campaigns AS campaign
WHERE campaign.id = report.campaign_id
  AND campaign.phase = 'execution';

UPDATE campaign_tag_test_reports
SET calculation_version = 1,
    status = 'not_prepared',
    started_at = NULL,
    next_retry_at = NULL,
    error_code = NULL,
    error_message = NULL,
    updated_at = CURRENT_TIMESTAMP;
UPDATE campaign_tag_test_performances
SET calculation_version = 1,
    updated_at = CURRENT_TIMESTAMP;
UPDATE tag_test_phase_performance_summaries
SET calculation_version = 1,
    updated_at = CURRENT_TIMESTAMP;

ALTER TABLE tag_test_phase_performance_summaries
    ALTER COLUMN calculation_version SET DEFAULT 1;
ALTER TABLE campaign_tag_test_performances
    ALTER COLUMN calculation_version SET DEFAULT 1;
ALTER TABLE campaign_tag_test_reports
    ALTER COLUMN calculation_version SET DEFAULT 1;
ALTER TABLE campaign_tag_test_performances
    DROP COLUMN IF EXISTS phase_type;

COMMIT;
