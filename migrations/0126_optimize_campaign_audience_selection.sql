-- Optimize high-volume campaign audience selection and selection-snapshot writes.
--
-- CONCURRENTLY keeps the audience table available while the production indexes
-- are built. This migration must therefore be run without an enclosing BEGIN.

-- No application query performs containment/overlap searches on these snapshot
-- arrays. Indexing every ID made a 50k-audience snapshot create roughly 50k GIN
-- entries and added significant write amplification.
DROP INDEX CONCURRENTLY IF EXISTS idx_audience_selections_audience_ids;
DROP INDEX CONCURRENTLY IF EXISTS idx_bundle_aud_sel_audience_ids;

-- Audience eligibility is deliberately platform-independent.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audience_profiles_campaign_id_phone
    ON audience_profiles (id DESC)
    WHERE phone_number IS NOT NULL AND BTRIM(phone_number) <> '';

ANALYZE audience_profiles;
