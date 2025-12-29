-- Down Migration: Remove denormalized short_link fields from short_link_clicks

BEGIN;

-- Drop indexes first
DROP INDEX IF EXISTS idx_short_link_clicks_scenario_name_trgm;
DROP INDEX IF EXISTS idx_short_link_clicks_phone_number;
DROP INDEX IF EXISTS idx_short_link_clicks_client_id;
DROP INDEX IF EXISTS idx_short_link_clicks_campaign_id;
DROP INDEX IF EXISTS idx_short_link_clicks_uid;

ALTER TABLE short_link_clicks
    DROP COLUMN IF EXISTS short_link_updated_at,
    DROP COLUMN IF EXISTS short_link_created_at,
    DROP COLUMN IF EXISTS short_link,
    DROP COLUMN IF EXISTS long_link,
    DROP COLUMN IF EXISTS phone_number,
    DROP COLUMN IF EXISTS scenario_name,
    DROP COLUMN IF EXISTS client_id,
    DROP COLUMN IF EXISTS campaign_id,
    DROP COLUMN IF EXISTS uid;

COMMIT;
