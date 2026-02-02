-- Migration: 0063_create_audience_selection_cache.sql
-- Description: Persist audience selection history per customer/tags hash with correlation IDs and link to processed campaigns

-- UP MIGRATION
DROP TABLE IF EXISTS audience_selection_states;

CREATE TABLE IF NOT EXISTS audience_selections (
    id BIGSERIAL PRIMARY KEY,
    customer_id INTEGER NOT NULL,
    tags_hash VARCHAR(128) NOT NULL,
    correlation_id VARCHAR(128) NOT NULL UNIQUE,
    audience_ids BIGINT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);

CREATE INDEX IF NOT EXISTS idx_audience_selections_customer_tags_created ON audience_selections (customer_id, tags_hash, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_audience_selections_audience_ids ON audience_selections USING gin (audience_ids);

ALTER TABLE processed_campaigns
    ADD COLUMN IF NOT EXISTS audience_selection_id BIGINT REFERENCES audience_selections(id);

CREATE INDEX IF NOT EXISTS idx_processed_campaigns_audience_selection_id ON processed_campaigns (audience_selection_id);
