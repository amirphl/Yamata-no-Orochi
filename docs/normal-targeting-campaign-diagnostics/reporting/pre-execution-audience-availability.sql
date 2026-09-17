-- Final pre-execution availability for the latest standard SMS campaign in
-- bundle 318
--
-- Set campaign_id to a specific campaign to make the preflight immutable, or
-- leave it NULL to use the highest standard-SMS campaign ID in the bundle.
-- Unlike reports based on processed-campaign order, this query excludes EVERY
-- audience in the bundle allocation ledger. That is the rule used by the
-- current scheduler,
-- including for legacy selections whose campaign_id is NULL.
--
-- The seven rows are hypothetical grade configurations. Counts include the
-- scheduler's exact tag-overlap, exact white/pink color, usable-phone, active-
-- tag, percentile, and bundle-ledger predicates. A/B/C has no score constraint
-- and consequently includes otherwise-eligible audiences with a NULL score.
-- SMS selection prioritizes white and then fills the remainder with pink.

WITH params AS (
    SELECT
        318::bigint AS bundle_id,
        NULL::bigint AS campaign_id
),
campaign_config AS MATERIALIZED (
    SELECT
        campaign.id AS campaign_id,
        campaign.bundle_id,
        campaign.status,
        campaign.phase,
        campaign.num_audience AS requested_audience_count,
        NULLIF(BTRIM(campaign.spec->>'title'), '') AS campaign_title,
        LOWER(BTRIM(campaign.spec->>'platform')) AS platform,
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
        COALESCE((
            SELECT ARRAY_AGG(
                DISTINCT BTRIM(item.value)::integer
                ORDER BY BTRIM(item.value)::integer
            )
            FROM JSONB_ARRAY_ELEMENTS_TEXT(
                CASE
                    WHEN JSONB_TYPEOF(campaign.spec->'tags') = 'array'
                        THEN campaign.spec->'tags'
                    ELSE '[]'::jsonb
                END
            ) AS item(value)
            WHERE BTRIM(item.value) <> ''
        ), '{}'::integer[]) AS requested_tag_ids
    FROM params
    JOIN campaigns AS campaign
      ON campaign.bundle_id = params.bundle_id
    WHERE (params.campaign_id IS NULL
           OR campaign.id = params.campaign_id)
      AND LOWER(BTRIM(campaign.spec->>'platform')) = 'sms'
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
    ORDER BY campaign.id DESC
    LIMIT 1
),
campaign_rules AS MATERIALIZED (
    SELECT
        config.*,
        COALESCE((
            SELECT ARRAY_AGG(configured.tag_id ORDER BY configured.tag_id)
            FROM UNNEST(config.requested_tag_ids) AS configured(tag_id)
            JOIN tags AS tag
              ON tag.id = configured.tag_id
             AND tag.is_active = TRUE
        ), '{}'::integer[]) AS active_tag_ids
    FROM campaign_config AS config
),
matching_stats AS MATERIALIZED (
    SELECT stats.*
    FROM campaign_rules AS rules
    JOIN src_layer_all_stats AS stats
      ON (rules.level1 IS NULL
          OR stats.layer1_category = rules.level1)
     AND (CARDINALITY(rules.level2s) = 0
          OR stats.layer2_category = ANY(rules.level2s))
     AND (CARDINALITY(rules.level3s) = 0
          OR stats.layer3_category = ANY(rules.level3s))
    WHERE stats.p33 IS NOT NULL
      AND stats.p66 IS NOT NULL
),
stats_summary AS (
    SELECT
        COUNT(*) AS matching_stats_row_count,
        COUNT(DISTINCT (stats.p33, stats.p66))
            AS distinct_percentile_pair_count
    FROM matching_stats AS stats
),
scheduler_bounds AS (
    -- Mirrors GORM First for this model, whose first field is layer1_category.
    -- The scheduler log remains authoritative if distinct pair count is > 1.
    SELECT stats.p33, stats.p66
    FROM matching_stats AS stats
    ORDER BY stats.layer1_category ASC
    LIMIT 1
),
campaign_context AS MATERIALIZED (
    SELECT
        rules.*,
        CARDINALITY(rules.requested_tag_ids) > 0
            AND CARDINALITY(rules.active_tag_ids)
                = CARDINALITY(rules.requested_tag_ids)
            AS all_tags_active,
        bounds.p33,
        bounds.p66,
        summary.matching_stats_row_count,
        summary.distinct_percentile_pair_count
    FROM campaign_rules AS rules
    CROSS JOIN stats_summary AS summary
    LEFT JOIN scheduler_bounds AS bounds ON TRUE
),
eligible_audiences AS (
    SELECT
        audience.color,
        audience.normalized_score
    FROM campaign_context AS context
    JOIN audience_profiles AS audience
      ON audience.tags && context.active_tag_ids
     AND audience.color IN ('white', 'pink')
     AND audience.phone_number IS NOT NULL
     AND BTRIM(audience.phone_number) <> ''
    WHERE context.all_tags_active
      AND NOT EXISTS (
          SELECT 1
          FROM bundle_audience_selection_members AS used
          WHERE used.bundle_id = context.bundle_id
            AND used.audience_id = audience.id
      )
),
available_counts AS (
    SELECT
        COUNT(*) FILTER (
            WHERE eligible.color = 'white'
        ) AS abc_white,
        COUNT(*) FILTER (
            WHERE eligible.color = 'pink'
        ) AS abc_pink,
        COUNT(*) FILTER (
            WHERE eligible.color = 'white'
              AND eligible.normalized_score > context.p66
        ) AS a_white,
        COUNT(*) FILTER (
            WHERE eligible.color = 'pink'
              AND eligible.normalized_score > context.p66
        ) AS a_pink,
        COUNT(*) FILTER (
            WHERE eligible.color = 'white'
              AND eligible.normalized_score > context.p33
              AND eligible.normalized_score <= context.p66
        ) AS b_white,
        COUNT(*) FILTER (
            WHERE eligible.color = 'pink'
              AND eligible.normalized_score > context.p33
              AND eligible.normalized_score <= context.p66
        ) AS b_pink,
        COUNT(*) FILTER (
            WHERE eligible.color = 'white'
              AND eligible.normalized_score <= context.p33
        ) AS c_white,
        COUNT(*) FILTER (
            WHERE eligible.color = 'pink'
              AND eligible.normalized_score <= context.p33
        ) AS c_pink,
        COUNT(*) FILTER (
            WHERE eligible.color = 'white'
              AND eligible.normalized_score > context.p33
        ) AS ab_white,
        COUNT(*) FILTER (
            WHERE eligible.color = 'pink'
              AND eligible.normalized_score > context.p33
        ) AS ab_pink,
        COUNT(*) FILTER (
            WHERE eligible.color = 'white'
              AND (eligible.normalized_score <= context.p33
                   OR eligible.normalized_score > context.p66)
        ) AS ac_white,
        COUNT(*) FILTER (
            WHERE eligible.color = 'pink'
              AND (eligible.normalized_score <= context.p33
                   OR eligible.normalized_score > context.p66)
        ) AS ac_pink,
        COUNT(*) FILTER (
            WHERE eligible.color = 'white'
              AND eligible.normalized_score <= context.p66
        ) AS bc_white,
        COUNT(*) FILTER (
            WHERE eligible.color = 'pink'
              AND eligible.normalized_score <= context.p66
        ) AS bc_pink,
        COUNT(*) FILTER (
            WHERE eligible.color IS NOT NULL
              AND eligible.normalized_score IS NULL
        ) AS unscored_count
    FROM campaign_context AS context
    LEFT JOIN eligible_audiences AS eligible ON TRUE
    GROUP BY context.campaign_id
),
grade_counts AS (
    SELECT
        context.*,
        counts.unscored_count,
        grade.grade_order,
        grade.grade_combination,
        CASE
            WHEN grade.needs_bounds
                 AND (context.p33 IS NULL OR context.p66 IS NULL) THEN NULL
            ELSE grade.white_count
        END AS available_white_count,
        CASE
            WHEN grade.needs_bounds
                 AND (context.p33 IS NULL OR context.p66 IS NULL) THEN NULL
            ELSE grade.pink_count
        END AS available_pink_count
    FROM campaign_context AS context
    JOIN available_counts AS counts ON TRUE
    CROSS JOIN LATERAL (
        VALUES
            (1, 'A', TRUE, counts.a_white, counts.a_pink),
            (2, 'B', TRUE, counts.b_white, counts.b_pink),
            (3, 'C', TRUE, counts.c_white, counts.c_pink),
            (4, 'A or B', TRUE, counts.ab_white, counts.ab_pink),
            (5, 'A or C', TRUE, counts.ac_white, counts.ac_pink),
            (6, 'B or C', TRUE, counts.bc_white, counts.bc_pink),
            (7, 'A or B or C', FALSE,
                counts.abc_white, counts.abc_pink)
    ) AS grade(
        grade_order,
        grade_combination,
        needs_bounds,
        white_count,
        pink_count
    )
)
SELECT
    result.bundle_id,
    result.campaign_id,
    result.campaign_title,
    result.status,
    result.phase,
    result.requested_audience_count,
    result.requested_tag_ids AS tag_ids,
    result.all_tags_active,
    result.p33,
    result.p66,
    result.matching_stats_row_count,
    result.distinct_percentile_pair_count,
    result.grade_order,
    result.grade_combination,
    result.available_white_count,
    result.available_pink_count,
    result.available_white_count + result.available_pink_count
        AS available_white_or_pink_count,
    CASE
        WHEN result.available_white_count IS NULL THEN NULL
        ELSE GREATEST(
            COALESCE(result.requested_audience_count, 0)::bigint
                - result.available_white_count
                - result.available_pink_count,
            0
        )
    END AS shortfall_vs_requested,
    CASE
        WHEN result.requested_audience_count IS NULL
          OR result.available_white_count IS NULL THEN NULL
        ELSE result.available_white_count + result.available_pink_count
            >= result.requested_audience_count
    END AS enough_for_exact_allocation,
    result.unscored_count AS available_unscored_count,
    result.all_tags_active
        AND (result.grade_combination = 'A or B or C'
             OR (result.p33 IS NOT NULL AND result.p66 IS NOT NULL))
        AS preflight_row_is_computable
FROM grade_counts AS result
ORDER BY result.grade_order;
