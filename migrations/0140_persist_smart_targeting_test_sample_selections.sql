-- Immutable Smart Targeting Test sampling output.  A sampling preview is not
-- itself a bundle allocation: only the current preview is promoted to a
-- releasable reservation while a campaign waits to run.

BEGIN;

ALTER TABLE campaigns
    ADD COLUMN IF NOT EXISTS smart_targeting_test_sampling_generation BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS active_smart_targeting_test_selection_id BIGINT;

ALTER TABLE campaign_targeting_test_sampling_calculations
    ADD COLUMN IF NOT EXISTS generation BIGINT NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS uk_campaign_targeting_test_sampling_generation
    ON campaign_targeting_test_sampling_calculations (campaign_id, generation)
    WHERE generation > 0;

CREATE TABLE IF NOT EXISTS campaign_targeting_test_sample_selections (
    id BIGSERIAL PRIMARY KEY,
    calculation_id BIGINT NOT NULL UNIQUE REFERENCES campaign_targeting_test_sampling_calculations(id),
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id),
    bundle_id INTEGER NOT NULL REFERENCES bundles(id),
    generation BIGINT NOT NULL,
    input_hash CHAR(64) NOT NULL,
    effective_audience_count BIGINT NOT NULL CHECK (effective_audience_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    CONSTRAINT uk_campaign_targeting_test_sample_selection_generation UNIQUE (campaign_id, generation)
);

CREATE INDEX IF NOT EXISTS idx_campaign_targeting_test_sample_selections_campaign
    ON campaign_targeting_test_sample_selections (campaign_id, generation DESC);

CREATE TABLE IF NOT EXISTS campaign_targeting_test_sample_selection_members (
    id BIGSERIAL PRIMARY KEY,
    selection_id BIGINT NOT NULL REFERENCES campaign_targeting_test_sample_selections(id) ON DELETE CASCADE,
    audience_id BIGINT NOT NULL REFERENCES audience_profiles(id),
    assigned_tag_id INTEGER NOT NULL REFERENCES tags(id),
    tag_selection_order BIGINT NOT NULL CHECK (tag_selection_order >= 0),
    selection_order BIGINT NOT NULL CHECK (selection_order >= 0),
    audience_score NUMERIC,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    CONSTRAINT uk_campaign_targeting_test_sample_member_audience UNIQUE (selection_id, audience_id),
    CONSTRAINT uk_campaign_targeting_test_sample_member_order UNIQUE (selection_id, selection_order)
);

CREATE INDEX IF NOT EXISTS idx_campaign_targeting_test_sample_members_selection
    ON campaign_targeting_test_sample_selection_members (selection_id, selection_order);

-- Reservation rows are deliberately mutable lifecycle records.  The sample
-- selection and its members above remain append-only audit facts.
CREATE TABLE IF NOT EXISTS campaign_targeting_test_sample_reservations (
    id BIGSERIAL PRIMARY KEY,
    selection_id BIGINT NOT NULL REFERENCES campaign_targeting_test_sample_selections(id),
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id),
    bundle_id INTEGER NOT NULL REFERENCES bundles(id),
    audience_id BIGINT NOT NULL REFERENCES audience_profiles(id),
    state VARCHAR(16) NOT NULL CHECK (state IN ('active', 'released', 'materialized')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    released_at TIMESTAMPTZ,
    materialized_at TIMESTAMPTZ,
    CONSTRAINT uk_campaign_targeting_test_sample_reservation_member UNIQUE (selection_id, audience_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_campaign_targeting_test_sample_reservation_active_audience
    ON campaign_targeting_test_sample_reservations (bundle_id, audience_id)
    WHERE state = 'active';
CREATE INDEX IF NOT EXISTS idx_campaign_targeting_test_sample_reservations_campaign_state
    ON campaign_targeting_test_sample_reservations (campaign_id, state);

ALTER TABLE campaigns
    DROP CONSTRAINT IF EXISTS campaigns_active_smart_targeting_test_selection_id_fkey;
ALTER TABLE campaigns
    ADD CONSTRAINT campaigns_active_smart_targeting_test_selection_id_fkey
    FOREIGN KEY (active_smart_targeting_test_selection_id)
    REFERENCES campaign_targeting_test_sample_selections(id);

COMMIT;
