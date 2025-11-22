-- Migration: 0048_add_scenario_name_to_short_links_down.sql
-- Description: Drop scenario_name column and trigram index

-- DOWN MIGRATION

-- Drop index if exists
DROP INDEX IF EXISTS idx_short_links_scenario_name_trgm;

-- Drop unique constraint
ALTER TABLE short_links DROP CONSTRAINT IF EXISTS uk_short_links_scenario_name;

-- Drop column
ALTER TABLE short_links DROP COLUMN IF EXISTS scenario_name; 