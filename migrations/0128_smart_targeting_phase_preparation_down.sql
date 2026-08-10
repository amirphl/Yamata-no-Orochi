BEGIN;

DROP TABLE IF EXISTS campaign_audience_tag_attributions;
DROP INDEX IF EXISTS uk_bundle_aud_sel_member_selection_order;
ALTER TABLE bundle_audience_selection_members DROP CONSTRAINT IF EXISTS bundle_aud_sel_member_selection_order_nonnegative;
ALTER TABLE bundle_audience_selection_members DROP COLUMN IF EXISTS selection_order;
DROP INDEX IF EXISTS uk_campaign_selected_tags_campaign_order;
ALTER TABLE campaign_selected_tags DROP CONSTRAINT IF EXISTS campaign_selected_tags_selection_order_nonnegative;
ALTER TABLE campaign_selected_tags DROP COLUMN IF EXISTS selection_order;
ALTER TABLE campaigns DROP CONSTRAINT IF EXISTS campaigns_sample_size_per_tag_positive;
ALTER TABLE campaigns DROP COLUMN IF EXISTS smart_targeting_test_sampling_previewed_at;
ALTER TABLE campaigns DROP COLUMN IF EXISTS smart_targeting_test_sampling_input_hash;
ALTER TABLE campaigns DROP COLUMN IF EXISTS smart_targeting_test_satisfied_tag_ids;
ALTER TABLE campaigns DROP COLUMN IF EXISTS sample_size_per_tag;

COMMIT;
