-- Replace cumulative array snapshots with append-only campaign allocations and
-- a normalized uniqueness ledger. Capacity calculations retain counts only.

BEGIN;

DROP TABLE IF EXISTS campaign_targeting_candidate_stack;
ALTER TABLE campaign_targeting_capacity_calculations
    ALTER COLUMN calculation_version SET DEFAULT 2;
UPDATE campaign_targeting_capacity_calculations
SET expires_at = created_at + INTERVAL '24 hours'
WHERE expires_at IS NULL;
ALTER TABLE campaign_targeting_capacity_calculations
    ALTER COLUMN expires_at SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'campaign_targeting_capacity_calculations'::regclass
          AND conname = 'campaign_targeting_capacity_score_classes_valid'
    ) THEN
        ALTER TABLE campaign_targeting_capacity_calculations
            ADD CONSTRAINT campaign_targeting_capacity_score_classes_valid
            CHECK (
                selected_score_classes = ARRAY['A']::text[] OR
                selected_score_classes = ARRAY['B']::text[] OR
                selected_score_classes = ARRAY['C']::text[] OR
                selected_score_classes = ARRAY['A', 'B']::text[] OR
                selected_score_classes = ARRAY['A', 'C']::text[] OR
                selected_score_classes = ARRAY['B', 'C']::text[] OR
                selected_score_classes = ARRAY['A', 'B', 'C']::text[]
            );
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'campaign_targeting_capacity_calculations'::regclass
          AND conname = 'campaign_targeting_capacity_tag_count_matches'
    ) THEN
        ALTER TABLE campaign_targeting_capacity_calculations
            ADD CONSTRAINT campaign_targeting_capacity_tag_count_matches
            CHECK (selected_tag_count > 0 AND selected_tag_count = cardinality(selected_tag_ids));
    END IF;
END $$;

ALTER TABLE bundle_audience_selections
    ADD COLUMN IF NOT EXISTS campaign_id INTEGER REFERENCES campaigns(id),
    ADD COLUMN IF NOT EXISTS audience_count BIGINT NOT NULL DEFAULT 0;

-- Older schedulers could persist more than one processed row when a campaign
-- was retried. Keep those rows (and the delivery/status history that references
-- them), but identify the newest row using the same id-descending rule as the
-- application repository. New inserts default to the current row.
ALTER TABLE processed_campaigns
    ADD COLUMN IF NOT EXISTS is_current BOOLEAN NOT NULL DEFAULT TRUE;

WITH ranked_processed AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY campaign_id ORDER BY id DESC) AS position
    FROM processed_campaigns
)
UPDATE processed_campaigns AS processed
SET is_current = (ranked.position = 1)
FROM ranked_processed AS ranked
WHERE processed.id = ranked.id
  AND processed.is_current IS DISTINCT FROM (ranked.position = 1);

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM processed_campaigns
        WHERE bundle_audience_selection_id IS NOT NULL
        GROUP BY bundle_audience_selection_id
        HAVING COUNT(DISTINCT campaign_id) > 1
    ) THEN
        RAISE EXCEPTION 'a bundle audience selection is linked to multiple campaigns';
    END IF;
END $$;

UPDATE bundle_audience_selections AS selection
SET campaign_id = processed.campaign_id
FROM processed_campaigns AS processed
WHERE processed.bundle_audience_selection_id = selection.id
  AND processed.is_current
  AND selection.campaign_id IS NULL;

-- A retried legacy campaign can reference several immutable snapshots. Retain
-- every snapshot and its processed-campaign reference, but reserve campaign_id
-- for the canonical/current allocation so future retries are idempotent.
WITH ranked_selections AS (
    SELECT selection.id,
           ROW_NUMBER() OVER (
               PARTITION BY selection.campaign_id
               ORDER BY
                   EXISTS (
                       SELECT 1
                       FROM processed_campaigns AS processed
                       WHERE processed.bundle_audience_selection_id = selection.id
                         AND processed.is_current
                   ) DESC,
                   selection.created_at DESC,
                   selection.id DESC
           ) AS position
    FROM bundle_audience_selections AS selection
    WHERE selection.campaign_id IS NOT NULL
)
UPDATE bundle_audience_selections AS selection
SET campaign_id = NULL
FROM ranked_selections AS ranked
WHERE selection.id = ranked.id
  AND ranked.position > 1;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'bundle_audience_selections'::regclass
          AND conname = 'bundle_audience_selections_customer_id_fkey'
    ) THEN
        ALTER TABLE bundle_audience_selections
            ADD CONSTRAINT bundle_audience_selections_customer_id_fkey
            FOREIGN KEY (customer_id) REFERENCES customers(id) NOT VALID;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'bundle_audience_selections'::regclass
          AND conname = 'bundle_audience_selections_bundle_id_fkey'
    ) THEN
        ALTER TABLE bundle_audience_selections
            ADD CONSTRAINT bundle_audience_selections_bundle_id_fkey
            FOREIGN KEY (bundle_id) REFERENCES bundles(id) NOT VALID;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'bundle_audience_selections'::regclass
          AND conname = 'bundle_audience_selection_count_nonnegative'
    ) THEN
        ALTER TABLE bundle_audience_selections
            ADD CONSTRAINT bundle_audience_selection_count_nonnegative
            CHECK (audience_count >= 0) NOT VALID;
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uk_bundle_aud_sel_campaign
    ON bundle_audience_selections (campaign_id)
    WHERE campaign_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uk_bundle_aud_sel_id_bundle
    ON bundle_audience_selections (id, bundle_id);

