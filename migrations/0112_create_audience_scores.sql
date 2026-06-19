-- Description: Create audience_scores table

BEGIN;

CREATE TABLE IF NOT EXISTS audience_scores (
    id BIGSERIAL PRIMARY KEY,
    phone_number BIGINT,
    audience_profile_id BIGINT REFERENCES audience_profiles(id),
    penalty NUMERIC,
    final_score NUMERIC,
    calculated_at TIMESTAMPTZ,
    normalized_score NUMERIC,
    percentile_score NUMERIC
);

CREATE INDEX IF NOT EXISTS idx_audience_scores_normalized_score
    ON audience_scores (normalized_score);

COMMIT;
