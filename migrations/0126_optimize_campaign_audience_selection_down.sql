DROP INDEX CONCURRENTLY IF EXISTS idx_audience_profiles_campaign_id_phone;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audience_selections_audience_ids
    ON audience_selections USING gin (audience_ids);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_bundle_aud_sel_audience_ids
    ON bundle_audience_selections USING gin (audience_ids);
