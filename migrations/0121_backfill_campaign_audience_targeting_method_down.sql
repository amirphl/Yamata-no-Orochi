-- Description: Audience targeting method backfill rollback
--
-- This is intentionally a no-op. Removing the JSON key would discard explicit
-- choices made after the migration and cannot safely reconstruct prior state.

BEGIN;

COMMIT;
