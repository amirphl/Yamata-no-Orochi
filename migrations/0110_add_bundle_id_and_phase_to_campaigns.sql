-- Description: Add bundle_id and phase to campaigns and link existing campaigns to bundles

BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_type
        WHERE typname = 'campaign_phase'
    ) THEN
        CREATE TYPE campaign_phase AS ENUM ('test', 'execution');
    END IF;
END
$$;

ALTER TABLE campaigns
    ADD COLUMN IF NOT EXISTS bundle_id INTEGER NULL REFERENCES bundles(id),
    ADD COLUMN IF NOT EXISTS phase campaign_phase NOT NULL DEFAULT 'execution';

CREATE INDEX IF NOT EXISTS idx_campaigns_bundle_id ON campaigns(bundle_id);

UPDATE campaigns c
SET
    bundle_id = b.id,
    phase = 'execution'
FROM bundles b
WHERE b.metadata->>'_backfill_campaign_id' = c.id::text
  AND (
      c.bundle_id IS NULL
      OR c.bundle_id <> b.id
      OR c.phase IS DISTINCT FROM 'execution'::campaign_phase
  );

UPDATE bundles
SET metadata = '{}'::jsonb
WHERE metadata->>'_backfill_campaign_id' IS NOT NULL;

COMMIT;
