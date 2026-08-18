-- Confirm configuration identity and whether a failed attempt persisted any
-- selection or processed checkpoint. Expected after an exact-count failure:
-- selection_record_count = processed_record_count = 0.

WITH params AS (
    SELECT ARRAY[984, 985, 986]::bigint[] AS campaign_ids
)
SELECT
    campaign.id AS campaign_id,
    campaign.status,
    campaign.bundle_id,
    campaign.num_audience AS requested_audience_count,
    campaign.spec->>'title' AS campaign_title,
    campaign.spec->>'platform' AS platform,
    campaign.spec->'audience_grades' AS audience_grades,
    campaign.spec->'tags' AS tags,
    MD5(JSONB_BUILD_OBJECT(
        'bundle_id', campaign.bundle_id,
        'platform', campaign.spec->'platform',
        'tags', campaign.spec->'tags',
        'audience_grades', campaign.spec->'audience_grades',
        'level1', campaign.spec->'level1',
        'level2s', campaign.spec->'level2s',
        'level3s', campaign.spec->'level3s'
    )::text) AS targeting_signature,
    selection.selection_record_count,
    selection.selection_member_count,
    processed.processed_record_count,
    processed.current_processed_campaign_id
FROM params
JOIN campaigns AS campaign
  ON campaign.id = ANY(params.campaign_ids)
LEFT JOIN LATERAL (
    SELECT
        COUNT(DISTINCT bundle_selection.id) AS selection_record_count,
        COUNT(member.id) AS selection_member_count
    FROM bundle_audience_selections AS bundle_selection
    LEFT JOIN bundle_audience_selection_members AS member
      ON member.selection_id = bundle_selection.id
    WHERE bundle_selection.campaign_id = campaign.id
) AS selection ON TRUE
LEFT JOIN LATERAL (
    SELECT
        COUNT(*) AS processed_record_count,
        MAX(processed_campaign.id) FILTER (
            WHERE processed_campaign.is_current
        ) AS current_processed_campaign_id
    FROM processed_campaigns AS processed_campaign
    WHERE processed_campaign.campaign_id = campaign.id
) AS processed ON TRUE
ORDER BY campaign.id;