CREATE TABLE IF NOT EXISTS bundle_audience_selection_members (
    id BIGSERIAL PRIMARY KEY,
    selection_id BIGINT NOT NULL,
    bundle_id INTEGER NOT NULL,
    audience_id BIGINT NOT NULL REFERENCES audience_profiles(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    CONSTRAINT fk_bundle_aud_sel_member_selection_bundle
        FOREIGN KEY (selection_id, bundle_id)
        REFERENCES bundle_audience_selections(id, bundle_id),
    CONSTRAINT uk_bundle_aud_sel_member_selection_audience UNIQUE (selection_id, audience_id),
    CONSTRAINT uk_bundle_aud_sel_member_bundle_audience UNIQUE (bundle_id, audience_id)
);

-- Older installations store one cumulative audience_ids array per snapshot.
-- On the first run only, derive the IDs appended by each immutable snapshot,
-- populate the normalized ledger, and then remove the legacy persisted copy.
-- Keeping this block conditional makes the whole migration safe to reapply.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'bundle_audience_selections'
          AND column_name = 'audience_ids'
    ) THEN
        CREATE TEMP TABLE bundle_audience_allocation_backfill
            ON COMMIT DROP AS
        SELECT current.id AS selection_id,
               current.bundle_id,
               current.created_at,
               ARRAY(
                   SELECT id
                   FROM unnest(current.audience_ids) AS id
                   EXCEPT
                   SELECT id
                   FROM unnest(COALESCE(previous.audience_ids, ARRAY[]::bigint[])) AS id
                   ORDER BY id
               ) AS selected_ids
        FROM bundle_audience_selections AS current
        LEFT JOIN LATERAL (
            SELECT older.audience_ids
            FROM bundle_audience_selections AS older
            WHERE older.customer_id = current.customer_id
              AND older.bundle_id = current.bundle_id
              AND (older.created_at, older.id) < (current.created_at, current.id)
            ORDER BY older.created_at DESC, older.id DESC
            LIMIT 1
        ) AS previous ON TRUE;

        UPDATE bundle_audience_selections AS selection
        SET audience_count = cardinality(backfill.selected_ids)
        FROM bundle_audience_allocation_backfill AS backfill
        WHERE selection.id = backfill.selection_id;

        INSERT INTO bundle_audience_selection_members
            (selection_id, bundle_id, audience_id, created_at)
        SELECT backfill.selection_id,
               backfill.bundle_id,
               audience_id,
               backfill.created_at
        FROM bundle_audience_allocation_backfill AS backfill
        CROSS JOIN LATERAL unnest(backfill.selected_ids) AS audience_id
        ON CONFLICT (bundle_id, audience_id) DO NOTHING;

        ALTER TABLE bundle_audience_selections DROP COLUMN audience_ids;
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM bundle_audience_selections AS selection
        WHERE selection.audience_count <> (
            SELECT COUNT(*)
            FROM bundle_audience_selection_members AS member
            WHERE member.selection_id = selection.id
        )
    ) THEN
        RAISE EXCEPTION 'legacy bundle audience selections contain overlapping or inconsistent allocations';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'bundle_audience_selections'::regclass
          AND conname = 'bundle_audience_selection_campaign_required'
    ) AND NOT (
        SELECT attnotnull
        FROM pg_attribute
        WHERE attrelid = 'bundle_audience_selections'::regclass
          AND attname = 'campaign_id'
          AND NOT attisdropped
    ) THEN
        -- Retain orphaned and superseded legacy snapshots for audit, while
        -- preventing new allocations without an idempotency identity. Add this
        -- only after the legacy rows have received their normalized counts;
        -- NOT VALID constraints still apply to subsequent row updates.
        ALTER TABLE bundle_audience_selections
            ADD CONSTRAINT bundle_audience_selection_campaign_required
            CHECK (campaign_id IS NOT NULL) NOT VALID;
    END IF;
END $$;

ALTER TABLE bundle_audience_selections
    ALTER COLUMN audience_count DROP DEFAULT;

CREATE UNIQUE INDEX IF NOT EXISTS uk_processed_campaigns_campaign_id
    ON processed_campaigns (campaign_id)
    WHERE is_current;
CREATE UNIQUE INDEX IF NOT EXISTS uk_sent_bale_messages_processed_tracking
    ON sent_bale_messages (processed_campaign_id, tracking_id);
-- The partial unique campaign checkpoint index subsumes only the earlier
-- materialized-current lookup. Keep the normal campaign index because legacy
-- non-current rows remain queryable for delivery and status history.
DROP INDEX IF EXISTS idx_processed_campaigns_capacity_reservation_materialized;
-- The composite unique index has the same processed-campaign prefix.
DROP INDEX IF EXISTS idx_sent_bale_messages_processed_campaign_id;

COMMIT;
