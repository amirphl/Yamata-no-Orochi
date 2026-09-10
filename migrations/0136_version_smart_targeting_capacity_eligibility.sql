-- Version exact Smart Targeting capacity inputs so platform delivery colors
-- and Test-only Bundle exclusions are part of every durable generation.

BEGIN;

ALTER TABLE campaign_targeting_capacity_calculations
    ADD COLUMN IF NOT EXISTS apply_bundle_audience_exclusions BOOLEAN
        NOT NULL DEFAULT FALSE;

ALTER TABLE campaign_targeting_capacity_calculations
    ALTER COLUMN calculation_version SET DEFAULT 3;

COMMENT ON COLUMN campaign_targeting_capacity_calculations.apply_bundle_audience_exclusions IS
    'True when the generation excludes bundle_audience_exclusions (Smart Targeting Test phase)';

COMMIT;
