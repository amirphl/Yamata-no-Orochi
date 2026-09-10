BEGIN;

ALTER TABLE campaign_targeting_capacity_calculations
    ALTER COLUMN calculation_version SET DEFAULT 2;

ALTER TABLE campaign_targeting_capacity_calculations
    DROP COLUMN IF EXISTS apply_bundle_audience_exclusions;

COMMIT;
