-- Description: Remove bundle_id and phase from campaigns

BEGIN;

UPDATE bundles b
SET metadata = jsonb_build_object('_backfill_campaign_id', c.id)
FROM campaigns c
WHERE c.bundle_id = b.id
  AND c.bundle_id IS NOT NULL;

DROP INDEX IF EXISTS idx_campaigns_bundle_id;

ALTER TABLE campaigns
    DROP COLUMN IF EXISTS bundle_id,
    DROP COLUMN IF EXISTS phase;

DROP TYPE IF EXISTS campaign_phase;

COMMIT;
