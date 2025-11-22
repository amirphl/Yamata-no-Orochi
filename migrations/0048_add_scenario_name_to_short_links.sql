-- Migration: 0048_add_scenario_name_to_short_links.sql
-- Description: Add scenario_name column to short_links and create trigram index for LIKE queries

-- UP MIGRATION

-- Enable pg_trgm extension for trigram indexes (safe if already enabled)
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Add nullable scenario_name column
ALTER TABLE short_links
	ADD COLUMN IF NOT EXISTS scenario_name TEXT NULL;

-- Ensure unique constraint on scenario_name (allows multiple NULLs as per Postgres semantics)
DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint
		WHERE conname = 'uk_short_links_scenario_name'
	) THEN
		ALTER TABLE short_links ADD CONSTRAINT uk_short_links_scenario_name UNIQUE (scenario_name);
	END IF;
END $$;

-- Trigram GIN index for efficient LIKE '%...%' queries
CREATE INDEX IF NOT EXISTS idx_short_links_scenario_name_trgm
	ON short_links USING GIN (scenario_name gin_trgm_ops); 