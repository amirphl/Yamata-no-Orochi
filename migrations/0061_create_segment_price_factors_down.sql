-- Migration: 0061_create_segment_price_factors_down.sql
-- Description: Drop segment_price_factors table

-- DOWN MIGRATION
BEGIN;

DROP TABLE IF EXISTS segment_price_factors;

COMMIT;
