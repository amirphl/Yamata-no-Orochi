BEGIN;

ALTER TABLE campaign_targeting_test_sampling_calculations
    ALTER COLUMN calculation_version SET DEFAULT 1,
    DROP COLUMN IF EXISTS allocation_fingerprint;

COMMIT;
