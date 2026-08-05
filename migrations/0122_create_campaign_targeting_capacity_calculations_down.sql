BEGIN;

DROP TABLE IF EXISTS campaign_targeting_candidate_stack;
DROP TABLE IF EXISTS campaign_targeting_capacity_calculations;
DROP INDEX IF EXISTS idx_campaigns_bundle_capacity_reservations;

COMMIT;
