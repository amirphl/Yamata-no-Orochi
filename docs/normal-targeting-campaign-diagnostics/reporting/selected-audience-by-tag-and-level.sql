-- Selected audience counts by configured tag and level
--
-- A normal-targeting audience is selected from the union of campaign tags.
-- An audience can contain several configured tags, so rows intentionally use
-- COUNT(DISTINCT audience_id), and totals across rows are not additive.
-- Grade counts use the newest statistics row for that exact level path for
-- analysis. The selected-profile eligibility validation separately checks the
-- single threshold used by execution.

WITH params AS (
    SELECT 955::bigint AS campaign_id
),
campaign_context AS (
    SELECT
        campaign.id AS campaign_id,
        ARRAY(
            SELECT BTRIM(item.value)::integer
            FROM JSONB_ARRAY_ELEMENTS_TEXT(campaign.spec->'tags') AS item(value)
        )::integer[] AS tag_ids
    FROM params
    JOIN campaigns AS campaign
      ON campaign.id = params.campaign_id
),
configured_levels AS (
    SELECT
        configured.tag_id,
        reference.layer1_category,
        reference.layer2_category,
        reference.layer3_category,
        bounds.p33,
        bounds.p66,
        bounds.calculated_at
    FROM campaign_context AS context
    CROSS JOIN LATERAL UNNEST(context.tag_ids) AS configured(tag_id)
    LEFT JOIN src_reference AS reference
      ON reference.id = configured.tag_id
    LEFT JOIN LATERAL (
        SELECT stats.p33, stats.p66, stats.calculated_at
        FROM src_layer_all_stats AS stats
        WHERE stats.layer1_category = reference.layer1_category
          AND stats.layer2_category = reference.layer2_category
          AND stats.layer3_category = reference.layer3_category
          AND stats.p33 IS NOT NULL
          AND stats.p66 IS NOT NULL
        ORDER BY stats.calculated_at DESC NULLS LAST
        LIMIT 1
    ) AS bounds ON TRUE
)
SELECT
    level.tag_id,
    level.layer1_category,
    level.layer2_category,
    level.layer3_category,
    level.p33,
    level.p66,
    level.calculated_at AS bounds_calculated_at,
    COUNT(DISTINCT audience.id) AS selected_count,
    COUNT(DISTINCT audience.id) FILTER (
        WHERE LOWER(BTRIM(audience.color)) = 'white'
    ) AS white_count,
    COUNT(DISTINCT audience.id) FILTER (
        WHERE LOWER(BTRIM(audience.color)) = 'pink'
    ) AS pink_count,
    COUNT(DISTINCT audience.id) FILTER (
        WHERE LOWER(BTRIM(audience.color)) = 'black'
    ) AS black_count,
    COUNT(DISTINCT audience.id) FILTER (
        WHERE audience.normalized_score > level.p66
    ) AS grade_a_count,
    COUNT(DISTINCT audience.id) FILTER (
        WHERE audience.normalized_score > level.p33
          AND audience.normalized_score <= level.p66
    ) AS grade_b_count,
    COUNT(DISTINCT audience.id) FILTER (
        WHERE audience.normalized_score <= level.p33
    ) AS grade_c_count,
    COUNT(DISTINCT audience.id) FILTER (
        WHERE audience.normalized_score IS NULL
    ) AS unscored_count
FROM configured_levels AS level
LEFT JOIN bundle_audience_selections AS selection
  ON selection.campaign_id = (SELECT campaign_id FROM params)
LEFT JOIN bundle_audience_selection_members AS member
  ON member.selection_id = selection.id
LEFT JOIN audience_profiles AS audience
  ON audience.id = member.audience_id
 AND audience.tags @> ARRAY[level.tag_id]::integer[]
GROUP BY
    level.tag_id,
    level.layer1_category,
    level.layer2_category,
    level.layer3_category,
    level.p33,
    level.p66,
    level.calculated_at
ORDER BY
    level.layer1_category,
    level.layer2_category,
    level.layer3_category,
    level.tag_id;
