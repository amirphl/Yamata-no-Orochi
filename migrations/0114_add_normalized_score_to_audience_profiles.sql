-- Description: Add normalized_score to audience_profiles table

BEGIN;

ALTER TABLE audience_profiles ADD COLUMN IF NOT EXISTS normalized_score DOUBLE PRECISION;

CREATE INDEX IF NOT EXISTS idx_audience_profiles_normalized_score
    ON audience_profiles (normalized_score);

CREATE INDEX IF NOT EXISTS idx_audience_profiles_color_normalized_score
    ON audience_profiles (color, normalized_score);

COMMIT;
