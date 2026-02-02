-- Migration: 0063_create_audience_selection_cache_down.sql
-- Description: Drop audience selection tables and processed_campaigns column

-- DOWN MIGRATION
ALTER TABLE processed_campaigns DROP COLUMN IF EXISTS audience_selection_id;
DROP TABLE IF EXISTS audience_selections;
