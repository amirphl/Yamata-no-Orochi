BEGIN;

ALTER TABLE campaigns
    DROP CONSTRAINT IF EXISTS campaigns_active_smart_targeting_test_selection_id_fkey;
DROP TABLE IF EXISTS campaign_targeting_test_sample_reservations;
DROP TABLE IF EXISTS campaign_targeting_test_sample_selection_members;
DROP TABLE IF EXISTS campaign_targeting_test_sample_selections;
DROP INDEX IF EXISTS uk_campaign_targeting_test_sampling_generation;
ALTER TABLE campaign_targeting_test_sampling_calculations DROP COLUMN IF EXISTS generation;
ALTER TABLE campaigns
    DROP COLUMN IF EXISTS active_smart_targeting_test_selection_id,
    DROP COLUMN IF EXISTS smart_targeting_test_sampling_generation;

COMMIT;
