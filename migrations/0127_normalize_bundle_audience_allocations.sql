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
    ADD COLUMN IF NOT EXISTS campaign_id INTEGER,
    ADD COLUMN IF NOT EXISTS audience_count BIGINT NOT NULL DEFAULT 0;
-- These rows are an immutable audit/exclusion ledger. They must survive a
-- campaign deletion and remain restorable when the target keeps a different
-- audience_profiles snapshot.
ALTER TABLE bundle_audience_selections
    DROP CONSTRAINT IF EXISTS bundle_audience_selections_campaign_id_fkey;

-- Older schedulers could persist more than one processed row when a campaign
-- was retried. Keep those rows (and the delivery/status history that references
-- them), but identify the newest row using the same id-descending rule as the
-- application repository. New inserts default to the current row.
ALTER TABLE processed_campaigns
    ADD COLUMN IF NOT EXISTS is_current BOOLEAN NOT NULL DEFAULT TRUE;
UPDATE processed_campaigns SET is_current = TRUE WHERE is_current IS NULL;
ALTER TABLE processed_campaigns
    ALTER COLUMN is_current SET DEFAULT TRUE,
    ALTER COLUMN is_current SET NOT NULL;

WITH ranked_processed AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY campaign_id ORDER BY id DESC) AS position
    FROM processed_campaigns
)
UPDATE processed_campaigns AS processed
SET is_current = (ranked.position = 1)
FROM ranked_processed AS ranked
WHERE processed.id = ranked.id
  -- The deployment helper reapplies required migrations. Once the partial
  -- unique index exists, preserve the elected row instead of re-electing by ID.
  AND NOT EXISTS (
      SELECT 1
      FROM pg_index
      WHERE indexrelid = to_regclass('uk_processed_campaigns_campaign_id')
        AND indisunique
        AND pg_get_expr(indpred, indrelid) = 'is_current'
  )
  AND processed.is_current IS DISTINCT FROM (ranked.position = 1);

-- Bale repositories retain duplicate legacy attempts and consistently read the
-- newest row. Preserve that history while allowing a partial unique index to
-- prevent new duplicate tracking checkpoints.
ALTER TABLE sent_bale_messages
    ADD COLUMN IF NOT EXISTS is_current BOOLEAN NOT NULL DEFAULT TRUE;
UPDATE sent_bale_messages SET is_current = TRUE WHERE is_current IS NULL;
ALTER TABLE sent_bale_messages
    ALTER COLUMN is_current SET DEFAULT TRUE,
    ALTER COLUMN is_current SET NOT NULL;

WITH ranked_bale_messages AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY processed_campaign_id, tracking_id
               ORDER BY id DESC
           ) AS position
    FROM sent_bale_messages
)
UPDATE sent_bale_messages AS message
SET is_current = (ranked.position = 1)
FROM ranked_bale_messages AS ranked
WHERE message.id = ranked.id
  AND NOT EXISTS (
      SELECT 1
      FROM pg_index
      WHERE indexrelid = to_regclass('uk_sent_bale_messages_processed_tracking')
        AND indisunique
        AND pg_get_expr(indpred, indrelid) = 'is_current'
  )
  AND message.is_current IS DISTINCT FROM (ranked.position = 1);

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM processed_campaigns
        WHERE bundle_audience_selection_id IS NOT NULL
          AND is_current
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
    audience_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    CONSTRAINT fk_bundle_aud_sel_member_selection_bundle
        FOREIGN KEY (selection_id, bundle_id)
        REFERENCES bundle_audience_selections(id, bundle_id),
    CONSTRAINT uk_bundle_aud_sel_member_selection_audience UNIQUE (selection_id, audience_id),
    CONSTRAINT uk_bundle_aud_sel_member_bundle_audience UNIQUE (bundle_id, audience_id)
);
ALTER TABLE bundle_audience_selection_members
    DROP CONSTRAINT IF EXISTS bundle_audience_selection_members_audience_id_fkey;

