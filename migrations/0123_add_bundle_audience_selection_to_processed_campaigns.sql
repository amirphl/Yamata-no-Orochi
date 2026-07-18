-- Link processed bundle campaigns to the bundle-scoped audience selection table.

BEGIN;

ALTER TABLE processed_campaigns
    ADD COLUMN IF NOT EXISTS bundle_audience_selection_id BIGINT
        REFERENCES bundle_audience_selections(id);

CREATE INDEX IF NOT EXISTS idx_processed_campaigns_bundle_audience_selection_id
    ON processed_campaigns (bundle_audience_selection_id);

-- Before this column existed, bundle selection IDs were incorrectly written to
-- audience_selection_id. Repair rows that succeeded only because the two
-- selection tables happened to contain the same numeric ID.
UPDATE processed_campaigns AS processed
SET bundle_audience_selection_id = processed.audience_selection_id,
    audience_selection_id = NULL
WHERE processed.audience_selection_id IS NOT NULL
  AND NULLIF(processed.campaign_json ->> 'bundle_id', '') IS NOT NULL
  AND EXISTS (
      SELECT 1
      FROM bundle_audience_selections AS bundle_selection
      WHERE bundle_selection.id = processed.audience_selection_id
  );

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'processed_campaigns'::regclass
          AND conname = 'processed_campaigns_single_audience_selection'
    ) THEN
        ALTER TABLE processed_campaigns
            ADD CONSTRAINT processed_campaigns_single_audience_selection
            CHECK (
                num_nonnulls(audience_selection_id, bundle_audience_selection_id) <= 1
                AND (
                    audience_selection_id IS NULL
                    OR NULLIF(campaign_json ->> 'bundle_id', '') IS NULL
                )
                AND (
                    bundle_audience_selection_id IS NULL
                    OR NULLIF(campaign_json ->> 'bundle_id', '') IS NOT NULL
                )
            );
    END IF;
END
$$;

COMMIT;
