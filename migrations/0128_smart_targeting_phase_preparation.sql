-- Feature 4: ordered Test sampling, per-tag sample size, deterministic send
-- order, and scheduler-time Smart Targeting audience attribution.

BEGIN;

ALTER TABLE campaigns
    ADD COLUMN IF NOT EXISTS sample_size_per_tag BIGINT,
    ADD COLUMN IF NOT EXISTS smart_targeting_test_satisfied_tag_ids INTEGER[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS smart_targeting_test_sampling_input_hash CHAR(64),
    ADD COLUMN IF NOT EXISTS smart_targeting_test_sampling_previewed_at TIMESTAMPTZ;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'campaigns'::regclass
          AND conname = 'campaigns_sample_size_per_tag_positive'
    ) THEN
        ALTER TABLE campaigns
            ADD CONSTRAINT campaigns_sample_size_per_tag_positive
            CHECK (sample_size_per_tag IS NULL OR sample_size_per_tag > 0);
    END IF;
END $$;

ALTER TABLE campaign_selected_tags
    ADD COLUMN IF NOT EXISTS selection_order INTEGER;

WITH ordered AS (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY campaign_id ORDER BY tag_id, id) - 1 AS position
    FROM campaign_selected_tags
)
UPDATE campaign_selected_tags AS selected
SET selection_order = ordered.position
FROM ordered
WHERE selected.id = ordered.id
  AND selected.selection_order IS NULL;

ALTER TABLE campaign_selected_tags
    ALTER COLUMN selection_order SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uk_campaign_selected_tags_campaign_order
    ON campaign_selected_tags (campaign_id, selection_order);

ALTER TABLE bundle_audience_selection_members
    ADD COLUMN IF NOT EXISTS selection_order BIGINT;

WITH ordered AS (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY selection_id ORDER BY audience_id, id) - 1 AS position
    FROM bundle_audience_selection_members
)
UPDATE bundle_audience_selection_members AS member
SET selection_order = ordered.position
FROM ordered
WHERE member.id = ordered.id
  AND member.selection_order IS NULL;

ALTER TABLE bundle_audience_selection_members
    ALTER COLUMN selection_order SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uk_bundle_aud_sel_member_selection_order
    ON bundle_audience_selection_members (selection_id, selection_order);

CREATE TABLE IF NOT EXISTS campaign_audience_tag_attributions (
    id BIGSERIAL PRIMARY KEY,
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    bundle_id INTEGER NOT NULL REFERENCES bundles(id),
    bundle_audience_selection_id BIGINT NOT NULL REFERENCES bundle_audience_selections(id),
    audience_id BIGINT NOT NULL REFERENCES audience_profiles(id),
    assigned_tag_id INTEGER NOT NULL REFERENCES tags(id),
    phase_type campaign_phase NOT NULL,
    selection_method VARCHAR(32) NOT NULL,
    selection_order BIGINT NOT NULL,
    audience_score NUMERIC,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    CONSTRAINT uk_campaign_audience_tag UNIQUE (campaign_id, audience_id),
    CONSTRAINT campaign_audience_tag_selection_order_nonnegative CHECK (selection_order >= 0),
    CONSTRAINT campaign_audience_tag_selection_method_valid CHECK (
        selection_method IN ('random_per_tag', 'score_desc')
    )
);

CREATE INDEX IF NOT EXISTS idx_campaign_audience_tag_bundle
    ON campaign_audience_tag_attributions (bundle_id);
CREATE INDEX IF NOT EXISTS idx_campaign_audience_tag_selection
    ON campaign_audience_tag_attributions (bundle_audience_selection_id, selection_order);
CREATE UNIQUE INDEX IF NOT EXISTS uk_campaign_audience_tag_selection_order
    ON campaign_audience_tag_attributions (bundle_audience_selection_id, selection_order);
CREATE INDEX IF NOT EXISTS idx_campaign_audience_tag_assigned
    ON campaign_audience_tag_attributions (campaign_id, assigned_tag_id);

COMMIT;
