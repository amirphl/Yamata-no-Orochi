-- Description: Persist campaign-level Smart Targeting tag selection snapshots

BEGIN;

CREATE TABLE IF NOT EXISTS campaign_selected_tags (
    id BIGSERIAL PRIMARY KEY,
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    bundle_id INTEGER NOT NULL REFERENCES bundles(id),
    tag_id INTEGER NOT NULL REFERENCES tags(id),
    bundle_persona_fit_score_snapshot NUMERIC(5, 2),
    tag_display_title_snapshot TEXT,
    tag_audience_count_snapshot BIGINT,
    -- NULL means the per-tag CTR metric has not been measured/implemented;
    -- zero is reserved for an actual measured zero CTR.
    test_phase_avg_ctr_snapshot NUMERIC,
    overall_avg_ctr_snapshot NUMERIC,
    selected_by_customer_id INTEGER NOT NULL REFERENCES customers(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_campaign_selected_tags_campaign_tag UNIQUE (campaign_id, tag_id),
    CONSTRAINT campaign_selected_tags_fit_score_range CHECK (
        bundle_persona_fit_score_snapshot IS NULL OR
        (bundle_persona_fit_score_snapshot >= 0 AND bundle_persona_fit_score_snapshot <= 100)
    ),
    CONSTRAINT campaign_selected_tags_audience_count_nonnegative CHECK (
        tag_audience_count_snapshot IS NULL OR tag_audience_count_snapshot >= 0
    )
);

CREATE INDEX IF NOT EXISTS idx_campaign_selected_tags_campaign
    ON campaign_selected_tags (campaign_id, tag_id);

CREATE INDEX IF NOT EXISTS idx_campaign_selected_tags_bundle_tag
    ON campaign_selected_tags (bundle_id, tag_id);

-- Keep this migration safe to rerun in development environments where the
-- staged version may already have created the old column.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'campaign_selected_tags'
          AND column_name = 'selected_by_user_id'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'campaign_selected_tags'
          AND column_name = 'selected_by_customer_id'
    ) THEN
        ALTER TABLE campaign_selected_tags
            RENAME COLUMN selected_by_user_id TO selected_by_customer_id;
    END IF;
END
$$;

DROP INDEX IF EXISTS idx_campaign_selected_tags_selected_by_user;
CREATE INDEX IF NOT EXISTS idx_campaign_selected_tags_selected_by_customer
    ON campaign_selected_tags (selected_by_customer_id);

COMMIT;
