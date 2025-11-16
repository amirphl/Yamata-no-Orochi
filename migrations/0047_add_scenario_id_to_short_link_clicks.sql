BEGIN;

-- Add scenario_id to short_link_clicks
ALTER TABLE short_link_clicks
    ADD COLUMN IF NOT EXISTS scenario_id BIGINT NULL;

-- Backfill scenario_id from short_links
UPDATE short_link_clicks c
SET scenario_id = l.scenario_id
FROM short_links l
WHERE l.id = c.short_link_id
  AND (c.scenario_id IS NULL OR c.scenario_id <> l.scenario_id);

-- Index for faster filtering by scenario
CREATE INDEX IF NOT EXISTS idx_short_link_clicks_scenario_id ON short_link_clicks(scenario_id);

COMMIT; 