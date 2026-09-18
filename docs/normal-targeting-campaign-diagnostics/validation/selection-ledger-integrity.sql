-- Validate selection-ledger scope, uniqueness, ordering, and bundle reuse
--
-- Expected: one selection record and every violation count equal to zero.

WITH params AS (
    SELECT 955::bigint AS campaign_id
),
selections AS (
    SELECT
        selection.*,
        campaign.bundle_id AS expected_bundle_id,
        campaign.customer_id AS expected_customer_id
    FROM params
    JOIN campaigns AS campaign
      ON campaign.id = params.campaign_id
    JOIN bundle_audience_selections AS selection
      ON selection.campaign_id = campaign.id
),
selection_statistics AS (
    SELECT
        selection.id AS selection_id,
        selection.audience_count,
        COUNT(member.id) AS member_count,
        COUNT(DISTINCT member.audience_id) AS distinct_audience_count,
        COUNT(DISTINCT member.selection_order) AS distinct_order_count,
        MIN(member.selection_order) AS minimum_order,
        MAX(member.selection_order) AS maximum_order
    FROM selections AS selection
    LEFT JOIN bundle_audience_selection_members AS member
      ON member.selection_id = selection.id
    GROUP BY selection.id, selection.audience_count
),
previously_used AS (
    SELECT DISTINCT current_member.audience_id
    FROM selections AS current_selection
    JOIN bundle_audience_selection_members AS current_member
      ON current_member.selection_id = current_selection.id
    JOIN bundle_audience_selection_members AS previous_member
      ON previous_member.bundle_id = current_selection.bundle_id
     AND previous_member.audience_id = current_member.audience_id
     AND previous_member.selection_id <> current_selection.id
    JOIN bundle_audience_selections AS previous_selection
      ON previous_selection.id = previous_member.selection_id
    WHERE previous_selection.campaign_id IS NOT NULL
      AND (
          previous_selection.created_at,
          previous_selection.id
      ) < (
          current_selection.created_at,
          current_selection.id
      )
)
SELECT
    (SELECT COUNT(*) FROM selections) AS selection_record_count,
    COUNT(*) FILTER (
        WHERE selection.bundle_id IS DISTINCT FROM selection.expected_bundle_id
           OR selection.customer_id IS DISTINCT FROM selection.expected_customer_id
    ) AS scope_violation_count,
    COALESCE(SUM(
        GREATEST(stats.member_count - stats.distinct_audience_count, 0)
    ), 0) AS duplicate_audience_occurrences,
    COALESCE(SUM(
        GREATEST(stats.member_count - stats.distinct_order_count, 0)
    ), 0) AS duplicate_order_occurrences,
    COUNT(*) FILTER (
        WHERE stats.audience_count <> stats.member_count
    ) AS audience_count_mismatches,
    COUNT(*) FILTER (
        WHERE stats.member_count > 0
          AND (
              stats.minimum_order <> 0
              OR stats.maximum_order <> stats.member_count - 1
          )
    ) AS non_contiguous_order_violations,
    (SELECT COUNT(*) FROM previously_used)
        AS previously_used_in_bundle_count
FROM selections AS selection
JOIN selection_statistics AS stats
  ON stats.selection_id = selection.id;

