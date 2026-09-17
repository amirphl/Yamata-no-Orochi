-- Available white/pink audience by hypothetical grade combination for the
-- last running/executed standard campaign in bundle 318
--
-- "Last" and "previous" use current processed_campaigns.id order. Previous
-- allocations from any targeting method consume bundle capacity; the current
-- campaign's own allocation is not excluded. A/B/C uses no score predicate,
-- matching scheduler behavior, and therefore includes unscored audiences.
-- The other six rows exclude unscored profiles. Grade boundaries are disjoint:
-- p33 belongs to C, p66 belongs to B, and A starts strictly above p66.
-- This query does not require a nonblank phone number because the requested
-- eligibility rules are tags + white/pink + not used by a previous campaign.

WITH params AS (
    SELECT 318::bigint AS bundle_id
),
last_campaign AS (
    SELECT
        campaign.id AS campaign_id,
        campaign.bundle_id,
        campaign.status,
        campaign.phase,
        NULLIF(BTRIM(campaign.spec->>'title'), '') AS campaign_title,
        LOWER(BTRIM(campaign.spec->>'platform')) AS platform,
        processed.id AS processed_campaign_id,
        NULLIF(BTRIM(campaign.spec->>'level1'), '') AS level1,
        ARRAY(
            SELECT BTRIM(item.value)
            FROM JSONB_ARRAY_ELEMENTS_TEXT(
                CASE
                    WHEN JSONB_TYPEOF(campaign.spec->'level2s') = 'array'
                        THEN campaign.spec->'level2s'
                    ELSE '[]'::jsonb
                END
            ) AS item(value)
            WHERE BTRIM(item.value) <> ''
        )::text[] AS level2s,
        ARRAY(
            SELECT BTRIM(item.value)
            FROM JSONB_ARRAY_ELEMENTS_TEXT(
                CASE
                    WHEN JSONB_TYPEOF(campaign.spec->'level3s') = 'array'
                        THEN campaign.spec->'level3s'
                    ELSE '[]'::jsonb
                END
            ) AS item(value)
            WHERE BTRIM(item.value) <> ''
        )::text[] AS level3s,
        ARRAY(
            SELECT BTRIM(item.value)::integer
            FROM JSONB_ARRAY_ELEMENTS_TEXT(
                CASE
                    WHEN JSONB_TYPEOF(campaign.spec->'tags') = 'array'
                        THEN campaign.spec->'tags'
                    ELSE '[]'::jsonb
                END
            ) AS item(value)
            WHERE BTRIM(item.value) <> ''
        )::integer[] AS tag_ids
    FROM params
    JOIN campaigns AS campaign
      ON campaign.bundle_id = params.bundle_id
    JOIN processed_campaigns AS processed
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
    ORDER BY processed.id DESC
    LIMIT 1
),
campaign_context AS (
    SELECT
        campaign.*,
        bounds.p33,
        bounds.p66
    FROM last_campaign AS campaign
    LEFT JOIN LATERAL (
        SELECT stats.p33, stats.p66
        FROM src_layer_all_stats AS stats
        WHERE stats.p33 IS NOT NULL
          AND stats.p66 IS NOT NULL
          AND (
              campaign.level1 IS NULL
              OR stats.layer1_category = campaign.level1
          )
          AND (
              CARDINALITY(campaign.level2s) = 0
              OR stats.layer2_category = ANY(campaign.level2s)
          )
          AND (
              CARDINALITY(campaign.level3s) = 0
              OR stats.layer3_category = ANY(campaign.level3s)
          )
        ORDER BY stats.layer1_category ASC
        LIMIT 1
    ) AS bounds ON TRUE
),
available_counts AS (
    SELECT
        campaign.campaign_id,
        campaign.bundle_id,
        campaign.processed_campaign_id,
        campaign.campaign_title,
        campaign.status,
        campaign.phase,
        campaign.platform,
        campaign.tag_ids,
        campaign.p33,
        campaign.p66,
        COUNT(*) FILTER (
            WHERE audience.normalized_score > campaign.p66
        ) AS grade_a_count,
        COUNT(*) FILTER (
            WHERE audience.normalized_score > campaign.p33
              AND audience.normalized_score <= campaign.p66
        ) AS grade_b_count,
        COUNT(*) FILTER (
            WHERE audience.normalized_score <= campaign.p33
        ) AS grade_c_count,
        COUNT(*) FILTER (
            WHERE audience.normalized_score > campaign.p33
        ) AS grade_ab_count,
        COUNT(*) FILTER (
            WHERE audience.normalized_score <= campaign.p33
               OR audience.normalized_score > campaign.p66
        ) AS grade_ac_count,
        COUNT(*) FILTER (
            WHERE audience.normalized_score <= campaign.p66
        ) AS grade_bc_count,
        COUNT(*) AS grade_abc_count,
        COUNT(*) FILTER (
            WHERE audience.normalized_score IS NULL
        ) AS unscored_count
    FROM campaign_context AS campaign
    JOIN audience_profiles AS audience
      ON audience.tags && campaign.tag_ids
     AND audience.color IN ('white', 'pink')
    WHERE NOT EXISTS (
        SELECT 1
        FROM bundle_audience_selection_members AS previous_member
        JOIN bundle_audience_selections AS previous_selection
          ON previous_selection.id = previous_member.selection_id
        JOIN campaigns AS previous_campaign
          ON previous_campaign.id = previous_selection.campaign_id
         AND previous_campaign.bundle_id = campaign.bundle_id
        JOIN processed_campaigns AS previous_processed
          ON previous_processed.campaign_id = previous_campaign.id
         AND previous_processed.is_current
        WHERE previous_member.bundle_id = campaign.bundle_id
          AND previous_member.audience_id = audience.id
          AND previous_processed.id < campaign.processed_campaign_id
    )
    GROUP BY
        campaign.campaign_id,
        campaign.bundle_id,
        campaign.processed_campaign_id,
        campaign.campaign_title,
        campaign.status,
        campaign.phase,
        campaign.platform,
        campaign.tag_ids,
        campaign.p33,
        campaign.p66
)
SELECT
    counts.bundle_id,
    counts.processed_campaign_id,
    counts.campaign_id,
    counts.campaign_title,
    counts.status,
    counts.phase,
    counts.platform,
    counts.tag_ids,
    counts.p33,
    counts.p66,
    counts.p33 IS NOT NULL AND counts.p66 IS NOT NULL
        AS score_bounds_found,
    grade.grade_order,
    grade.grade_combination,
    grade.available_audience_count,
    counts.unscored_count AS available_unscored_count
FROM available_counts AS counts
CROSS JOIN LATERAL (
    VALUES
        (1, 'A', counts.grade_a_count),
        (2, 'B', counts.grade_b_count),
        (3, 'C', counts.grade_c_count),
        (4, 'A or B', counts.grade_ab_count),
        (5, 'A or C', counts.grade_ac_count),
        (6, 'B or C', counts.grade_bc_count),
        (7, 'A or B or C', counts.grade_abc_count)
) AS grade(grade_order, grade_combination, available_audience_count)
ORDER BY grade.grade_order;
