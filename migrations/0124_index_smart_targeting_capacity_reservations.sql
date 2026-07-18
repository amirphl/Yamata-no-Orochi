-- Support exact-capacity checks that distinguish an in-flight reservation from
-- a running campaign whose bundle audience selection has been materialized.

BEGIN;

CREATE INDEX IF NOT EXISTS idx_processed_campaigns_capacity_reservation_materialized
    ON processed_campaigns (campaign_id)
    WHERE bundle_audience_selection_id IS NOT NULL;

COMMIT;
