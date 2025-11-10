BEGIN;

-- Create sequence for scenario_id if it does not exist
CREATE SEQUENCE IF NOT EXISTS short_links_scenario_id_seq START WITH 1 INCREMENT BY 1;

-- Add scenario_id column (nullable) with default auto-generation
ALTER TABLE short_links
    ADD COLUMN IF NOT EXISTS scenario_id BIGINT NULL DEFAULT nextval('short_links_scenario_id_seq');

-- Optional index for faster lookup by scenario_id
CREATE INDEX IF NOT EXISTS idx_short_links_scenario_id ON short_links(scenario_id);

-- Comment
COMMENT ON COLUMN short_links.scenario_id IS 'Auto-generated scenario identifier (nullable)';

COMMIT; 