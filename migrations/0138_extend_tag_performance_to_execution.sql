-- Feature 6: include Smart Targeting Execution Campaigns in the existing
-- per-Campaign tag materialization and derive global, delivered-based CTRs.

BEGIN;

ALTER TABLE campaign_tag_test_performances
    ADD COLUMN IF NOT EXISTS phase_type campaign_phase;

-- Every row created before Feature 6 came from the Test-only scheduler.
UPDATE campaign_tag_test_performances
SET phase_type = 'test'
WHERE phase_type IS NULL;

ALTER TABLE campaign_tag_test_performances
    ALTER COLUMN phase_type SET NOT NULL;

ALTER TABLE campaign_tag_test_reports
    ALTER COLUMN calculation_version SET DEFAULT 2;
ALTER TABLE campaign_tag_test_performances
    ALTER COLUMN calculation_version SET DEFAULT 2;
ALTER TABLE tag_test_phase_performance_summaries
    ALTER COLUMN calculation_version SET DEFAULT 2;

CREATE INDEX IF NOT EXISTS idx_campaign_tag_performances_bundle_phase_tag
    ON campaign_tag_test_performances (bundle_id, phase_type, tag_id);

CREATE TABLE IF NOT EXISTS tag_overall_performance_summaries (
    tag_id BIGINT PRIMARY KEY REFERENCES tags(id),
    total_selected_count BIGINT NOT NULL,
    total_sent_count BIGINT NOT NULL,
    total_delivered_count BIGINT NOT NULL,
    total_click_count BIGINT NOT NULL,
    overall_avg_ctr NUMERIC GENERATED ALWAYS AS (
        CASE
            WHEN total_delivered_count = 0 THEN NULL
            ELSE total_click_count::NUMERIC / total_delivered_count::NUMERIC
        END
    ) STORED,
    calculation_version INTEGER NOT NULL DEFAULT 2,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT tag_overall_performance_version_positive CHECK (calculation_version > 0),
    CONSTRAINT tag_overall_performance_counts_nonnegative CHECK (
        total_selected_count >= 0
        AND total_sent_count >= 0
        AND total_delivered_count >= 0
        AND total_click_count >= 0
    ),
    CONSTRAINT tag_overall_performance_count_order_valid CHECK (
        total_delivered_count <= total_sent_count
        AND total_sent_count <= total_selected_count
        AND total_click_count <= total_selected_count
    )
);

-- Preserve immediately usable overall metrics for already materialized Test
-- Campaigns. The scheduler reconciles these rows at calculation version 2 and
-- adds Execution Campaigns without incrementing counters.
INSERT INTO tag_overall_performance_summaries (
    tag_id,
    total_selected_count,
    total_sent_count,
    total_delivered_count,
    total_click_count,
    calculation_version
)
SELECT
    tag_id,
    SUM(selected_count),
    SUM(sent_count),
    SUM(delivered_count),
    SUM(click_count),
    2
FROM campaign_tag_test_performances
GROUP BY tag_id
ON CONFLICT (tag_id) DO UPDATE
SET total_selected_count = EXCLUDED.total_selected_count,
    total_sent_count = EXCLUDED.total_sent_count,
    total_delivered_count = EXCLUDED.total_delivered_count,
    total_click_count = EXCLUDED.total_click_count,
    calculation_version = EXCLUDED.calculation_version,
    updated_at = CURRENT_TIMESTAMP;

-- Feature 5 already created the equivalent Test partial index. This
-- complementary index lets discovery backfill and revisit Execution Campaigns
-- without penalizing unrelated Campaign writes.
CREATE INDEX IF NOT EXISTS idx_campaigns_smart_execution_performance
    ON campaigns (id, bundle_id)
    WHERE phase = 'execution'
      AND LOWER(BTRIM(COALESCE(spec->>'audience_targeting_method', ''))) = 'smart_targeting';

COMMIT;
