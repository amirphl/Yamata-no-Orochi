-- Validate the current processed-campaign checkpoint
--
-- For a completely prepared and sent campaign, all mismatch columns should be
-- false. The last-audience check is only expected to be false after the whole
-- audience has been processed.

WITH params AS (
    SELECT 1108::bigint AS campaign_id
),
selected AS (
    SELECT
        selection.id AS selection_id,
        COALESCE(
            ARRAY_AGG(member.audience_id ORDER BY member.selection_order)
                FILTER (WHERE member.audience_id IS NOT NULL),
            '{}'::bigint[]
        ) AS selected_audience_ids
    FROM params
    JOIN bundle_audience_selections AS selection
      ON selection.campaign_id = params.campaign_id
    LEFT JOIN bundle_audience_selection_members AS member
      ON member.selection_id = selection.id
    GROUP BY selection.id
)
SELECT
    processed.id AS processed_campaign_id,
    processed.is_current,
    processed.bundle_audience_selection_id,
    selected.selection_id AS expected_selection_id,
    CARDINALITY(processed.audience_ids) AS processed_audience_count,
    CARDINALITY(processed.audience_codes) AS processed_code_count,
    processed.bundle_audience_selection_id
        IS DISTINCT FROM selected.selection_id
        AS wrong_selection_reference,
    processed.audience_ids
        IS DISTINCT FROM selected.selected_audience_ids
        AS audience_ids_or_order_mismatch,
    CARDINALITY(processed.audience_ids)
        <> CARDINALITY(processed.audience_codes)
        AS id_code_count_mismatch,
    processed.last_audience_id
        IS DISTINCT FROM selected.selected_audience_ids[
            CARDINALITY(selected.selected_audience_ids)
        ] AS last_audience_id_mismatch
FROM params
JOIN processed_campaigns AS processed
  ON processed.campaign_id = params.campaign_id
 AND processed.is_current
JOIN selected ON TRUE;

