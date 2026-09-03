-- Make Smart Targeting Test sampling freshness independent from exact-capacity
-- generations. Existing version-1 results intentionally receive a sentinel
-- fingerprint and are treated as stale by the version-2 application flow.

BEGIN;

ALTER TABLE campaign_targeting_test_sampling_calculations
    ADD COLUMN IF NOT EXISTS allocation_fingerprint CHAR(64) NOT NULL
        DEFAULT REPEAT('0', 64);

ALTER TABLE campaign_targeting_test_sampling_calculations
    ALTER COLUMN allocation_fingerprint SET DEFAULT REPEAT('0', 64),
    ALTER COLUMN allocation_fingerprint SET NOT NULL,
    ALTER COLUMN calculation_version SET DEFAULT 2;

ALTER TABLE campaign_targeting_test_sampling_calculations
    DROP CONSTRAINT IF EXISTS campaign_targeting_test_sampling_allocation_fingerprint_valid,
    ADD CONSTRAINT campaign_targeting_test_sampling_allocation_fingerprint_valid
        CHECK (allocation_fingerprint ~ '^[0-9a-f]{64}$');

COMMIT;