-- CREATE TABLE IF NOT EXISTS does not repair a table created by an older app or
-- restored independently. Ensure the conflict targets used by the backfill are
-- present before relying on them.
CREATE UNIQUE INDEX IF NOT EXISTS uk_bundle_aud_sel_member_selection_audience
    ON bundle_audience_selection_members (selection_id, audience_id);
CREATE UNIQUE INDEX IF NOT EXISTS uk_bundle_aud_sel_member_bundle_audience
    ON bundle_audience_selection_members (bundle_id, audience_id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'bundle_audience_selection_members'::regclass
          AND conname = 'fk_bundle_aud_sel_member_selection_bundle'
    ) THEN
        ALTER TABLE bundle_audience_selection_members
            ADD CONSTRAINT fk_bundle_aud_sel_member_selection_bundle
            FOREIGN KEY (selection_id, bundle_id)
            REFERENCES bundle_audience_selections(id, bundle_id) NOT VALID;
    END IF;
END $$;

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
        WITH distinct_snapshot_members AS (
            -- Legacy arrays were intended to be cumulative, but production
            -- snapshots can shrink and later contain the same audience again.
            -- De-duplicate each snapshot before choosing a historical owner.
            SELECT selection.id AS selection_id,
                   selection.bundle_id,
                   selection.created_at,
                   member.audience_id,
                   MIN(member.snapshot_order) AS snapshot_order
            FROM bundle_audience_selections AS selection
            CROSS JOIN LATERAL (
                SELECT audience_id, snapshot_order
                FROM unnest(selection.audience_ids) WITH ORDINALITY
                    AS source(audience_id, snapshot_order)
            ) AS member
            GROUP BY selection.id, selection.bundle_id, selection.created_at,
                     member.audience_id
        ), unowned_snapshot_members AS (
            -- A restored or partially normalized database may already contain
            -- ledger rows. Keep their original owner and backfill only pairs
            -- that are not represented in the normalized ledger yet.
            SELECT member.*
            FROM distinct_snapshot_members AS member
            WHERE NOT EXISTS (
                SELECT 1
                FROM bundle_audience_selection_members AS existing
                WHERE existing.bundle_id = member.bundle_id
                  AND existing.audience_id = member.audience_id
            )
        ),
        ranked_allocations AS (
            -- The normalized ledger permits an audience only once per bundle.
            -- Attribute it to its first persisted snapshot, even if it vanished
            -- from an intermediate cumulative array and later reappeared.
            SELECT member.*,
                   ROW_NUMBER() OVER (
                       PARTITION BY member.bundle_id, member.audience_id
                       ORDER BY member.created_at, member.selection_id
                   ) AS allocation_position
            FROM unowned_snapshot_members AS member
        )
        SELECT selection.id AS selection_id,
               selection.bundle_id,
               selection.created_at,
               COALESCE(
                   ARRAY_AGG(
                       allocation.audience_id
                       ORDER BY allocation.snapshot_order, allocation.audience_id
                   )
                       FILTER (WHERE allocation.audience_id IS NOT NULL),
                   ARRAY[]::bigint[]
               ) AS selected_ids
        FROM bundle_audience_selections AS selection
        LEFT JOIN ranked_allocations AS allocation
          ON allocation.selection_id = selection.id
         AND allocation.allocation_position = 1
        GROUP BY selection.id, selection.bundle_id, selection.created_at;

        IF EXISTS (
            SELECT 1
            FROM pg_attribute
            WHERE attrelid = 'bundle_audience_selection_members'::regclass
              AND attname = 'selection_order'
              AND attnotnull
              AND NOT attisdropped
        ) THEN
            -- A restored hybrid database may already have migration 0128's
            -- required ordering column. Continue each existing selection's
            -- order without weakening its NOT NULL constraint.
            INSERT INTO bundle_audience_selection_members
                (selection_id, bundle_id, audience_id, selection_order, created_at)
            SELECT backfill.selection_id,
                   backfill.bundle_id,
                   member.audience_id,
                   existing_order.maximum_order + member.member_order,
                   backfill.created_at
            FROM bundle_audience_allocation_backfill AS backfill
            CROSS JOIN LATERAL unnest(backfill.selected_ids) WITH ORDINALITY
                AS member(audience_id, member_order)
            CROSS JOIN LATERAL (
                SELECT COALESCE(MAX(selection_order), -1) AS maximum_order
                FROM bundle_audience_selection_members AS existing
                WHERE existing.selection_id = backfill.selection_id
            ) AS existing_order
            ORDER BY backfill.selection_id, member.member_order
            ON CONFLICT (bundle_id, audience_id) DO NOTHING;
        ELSE
            INSERT INTO bundle_audience_selection_members
                (selection_id, bundle_id, audience_id, created_at)
            SELECT backfill.selection_id,
                   backfill.bundle_id,
                   member.audience_id,
                   backfill.created_at
            FROM bundle_audience_allocation_backfill AS backfill
            CROSS JOIN LATERAL unnest(backfill.selected_ids) WITH ORDINALITY
                AS member(audience_id, member_order)
            ORDER BY backfill.selection_id, member.member_order
            ON CONFLICT (bundle_id, audience_id) DO NOTHING;
        END IF;

        -- Existing normalized rows are authoritative in hybrid/restored
        -- databases. Derive every count from the final ledger, not from the
        -- attempted insert set, so the repository's count invariant is exact.
        UPDATE bundle_audience_selections AS selection
        SET audience_count = (
            SELECT COUNT(*)
            FROM bundle_audience_selection_members AS member
            WHERE member.selection_id = selection.id
        )
        WHERE selection.audience_count IS DISTINCT FROM (
            SELECT COUNT(*)
            FROM bundle_audience_selection_members AS member
            WHERE member.selection_id = selection.id
        );

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
        RAISE EXCEPTION 'bundle audience selection counts do not match the normalized member ledger';
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

