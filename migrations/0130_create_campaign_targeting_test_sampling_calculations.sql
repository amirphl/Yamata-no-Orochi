-- Persist durable asynchronous Smart Targeting Test sampling jobs and their
-- aggregate per-tag results. Audience IDs are never stored in this table.

BEGIN;

CREATE TABLE IF NOT EXISTS campaign_targeting_test_sampling_calculations (
    id BIGSERIAL PRIMARY KEY,
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id),
    bundle_id INTEGER NOT NULL REFERENCES bundles(id),
    customer_id INTEGER NOT NULL REFERENCES customers(id),
    requested_by_customer_id INTEGER NOT NULL REFERENCES customers(id),
    selected_tag_ids BIGINT[] NOT NULL,
    input_hash CHAR(64) NOT NULL,
    selected_score_classes TEXT[] NOT NULL
        CONSTRAINT campaign_targeting_test_sampling_score_classes_valid
        CHECK (
            selected_score_classes = ARRAY['A']::text[] OR
            selected_score_classes = ARRAY['B']::text[] OR
            selected_score_classes = ARRAY['C']::text[] OR
            selected_score_classes = ARRAY['A', 'B']::text[] OR
            selected_score_classes = ARRAY['A', 'C']::text[] OR
            selected_score_classes = ARRAY['B', 'C']::text[] OR
            selected_score_classes = ARRAY['A', 'B', 'C']::text[]
        ),
    selected_tag_count INTEGER NOT NULL
        CONSTRAINT campaign_targeting_test_sampling_tag_count_matches
        CHECK (selected_tag_count > 0 AND selected_tag_count = cardinality(selected_tag_ids)),
    sample_size_per_tag BIGINT NOT NULL CHECK (sample_size_per_tag > 0),
    campaign_updated_at TIMESTAMPTZ,
    tag_results JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(tag_results) = 'array'),
    satisfied_tag_count INTEGER NOT NULL DEFAULT 0
        CHECK (satisfied_tag_count >= 0 AND satisfied_tag_count <= selected_tag_count),
    effective_audience_count BIGINT NOT NULL DEFAULT 0 CHECK (effective_audience_count >= 0),
    campaign_cost NUMERIC(20, 0) NOT NULL DEFAULT 0 CHECK (campaign_cost >= 0),
    status VARCHAR(32) NOT NULL CHECK (status IN ('calculating', 'calculated', 'failed')),
    calculation_version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    error_code VARCHAR(128),
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_campaign_targeting_test_sampling_campaign_created
    ON campaign_targeting_test_sampling_calculations (campaign_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_campaign_targeting_test_sampling_campaign_status
    ON campaign_targeting_test_sampling_calculations (campaign_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_campaign_targeting_test_sampling_bundle
    ON campaign_targeting_test_sampling_calculations (bundle_id);
CREATE INDEX IF NOT EXISTS idx_campaign_targeting_test_sampling_input
    ON campaign_targeting_test_sampling_calculations (input_hash);
CREATE INDEX IF NOT EXISTS idx_campaign_targeting_test_sampling_pending
    ON campaign_targeting_test_sampling_calculations (started_at ASC, created_at ASC, id ASC)
    WHERE status = 'calculating';
CREATE UNIQUE INDEX IF NOT EXISTS uk_campaign_targeting_test_sampling_one_calculating
    ON campaign_targeting_test_sampling_calculations (campaign_id)
    WHERE status = 'calculating';

COMMIT;
