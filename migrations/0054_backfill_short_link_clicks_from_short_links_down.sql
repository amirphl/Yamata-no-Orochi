-- Down Migration: Clear denormalized short_link data on short_link_clicks

BEGIN;

UPDATE short_link_clicks
SET uid = NULL,
    campaign_id = NULL,
    client_id = NULL,
    scenario_name = NULL,
    phone_number = NULL,
    long_link = NULL,
    short_link = NULL,
    short_link_created_at = NULL,
    short_link_updated_at = NULL;

COMMIT;