-- Replace any earlier full or differently-predicated indexes. The election
-- above ran first, so replacing them cannot reclassify retained history.
DO $$
BEGIN
    IF to_regclass('uk_processed_campaigns_campaign_id') IS NOT NULL AND NOT EXISTS (
        SELECT 1
        FROM pg_index
        WHERE indexrelid = to_regclass('uk_processed_campaigns_campaign_id')
          AND indisunique
          AND pg_get_expr(indpred, indrelid) = 'is_current'
    ) THEN
        DROP INDEX uk_processed_campaigns_campaign_id;
    END IF;
    IF to_regclass('uk_sent_bale_messages_processed_tracking') IS NOT NULL AND NOT EXISTS (
        SELECT 1
        FROM pg_index
        WHERE indexrelid = to_regclass('uk_sent_bale_messages_processed_tracking')
          AND indisunique
          AND pg_get_expr(indpred, indrelid) = 'is_current'
    ) THEN
        DROP INDEX uk_sent_bale_messages_processed_tracking;
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uk_processed_campaigns_campaign_id
    ON processed_campaigns (campaign_id)
    WHERE is_current;
CREATE UNIQUE INDEX IF NOT EXISTS uk_sent_bale_messages_processed_tracking
    ON sent_bale_messages (processed_campaign_id, tracking_id)
    WHERE is_current;
-- The partial unique campaign checkpoint index subsumes only the earlier
-- materialized-current lookup. Keep the normal campaign index because legacy
-- non-current rows remain queryable for delivery and status history.
DROP INDEX IF EXISTS idx_processed_campaigns_capacity_reservation_materialized;
-- Keep the normal processed-campaign index as well: the partial unique index
-- does not cover retained non-current delivery attempts.

COMMIT;
