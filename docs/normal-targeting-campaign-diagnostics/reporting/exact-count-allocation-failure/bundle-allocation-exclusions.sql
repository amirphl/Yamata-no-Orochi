-- Show all bundle allocations excluded by the scheduler. This query does not
-- read audience_profiles and should remain inexpensive with ~70 rows.
-- allocated_member_count is authoritative; recorded_audience_count is the
-- selection header's denormalized count and should match it.

WITH params AS (
    SELECT 318::bigint AS bundle_id
),
allocation_rows AS (
    SELECT
        selection.id AS selection_id,
        selection.created_at AS allocated_at,
        selection.campaign_id,
        campaign.status,
        campaign.spec->>'title' AS campaign_title,
        CASE
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
        END AS targeting_method,
        processed.id AS current_processed_campaign_id,
        selection.audience_count AS recorded_audience_count,
        COUNT(member.id) AS allocated_member_count
    FROM params
    JOIN bundle_audience_selections AS selection
      ON selection.bundle_id = params.bundle_id
    LEFT JOIN campaigns AS campaign
      ON campaign.id = selection.campaign_id
    LEFT JOIN processed_campaigns AS processed
      ON processed.campaign_id = campaign.id
     AND processed.is_current
    LEFT JOIN bundle_audience_selection_members AS member
      ON member.selection_id = selection.id
    GROUP BY
        selection.id,
        selection.created_at,
        selection.campaign_id,
        campaign.status,
        campaign.spec,
        processed.id,
        selection.audience_count
)
SELECT
    allocation.*,
    SUM(allocation.allocated_member_count) OVER (
        ORDER BY allocation.allocated_at, allocation.selection_id
    ) AS cumulative_bundle_allocated_count
FROM allocation_rows AS allocation
ORDER BY allocation.allocated_at, allocation.selection_id;

