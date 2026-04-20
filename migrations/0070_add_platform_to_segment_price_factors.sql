-- Migration: 0070_add_platform_to_segment_price_factors.sql
-- Description: Add platform column and indexes to segment_price_factors

BEGIN;

ALTER TABLE segment_price_factors
ADD COLUMN IF NOT EXISTS platform VARCHAR(20) NOT NULL DEFAULT 'sms';

CREATE INDEX IF NOT EXISTS idx_segment_price_factors_platform
ON segment_price_factors(platform);

CREATE INDEX IF NOT EXISTS idx_segment_price_factors_platform_level3_created_at
ON segment_price_factors(platform, level3, created_at DESC);

COMMIT;
