-- Migration: Denormalize short_link_clicks with short_links fields (nullable)
-- Adds columns so each click row carries the original short link data

BEGIN;

ALTER TABLE short_link_clicks
    ADD COLUMN IF NOT EXISTS uid VARCHAR(64),
    ADD COLUMN IF NOT EXISTS campaign_id BIGINT,
    ADD COLUMN IF NOT EXISTS client_id BIGINT,
    ADD COLUMN IF NOT EXISTS scenario_name TEXT,
    ADD COLUMN IF NOT EXISTS phone_number VARCHAR(20),
    ADD COLUMN IF NOT EXISTS long_link TEXT,
    ADD COLUMN IF NOT EXISTS short_link TEXT,
    ADD COLUMN IF NOT EXISTS short_link_created_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS short_link_updated_at TIMESTAMPTZ;

-- Optional supporting indexes to mirror short_links access patterns
CREATE INDEX IF NOT EXISTS idx_short_link_clicks_uid ON short_link_clicks(uid);
CREATE INDEX IF NOT EXISTS idx_short_link_clicks_campaign_id ON short_link_clicks(campaign_id);
CREATE INDEX IF NOT EXISTS idx_short_link_clicks_client_id ON short_link_clicks(client_id);
CREATE INDEX IF NOT EXISTS idx_short_link_clicks_phone_number ON short_link_clicks(phone_number);

-- Trigram index for scenario_name LIKE/ILIKE queries (pg_trgm already enabled in 0048)
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS idx_short_link_clicks_scenario_name_trgm
    ON short_link_clicks USING GIN (scenario_name gin_trgm_ops);

COMMIT;
