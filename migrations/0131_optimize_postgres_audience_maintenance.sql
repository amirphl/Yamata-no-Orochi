-- Enable query-level I/O attribution in existing databases. The library is
-- already preloaded by docker/postgres/postgresql.conf.
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- pgstatginindex() reads the GIN metapage without scanning the 2.2 GiB index,
-- allowing the pending list to be monitored safely.
CREATE EXTENSION IF NOT EXISTS pgstattuple;

-- These non-unique indexes duplicate the indexes backing the existing UNIQUE
-- constraints. Removing them saves roughly 8.8 GiB on the measured production
-- database and reduces cache pressure and write amplification.
--
-- CONCURRENTLY keeps audience reads and writes available. Do not wrap this
-- migration in a transaction block.
DROP INDEX CONCURRENTLY IF EXISTS idx_audience_profiles_uid;
DROP INDEX CONCURRENTLY IF EXISTS idx_audience_profiles_phone_number;

-- The default 10-20% thresholds are too coarse for a 94-million-row table and
-- a multi-million-row allocation ledger. Keep statistics and reusable space
-- current without making global autovacuum thresholds overly aggressive.
ALTER TABLE audience_profiles SET (
    autovacuum_vacuum_scale_factor = 0.02,
    autovacuum_vacuum_threshold = 50000,
    autovacuum_analyze_scale_factor = 0.01,
    autovacuum_analyze_threshold = 50000
);

ALTER TABLE bundle_audience_selection_members SET (
    autovacuum_vacuum_scale_factor = 0.02,
    autovacuum_vacuum_threshold = 10000,
    autovacuum_analyze_scale_factor = 0.01,
    autovacuum_analyze_threshold = 10000
);

-- Tag frequencies are highly skewed (from zero to tens of millions). Retain a
-- larger element-frequency sample so overlap selectivity estimates are useful.
-- A maintenance-window ANALYZE activates this setting; it is not run here.
ALTER TABLE audience_profiles ALTER COLUMN tags SET STATISTICS 1000;
