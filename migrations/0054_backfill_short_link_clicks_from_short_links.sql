-- Migration: Populate denormalized short_link_clicks fields from short_links

BEGIN;

UPDATE short_link_clicks c
SET uid = sl.uid,
    campaign_id = sl.campaign_id,
    client_id = sl.client_id,
    scenario_id = COALESCE(c.scenario_id, sl.scenario_id),
    scenario_name = sl.scenario_name,
    phone_number = sl.phone_number,
    long_link = sl.long_link,
    short_link = sl.short_link,
    short_link_created_at = sl.created_at,
    short_link_updated_at = sl.updated_at
FROM short_links sl
WHERE c.short_link_id = sl.id;

COMMIT;
