BEGIN;

DROP INDEX IF EXISTS idx_short_link_clicks_scenario_id;
ALTER TABLE short_link_clicks
    DROP COLUMN IF EXISTS scenario_id;

COMMIT; 