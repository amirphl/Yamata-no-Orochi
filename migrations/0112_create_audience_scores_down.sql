-- Description: Drop audience_scores table

BEGIN;

DROP INDEX IF EXISTS idx_audience_scores_normalized_score;

DROP TABLE IF EXISTS audience_scores;

COMMIT;
