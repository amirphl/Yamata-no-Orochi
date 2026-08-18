-- Validate selected profiles against normal-targeting execution rules
--
-- Normal-targeting grade boundaries are disjoint, matching Smart Targeting:
--   A   => score > p66
--   B   => p33 < score <= p66
--   C   => score <= p33
--   A+B => score > p33
--   B+C => score <= p66
--   A+C => score <= p33 OR score > p66
-- Empty grades or A+B+C has no score predicate. SMS additionally requires
-- exactly white or pink color.

WITH params AS (
    SELECT 955::bigint AS campaign_id
),
campaign_context AS (
    SELECT
        campaign.id AS campaign_id,
        LOWER(BTRIM(campaign.spec->>'platform')) AS platform,
        NULLIF(BTRIM(campaign.spec->>'level1'), '') AS level1,
        ARRAY(
            SELECT BTRIM(item.value)
            FROM JSONB_ARRAY_ELEMENTS_TEXT(campaign.spec->'level2s') AS item(value)
        )::text[] AS level2s,
        ARRAY(
            SELECT BTRIM(item.value)
            FROM JSONB_ARRAY_ELEMENTS_TEXT(campaign.spec->'level3s') AS item(value)
        )::text[] AS level3s,
        ARRAY(
            SELECT BTRIM(item.value)::integer
            FROM JSONB_ARRAY_ELEMENTS_TEXT(campaign.spec->'tags') AS item(value)
        )::integer[] AS tag_ids,
        CASE
            WHEN JSONB_TYPEOF(campaign.spec->'audience_grades') = 'array'
                THEN ARRAY(
                    SELECT UPPER(BTRIM(item.value))
                    FROM JSONB_ARRAY_ELEMENTS_TEXT(campaign.spec->'audience_grades')
                        AS item(value)
                    WHERE BTRIM(item.value) <> ''
                )::text[]
            ELSE ARRAY['A', 'B', 'C']::text[]
        END AS audience_grades
    FROM params
    JOIN campaigns AS campaign
      ON campaign.id = params.campaign_id
),
campaign_rules AS (
    SELECT
        context.*,
        'A' = ANY(context.audience_grades) AS has_a,
        'B' = ANY(context.audience_grades) AS has_b,
        'C' = ANY(context.audience_grades) AS has_c,
        NOT (
            CARDINALITY(context.audience_grades) = 0
            OR context.audience_grades @> ARRAY['A', 'B', 'C']::text[]
        ) AS grade_filter_requested
    FROM campaign_context AS context
),
matching_stats AS (
    SELECT stats.*
    FROM campaign_rules AS rules
    JOIN src_layer_all_stats AS stats
      ON stats.layer1_category = rules.level1
     AND stats.layer2_category = ANY(rules.level2s)
     AND stats.layer3_category = ANY(rules.level3s)
    WHERE stats.p33 IS NOT NULL
      AND stats.p66 IS NOT NULL
),
stats_summary AS (
    SELECT
        COUNT(*) AS matching_stats_row_count,
        COUNT(DISTINCT (p33, p66)) AS distinct_bound_pair_count
    FROM matching_stats
),
scheduler_bounds AS (
    -- Mirrors SrcLayerAllStatsRepository.FetchPercentiles: GORM First orders
    -- by the model's first column because this table has no primary key.
    SELECT stats.p33, stats.p66
    FROM matching_stats AS stats
    ORDER BY stats.layer1_category
    LIMIT 1
),
evaluated AS (
    SELECT
        member.audience_id,
        audience.id AS profile_id,
        audience.phone_number,
        audience.tags,
        audience.color,
        audience.normalized_score,
        rules.platform,
        rules.tag_ids,
        rules.audience_grades,
        rules.grade_filter_requested,
        bounds.p33,
        bounds.p66,
        CASE
            WHEN NOT rules.grade_filter_requested THEN TRUE
            WHEN bounds.p33 IS NULL OR bounds.p66 IS NULL THEN FALSE
            WHEN rules.has_a AND rules.has_b
                THEN audience.normalized_score > bounds.p33
            WHEN rules.has_b AND rules.has_c
                THEN audience.normalized_score <= bounds.p66
            WHEN rules.has_a AND rules.has_c
                THEN audience.normalized_score <= bounds.p33
                  OR audience.normalized_score > bounds.p66
            WHEN rules.has_a
                THEN audience.normalized_score > bounds.p66
            WHEN rules.has_b
                THEN audience.normalized_score > bounds.p33
                 AND audience.normalized_score <= bounds.p66
            WHEN rules.has_c
                THEN audience.normalized_score <= bounds.p33
            ELSE TRUE
        END AS score_is_eligible
    FROM campaign_rules AS rules
    JOIN bundle_audience_selections AS selection
      ON selection.campaign_id = rules.campaign_id
    JOIN bundle_audience_selection_members AS member
      ON member.selection_id = selection.id
    LEFT JOIN audience_profiles AS audience
      ON audience.id = member.audience_id
    LEFT JOIN scheduler_bounds AS bounds ON TRUE
)
SELECT
    COUNT(*) AS selected_count,
    COUNT(*) FILTER (WHERE profile_id IS NULL) AS missing_profile_count,
    COUNT(*) FILTER (
        WHERE phone_number IS NULL OR BTRIM(phone_number) = ''
    ) AS missing_phone_count,
    COUNT(*) FILTER (
        WHERE profile_id IS NOT NULL
          AND COALESCE(LOWER(BTRIM(color)), '')
                NOT IN ('white', 'pink', 'black')
    ) AS unknown_color_count,
    COUNT(*) FILTER (
        WHERE platform = 'sms'
          AND profile_id IS NOT NULL
          AND color NOT IN ('white', 'pink')
    ) AS sms_ineligible_color_count,
    COUNT(*) FILTER (
        WHERE profile_id IS NULL
           OR NOT COALESCE(tags && tag_ids, FALSE)
    ) AS belongs_to_no_campaign_tag_count,
    COUNT(*) FILTER (
        WHERE normalized_score > p66
    ) AS grade_a_count,
    COUNT(*) FILTER (
        WHERE normalized_score > p33
          AND normalized_score <= p66
    ) AS grade_b_count,
    COUNT(*) FILTER (
        WHERE normalized_score <= p33
    ) AS grade_c_count,
    COUNT(*) FILTER (
        WHERE normalized_score IS NULL
    ) AS unscored_count,
    COUNT(*) FILTER (
        WHERE NOT COALESCE(score_is_eligible, FALSE)
    ) AS selected_outside_requested_grades_count,
    (SELECT p33 FROM scheduler_bounds) AS scheduler_p33,
    (SELECT p66 FROM scheduler_bounds) AS scheduler_p66,
    (SELECT matching_stats_row_count FROM stats_summary)
        AS matching_stats_row_count,
    (SELECT distinct_bound_pair_count FROM stats_summary)
        AS distinct_bound_pair_count,
    (SELECT distinct_bound_pair_count = 1 FROM stats_summary)
        AS bounds_are_unambiguous
FROM evaluated;
