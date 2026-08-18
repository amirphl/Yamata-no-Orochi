-- Fast selection-integrity audit without audience_profiles
--
-- Use this version for large production datasets when ledger/checkpoint
-- integrity is the immediate concern. It never reads audience_profiles, so it
-- cannot validate current tags, grades, SMS color, phone number, or profile
-- existence. Work is proportional to selected ledger/checkpoint rows for this
-- bundle, not to the full audience population. Offending-ID samples are capped
-- at 100 values; the corresponding *_count columns remain complete.
--
-- Expected supporting indexes already exist in the application schema:
--   campaigns(bundle_id)
--   bundle_audience_selections(campaign_id) UNIQUE
--   bundle_audience_selection_members(selection_id, audience_id) UNIQUE
--   bundle_audience_selection_members(bundle_id, audience_id) UNIQUE
--   processed_campaigns(campaign_id) WHERE is_current UNIQUE

WITH params AS (
    SELECT 318::bigint AS bundle_id
),
target_campaigns AS (
    SELECT
        campaign.id AS campaign_id,
        campaign.bundle_id,
        campaign.status,
        campaign.phase,
        campaign.num_audience AS requested_audience_count,
        NULLIF(BTRIM(campaign.spec->>'title'), '') AS campaign_title,
        LOWER(BTRIM(campaign.spec->>'platform')) AS platform,
        processed.id AS processed_campaign_id,
        processed.bundle_audience_selection_id
            AS processed_selection_id,
        processed.audience_ids AS processed_audience_ids,
        ROW_NUMBER() OVER (
            ORDER BY
                processed.id ASC NULLS LAST,
                campaign.id ASC
        ) AS execution_position
    FROM params
    JOIN campaigns AS campaign
      ON campaign.bundle_id = params.bundle_id
    LEFT JOIN processed_campaigns AS processed
      ON processed.campaign_id = campaign.id
     AND processed.is_current
    WHERE campaign.status IN ('running', 'executed')
      AND CASE
          WHEN LOWER(BTRIM(COALESCE(
                  campaign.spec->>'audience_targeting_method', ''
              ))) IN ('standard', 'smart_targeting', 'excel')
              THEN LOWER(BTRIM(
                  campaign.spec->>'audience_targeting_method'
              ))
          WHEN NULLIF(BTRIM(
                  campaign.spec->>'target_audience_excel_file_uuid'
              ), '') IS NOT NULL
              THEN 'excel'
          ELSE 'standard'
      END = 'standard'
),
selection_records AS (
    SELECT
        campaign.campaign_id,
        COUNT(selection.id) AS selection_record_count,
        MIN(selection.id) AS sole_selection_id,
        COALESCE(SUM(selection.audience_count), 0)
            AS selection_recorded_count,
        COUNT(selection.id) FILTER (
            WHERE selection.bundle_id IS DISTINCT FROM campaign.bundle_id
        ) AS wrong_selection_bundle_count
    FROM target_campaigns AS campaign
    LEFT JOIN bundle_audience_selections AS selection
      ON selection.campaign_id = campaign.campaign_id
    GROUP BY campaign.campaign_id
),
target_members AS MATERIALIZED (
    SELECT
        campaign.campaign_id,
        campaign.bundle_id,
        campaign.processed_campaign_id,
        selection.id AS selection_id,
        member.id AS member_id,
        member.audience_id,
        member.selection_order,
        member.bundle_id AS member_bundle_id
    FROM target_campaigns AS campaign
    JOIN bundle_audience_selections AS selection
      ON selection.campaign_id = campaign.campaign_id
    JOIN bundle_audience_selection_members AS member
      ON member.selection_id = selection.id
),
member_structure AS (
    SELECT
        campaign.campaign_id,
        COUNT(member.member_id) AS selected_member_count,
        COUNT(member.member_id)
            - COUNT(DISTINCT member.audience_id)
            AS duplicate_within_campaign_count,
        COUNT(member.member_id)
            - COUNT(DISTINCT (
                member.selection_id,
                member.selection_order
            )) AS duplicate_selection_order_count,
        COUNT(member.member_id) FILTER (
            WHERE member.member_bundle_id
                IS DISTINCT FROM campaign.bundle_id
        ) AS wrong_member_bundle_count
    FROM target_campaigns AS campaign
    LEFT JOIN target_members AS member
      ON member.campaign_id = campaign.campaign_id
    GROUP BY campaign.campaign_id
),
duplicate_within_groups AS (
    SELECT
        member.campaign_id,
        member.audience_id,
        COUNT(*) AS occurrences
    FROM target_members AS member
    GROUP BY member.campaign_id, member.audience_id
    HAVING COUNT(*) > 1
),
duplicate_within_audit AS (
    SELECT
        duplicate.campaign_id,
        SUM(duplicate.occurrences - 1)
            AS duplicate_within_campaign_count,
        (ARRAY_AGG(duplicate.audience_id ORDER BY
            duplicate.audience_id
        ))[1:100] AS duplicate_within_campaign_sample_ids
    FROM duplicate_within_groups AS duplicate
    GROUP BY duplicate.campaign_id
),
previous_reuse_rows AS (
    SELECT DISTINCT
        current_member.campaign_id,
        current_member.audience_id
    FROM target_members AS current_member
    JOIN bundle_audience_selection_members AS previous_member
      ON previous_member.bundle_id = current_member.bundle_id
     AND previous_member.audience_id = current_member.audience_id
     AND previous_member.selection_id <> current_member.selection_id
    JOIN bundle_audience_selections AS previous_selection
      ON previous_selection.id = previous_member.selection_id
    JOIN campaigns AS previous_campaign
      ON previous_campaign.id = previous_selection.campaign_id
     AND previous_campaign.bundle_id = current_member.bundle_id
    JOIN processed_campaigns AS previous_processed
      ON previous_processed.campaign_id = previous_campaign.id
     AND previous_processed.is_current
    WHERE previous_processed.id < current_member.processed_campaign_id
       OR current_member.processed_campaign_id IS NULL
),
previous_reuse_audit AS (
    SELECT
        reused.campaign_id,
        COUNT(*) AS reused_from_previous_count,
        (ARRAY_AGG(reused.audience_id ORDER BY
            reused.audience_id
        ))[1:100] AS reused_from_previous_sample_ids
    FROM previous_reuse_rows AS reused
    GROUP BY reused.campaign_id
),
processed_duplicate_groups AS (
    SELECT
        campaign.campaign_id,
        processed_id.audience_id,
        COUNT(*) AS occurrences
    FROM target_campaigns AS campaign
    CROSS JOIN LATERAL UNNEST(COALESCE(
        campaign.processed_audience_ids,
        '{}'::bigint[]
    )) AS processed_id(audience_id)
    GROUP BY campaign.campaign_id, processed_id.audience_id
    HAVING COUNT(*) > 1
),
processed_duplicate_audit AS (
    SELECT
        duplicate.campaign_id,
        SUM(duplicate.occurrences - 1)
            AS duplicate_processed_count,
        (ARRAY_AGG(duplicate.audience_id ORDER BY
            duplicate.audience_id
        ))[1:100] AS duplicate_processed_sample_ids
    FROM processed_duplicate_groups AS duplicate
    GROUP BY duplicate.campaign_id
),
noncontiguous_selections AS (
    SELECT
        per_selection.campaign_id,
        COUNT(*) FILTER (
            WHERE per_selection.member_count > 0
              AND (
                  per_selection.minimum_order <> 0
                  OR per_selection.maximum_order
                      <> per_selection.member_count - 1
              )
        ) AS noncontiguous_selection_count
    FROM (
        SELECT
            member.campaign_id,
            member.selection_id,
            COUNT(*) AS member_count,
            MIN(member.selection_order) AS minimum_order,
            MAX(member.selection_order) AS maximum_order
        FROM target_members AS member
        GROUP BY member.campaign_id, member.selection_id
    ) AS per_selection
    GROUP BY per_selection.campaign_id
),
audit_inputs AS (
    SELECT
        campaign.*,
        records.selection_record_count,
        records.sole_selection_id,
        records.selection_recorded_count,
        records.wrong_selection_bundle_count,
        structure.selected_member_count,
        structure.duplicate_selection_order_count,
        structure.wrong_member_bundle_count,
        COALESCE(contiguous.noncontiguous_selection_count, 0)
            AS noncontiguous_selection_count,
        COALESCE(within_audit.duplicate_within_campaign_count, 0)
            AS duplicate_within_campaign_count,
        COALESCE(
            within_audit.duplicate_within_campaign_sample_ids,
            '{}'::bigint[]
        ) AS duplicate_within_campaign_sample_ids,
        COALESCE(previous.reused_from_previous_count, 0)
            AS reused_from_previous_count,
        COALESCE(
            previous.reused_from_previous_sample_ids,
            '{}'::bigint[]
        ) AS reused_from_previous_sample_ids,
        COALESCE(processed_duplicates.duplicate_processed_count, 0)
            AS duplicate_processed_count,
        COALESCE(
            processed_duplicates.duplicate_processed_sample_ids,
            '{}'::bigint[]
        ) AS duplicate_processed_sample_ids
    FROM target_campaigns AS campaign
    JOIN selection_records AS records
      ON records.campaign_id = campaign.campaign_id
    JOIN member_structure AS structure
      ON structure.campaign_id = campaign.campaign_id
    LEFT JOIN duplicate_within_audit AS within_audit
      ON within_audit.campaign_id = campaign.campaign_id
    LEFT JOIN previous_reuse_audit AS previous
      ON previous.campaign_id = campaign.campaign_id
    LEFT JOIN processed_duplicate_audit AS processed_duplicates
      ON processed_duplicates.campaign_id = campaign.campaign_id
    LEFT JOIN noncontiguous_selections AS contiguous
      ON contiguous.campaign_id = campaign.campaign_id
),
audit_results AS (
    SELECT
        audit.*,
        ARRAY_REMOVE(ARRAY[
            CASE WHEN audit.processed_campaign_id IS NULL
                THEN 'missing_current_processed_campaign' END,
            CASE WHEN audit.selection_record_count <> 1
                THEN 'selection_record_count_not_one' END,
            CASE WHEN audit.selection_recorded_count
                    <> audit.selected_member_count
                THEN 'selection_audience_count_mismatch' END,
            CASE WHEN audit.wrong_selection_bundle_count > 0
                THEN 'selection_bundle_mismatch' END,
            CASE WHEN audit.wrong_member_bundle_count > 0
                THEN 'selection_member_bundle_mismatch' END,
            CASE WHEN audit.duplicate_within_campaign_count > 0
                THEN 'duplicate_audience_within_campaign' END,
            CASE WHEN audit.duplicate_processed_count > 0
                THEN 'duplicate_audience_in_processed_checkpoint' END,
            CASE WHEN audit.reused_from_previous_count > 0
                THEN 'audience_reused_from_previous_campaign' END,
            CASE WHEN audit.duplicate_selection_order_count > 0
                THEN 'duplicate_selection_order' END,
            CASE WHEN audit.noncontiguous_selection_count > 0
                THEN 'noncontiguous_selection_order' END,
            CASE WHEN audit.requested_audience_count IS NOT NULL
                    AND audit.selected_member_count
                        > audit.requested_audience_count
                THEN 'selected_more_than_requested' END,
            CASE WHEN audit.selection_record_count = 1
                    AND audit.processed_selection_id IS DISTINCT FROM
                        audit.sole_selection_id
                THEN 'processed_selection_reference_mismatch' END,
            CASE WHEN audit.processed_campaign_id IS NOT NULL
                    AND CARDINALITY(audit.processed_audience_ids)
                        IS DISTINCT FROM audit.selected_member_count
                THEN 'processed_and_selection_count_mismatch' END
        ], NULL)::text[] AS failed_checks
    FROM audit_inputs AS audit
)
SELECT
    result.execution_position,
    result.processed_campaign_id,
    result.campaign_id,
    result.campaign_title,
    result.status,
    result.phase,
    result.platform,
    result.requested_audience_count,
    result.selected_member_count AS selected_audience_count,
    CARDINALITY(result.processed_audience_ids)
        AS processed_audience_count,
    CARDINALITY(result.failed_checks) = 0 AS is_valid,
    result.failed_checks,
    result.selection_record_count,
    result.selection_recorded_count,
    result.duplicate_within_campaign_count,
    result.duplicate_within_campaign_sample_ids,
    result.duplicate_processed_count,
    result.duplicate_processed_sample_ids,
    result.reused_from_previous_count,
    result.reused_from_previous_sample_ids,
    result.duplicate_selection_order_count,
    result.noncontiguous_selection_count,
    result.wrong_selection_bundle_count,
    result.wrong_member_bundle_count,
    result.processed_selection_id,
    result.sole_selection_id
FROM audit_results AS result
ORDER BY
    result.processed_campaign_id ASC NULLS LAST,
    result.campaign_id ASC;

