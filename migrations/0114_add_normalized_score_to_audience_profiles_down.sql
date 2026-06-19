-- Description: Drop normalized_score from audience_profiles table

BEGIN;

DROP INDEX IF EXISTS idx_audience_profiles_color_normalized_score;
DROP INDEX IF EXISTS idx_audience_profiles_normalized_score;

ALTER TABLE audience_profiles DROP COLUMN IF EXISTS normalized_score;

COMMIT;
