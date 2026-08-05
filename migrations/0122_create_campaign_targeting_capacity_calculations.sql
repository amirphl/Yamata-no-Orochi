-- Persist append-oriented exact Smart Targeting capacity calculation generations
-- and their expiring candidate populations.

BEGIN;

CREATE TABLE IF NOT EXISTS campaign_targeting_capacity_calculations (
    id BIGSERIAL PRIMARY KEY,
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id),
    bundle_id INTEGER NOT NULL REFERENCES bundles(id),
    customer_id INTEGER NOT NULL REFERENCES customers(id),
    platform VARCHAR(32) NOT NULL CHECK (platform IN ('sms', 'bale', 'rubika', 'splus')),
    requested_by_customer_id INTEGER NOT NULL REFERENCES customers(id),
    selected_tag_ids BIGINT[] NOT NULL,
    selected_tags_hash CHAR(64) NOT NULL,
    input_hash CHAR(64) NOT NULL,
    selected_score_classes TEXT[] NOT NULL,
    selected_tag_count INTEGER NOT NULL CHECK (selected_tag_count > 0),
    raw_audience_count BIGINT NOT NULL DEFAULT 0 CHECK (raw_audience_count >= 0),
    eligible_unique_audience_count BIGINT NOT NULL DEFAULT 0 CHECK (eligible_unique_audience_count >= 0),
    approved_campaign_deduction BIGINT NOT NULL DEFAULT 0 CHECK (approved_campaign_deduction >= 0),
    usable_unique_audience_count BIGINT NOT NULL DEFAULT 0 CHECK (usable_unique_audience_count >= 0),
    allocation_fingerprint CHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL CHECK (status IN ('calculating', 'calculated', 'failed')),
    calculation_version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    error_code VARCHAR(128),
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_campaign_targeting_capacity_campaign_created
    ON campaign_targeting_capacity_calculations (campaign_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_campaign_targeting_capacity_campaign_status
    ON campaign_targeting_capacity_calculations (campaign_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_campaign_targeting_capacity_bundle
    ON campaign_targeting_capacity_calculations (bundle_id);
CREATE INDEX IF NOT EXISTS idx_campaign_targeting_capacity_tags_hash
    ON campaign_targeting_capacity_calculations (selected_tags_hash);
CREATE INDEX IF NOT EXISTS idx_campaign_targeting_capacity_expires
    ON campaign_targeting_capacity_calculations (expires_at);
CREATE INDEX IF NOT EXISTS idx_campaign_targeting_capacity_pending
    ON campaign_targeting_capacity_calculations (started_at ASC, created_at ASC, id ASC)
    WHERE status = 'calculating';
CREATE UNIQUE INDEX IF NOT EXISTS uk_campaign_targeting_capacity_one_calculating
    ON campaign_targeting_capacity_calculations (campaign_id)
    WHERE status = 'calculating';

CREATE TABLE IF NOT EXISTS campaign_targeting_candidate_stack (
    id BIGSERIAL PRIMARY KEY,
    calculation_id BIGINT NOT NULL REFERENCES campaign_targeting_capacity_calculations(id),
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id),
    bundle_id INTEGER NOT NULL REFERENCES bundles(id),
    audience_id BIGINT NOT NULL REFERENCES audience_profiles(id),
    audience_score DOUBLE PRECISION,
    score_class VARCHAR(16) NOT NULL,
    matched_tag_ids INTEGER[] NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    expires_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT uk_campaign_targeting_candidate_calculation_audience UNIQUE (calculation_id, audience_id)
);

CREATE INDEX IF NOT EXISTS idx_campaign_targeting_candidate_calculation
    ON campaign_targeting_candidate_stack (calculation_id, audience_id);
CREATE INDEX IF NOT EXISTS idx_campaign_targeting_candidate_campaign
    ON campaign_targeting_candidate_stack (campaign_id);
CREATE INDEX IF NOT EXISTS idx_campaign_targeting_candidate_bundle
    ON campaign_targeting_candidate_stack (bundle_id);
CREATE INDEX IF NOT EXISTS idx_campaign_targeting_candidate_expires
    ON campaign_targeting_candidate_stack (expires_at, id);

-- Exact-capacity deductions need this exact lookup shape.
CREATE INDEX IF NOT EXISTS idx_campaigns_bundle_capacity_reservations
    ON campaigns (bundle_id, status, id) INCLUDE (num_audience)
    WHERE status IN ('approved', 'running', 'executed');

COMMIT;
