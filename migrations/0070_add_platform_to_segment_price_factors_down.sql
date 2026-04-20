-- Migration: 0070_add_platform_to_segment_price_factors_down.sql
-- Description: Remove platform column and indexes from segment_price_factors

BEGIN;

DROP INDEX IF EXISTS idx_segment_price_factors_platform_level3_created_at;
DROP INDEX IF EXISTS idx_segment_price_factors_platform;

ALTER TABLE segment_price_factors
DROP COLUMN IF EXISTS platform;

COMMIT;
