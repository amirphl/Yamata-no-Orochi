-- Migration: 0061_create_segment_price_factors.sql
-- Description: Create segment_price_factors table to store price factors per level3 (latest row wins)

-- UP MIGRATION
BEGIN;

CREATE TABLE IF NOT EXISTS segment_price_factors (
	id SERIAL PRIMARY KEY,
	level3 VARCHAR(255) NOT NULL,
	price_factor NUMERIC(10,4) NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);

CREATE INDEX IF NOT EXISTS idx_segment_price_factors_level3 ON segment_price_factors (level3, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_segment_price_factors_created_at ON segment_price_factors (created_at);

COMMIT;
