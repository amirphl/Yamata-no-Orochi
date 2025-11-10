BEGIN;

-- Drop index if exists
DROP INDEX IF EXISTS idx_short_links_scenario_id;

-- Drop column
ALTER TABLE short_links
    DROP COLUMN IF EXISTS scenario_id;

-- Drop sequence
DROP SEQUENCE IF EXISTS short_links_scenario_id_seq;

COMMIT; 