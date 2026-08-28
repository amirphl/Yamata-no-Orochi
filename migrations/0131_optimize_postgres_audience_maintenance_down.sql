-- Keep monitoring extensions installed: they may predate this migration, and
-- dropping pg_stat_statements would destroy accumulated query statistics.

ALTER TABLE audience_profiles RESET (
    autovacuum_vacuum_scale_factor,
    autovacuum_vacuum_threshold,
    autovacuum_analyze_scale_factor,
    autovacuum_analyze_threshold
);

ALTER TABLE bundle_audience_selection_members RESET (
    autovacuum_vacuum_scale_factor,
    autovacuum_vacuum_threshold,
    autovacuum_analyze_scale_factor,
    autovacuum_analyze_threshold
);

ALTER TABLE audience_profiles ALTER COLUMN tags SET STATISTICS -1;

-- CONCURRENTLY keeps audience reads and writes available. These indexes are
-- redundant, but recreating them makes an explicit rollback schema-complete.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audience_profiles_uid
    ON audience_profiles (uid);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audience_profiles_phone_number
    ON audience_profiles (phone_number);
