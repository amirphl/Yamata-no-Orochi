-- Migration: 0111_create_bundle_audience_selections.sql
-- Description: Create bundle_audience_selections table to track cumulative audience IDs
--              used across campaigns within a bundle, scoped to (customer_id, bundle_id).

BEGIN;

CREATE TABLE IF NOT EXISTS bundle_audience_selections (
    id BIGSERIAL PRIMARY KEY,
    customer_id INTEGER NOT NULL,
    bundle_id INTEGER NOT NULL,
    correlation_id VARCHAR(128) NOT NULL UNIQUE,
    audience_ids BIGINT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);

CREATE INDEX IF NOT EXISTS idx_bundle_aud_sel_customer_bundle
    ON bundle_audience_selections (customer_id, bundle_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_bundle_aud_sel_audience_ids
    ON bundle_audience_selections USING gin (audience_ids);

COMMIT;
