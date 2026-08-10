BEGIN;

-- Superseded legacy snapshots intentionally retain a NULL campaign_id. Remove
-- the new-write guard before updating those rows to reconstruct audience_ids;
-- NOT VALID constraints still apply to row updates.
ALTER TABLE bundle_audience_selections
    DROP CONSTRAINT IF EXISTS bundle_audience_selection_campaign_required;

ALTER TABLE bundle_audience_selections
    ADD COLUMN IF NOT EXISTS audience_ids BIGINT[] NOT NULL DEFAULT '{}';

UPDATE bundle_audience_selections AS snapshot
SET audience_ids = COALESCE((
    SELECT array_agg(member.audience_id ORDER BY member.audience_id)
    FROM bundle_audience_selection_members AS member
    JOIN bundle_audience_selections AS owner ON owner.id = member.selection_id
    WHERE owner.customer_id = snapshot.customer_id
      AND owner.bundle_id = snapshot.bundle_id
      AND (owner.created_at, owner.id) <= (snapshot.created_at, snapshot.id)
), ARRAY[]::bigint[]);

DROP INDEX IF EXISTS uk_sent_bale_messages_processed_tracking;
DROP INDEX IF EXISTS uk_processed_campaigns_campaign_id;
CREATE INDEX IF NOT EXISTS idx_processed_campaigns_capacity_reservation_materialized
    ON processed_campaigns (campaign_id)
    WHERE bundle_audience_selection_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_processed_campaigns_campaign_id
    ON processed_campaigns (campaign_id);
CREATE INDEX IF NOT EXISTS idx_sent_bale_messages_processed_campaign_id
    ON sent_bale_messages (processed_campaign_id);
DROP TABLE IF EXISTS bundle_audience_selection_members;
DROP INDEX IF EXISTS uk_bundle_aud_sel_campaign;
DROP INDEX IF EXISTS uk_bundle_aud_sel_id_bundle;

ALTER TABLE processed_campaigns
    DROP COLUMN IF EXISTS is_current;

ALTER TABLE bundle_audience_selections
    DROP CONSTRAINT IF EXISTS bundle_audience_selection_count_nonnegative,
    DROP CONSTRAINT IF EXISTS bundle_audience_selections_campaign_id_fkey,
    DROP CONSTRAINT IF EXISTS bundle_audience_selections_customer_id_fkey,
    DROP CONSTRAINT IF EXISTS bundle_audience_selections_bundle_id_fkey;

-- Keep the new allocation columns on rollback to avoid destroying audit data.
ALTER TABLE campaign_targeting_capacity_calculations
    DROP CONSTRAINT IF EXISTS campaign_targeting_capacity_score_classes_valid,
    DROP CONSTRAINT IF EXISTS campaign_targeting_capacity_tag_count_matches,
    ALTER COLUMN expires_at DROP NOT NULL,
    ALTER COLUMN calculation_version SET DEFAULT 1;

COMMIT;
