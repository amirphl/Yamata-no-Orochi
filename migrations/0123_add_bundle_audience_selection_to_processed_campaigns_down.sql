BEGIN;

ALTER TABLE processed_campaigns
    DROP CONSTRAINT IF EXISTS processed_campaigns_single_audience_selection;

DROP INDEX IF EXISTS idx_processed_campaigns_bundle_audience_selection_id;

ALTER TABLE processed_campaigns
    DROP COLUMN IF EXISTS bundle_audience_selection_id;

COMMIT;
