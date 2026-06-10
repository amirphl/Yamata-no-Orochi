-- Migration: Drop bundles table
-- Down migration

BEGIN;

DROP TABLE IF EXISTS bundles;

COMMIT;
