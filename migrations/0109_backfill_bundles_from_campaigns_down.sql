-- Description: Remove bundles that were backfilled from campaigns

BEGIN;

DELETE FROM bundles
WHERE metadata->>'_backfill_campaign_id' IS NOT NULL;

COMMIT;
