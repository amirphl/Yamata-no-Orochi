-- Campaign configuration, configured-tag scope, and score-bound resolution
--
-- Expected for a valid normal-targeting campaign:
--   targeting_method = standard
--   invalid_or_inactive_tag_count = 0
--   tag_outside_selected_levels_count = 0
--   selection_record_count = 1 after preparation
--   current_processed_record_count = 1 after preparation
--
-- If grade_filter_required is true, matching_stats_row_count must be positive.
-- bounds_are_unambiguous should also be true. The application currently calls
-- GORM First after filtering by all configured levels; when several matching
-- rows have different p33/p66 values, the selected threshold is not stable
-- enough to reconstruct historically.

WITH params AS (
    SELECT 955::bigint AS campaign_id
),
campaign_context AS (
    SELECT
        campaign.id AS campaign_id,
        campaign.status,
        campaign.phase,
        campaign.bundle_id,
        campaign.customer_id,
        campaign.num_audience,
        LOWER(BTRIM(campaign.spec->>'platform')) AS platform,
        CASE
            WHEN LOWER(BTRIM(campaign.spec->>'audience_targeting_method'))
                    IN ('standard', 'smart_targeting', 'excel')
                THEN LOWER(BTRIM(campaign.spec->>'audience_targeting_method'))
            WHEN NULLIF(BTRIM(campaign.spec->>'target_audience_excel_file_uuid'), '')
                    IS NOT NULL
                THEN 'excel'
            ELSE 'standard'
        END AS targeting_method,
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
        NOT (
            CARDINALITY(context.audience_grades) = 0
            OR context.audience_grades @> ARRAY['A', 'B', 'C']::text[]
        ) AS grade_filter_requested
    FROM campaign_context AS context
),
configured_tags AS (
    SELECT
        rules.campaign_id,
        configured.tag_id,
        reference.layer1_category,
        reference.layer2_category,
        reference.layer3_category,
        tag.id IS NOT NULL AS active_tag,
        reference.id IS NOT NULL
          AND reference.layer1_category = rules.level1
          AND reference.layer2_category = ANY(rules.level2s)
          AND reference.layer3_category = ANY(rules.level3s)
            AS inside_selected_levels
    FROM campaign_rules AS rules
    CROSS JOIN LATERAL UNNEST(rules.tag_ids) AS configured(tag_id)
    LEFT JOIN src_reference AS reference
      ON reference.id = configured.tag_id
    LEFT JOIN tags AS tag
      ON tag.id = configured.tag_id
     AND tag.is_active IS TRUE
),
matching_stats AS (
    SELECT
        stats.layer1_category,
        stats.layer2_category,
        stats.layer3_category,
        stats.calculated_at,
        stats.p33,
        stats.p66
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
        COUNT(DISTINCT (p33, p66)) AS distinct_bound_pair_count,
        MIN(p33) AS sole_p33,
        MIN(p66) AS sole_p66
    FROM matching_stats
)
SELECT
    rules.campaign_id,
    rules.status,
    rules.phase,
    rules.bundle_id,
    rules.customer_id,
    rules.platform,
    rules.targeting_method,
    rules.num_audience AS requested_audience_count,
    rules.level1,
    rules.level2s,
    rules.level3s,
    rules.tag_ids,
    rules.audience_grades,
    rules.grade_filter_requested AS grade_filter_required,
    stats.matching_stats_row_count,
    stats.distinct_bound_pair_count,
    stats.distinct_bound_pair_count = 1 AS bounds_are_unambiguous,
    CASE WHEN stats.distinct_bound_pair_count = 1 THEN stats.sole_p33 END
        AS unambiguous_p33,
    CASE WHEN stats.distinct_bound_pair_count = 1 THEN stats.sole_p66 END
        AS unambiguous_p66,
    (
        SELECT COUNT(*)
        FROM configured_tags
        WHERE NOT active_tag OR layer1_category IS NULL
    ) AS invalid_or_inactive_tag_count,
    (
        SELECT COUNT(*)
        FROM configured_tags
        WHERE layer1_category IS NOT NULL
          AND NOT inside_selected_levels
    ) AS tag_outside_selected_levels_count,
    (
        SELECT COUNT(*)
        FROM bundle_audience_selections AS selection
        WHERE selection.campaign_id = rules.campaign_id
    ) AS selection_record_count,
    (
        SELECT COUNT(*)
        FROM processed_campaigns AS processed
        WHERE processed.campaign_id = rules.campaign_id
          AND processed.is_current
    ) AS current_processed_record_count,
    (
        SELECT COUNT(*)
        FROM campaign_selected_tags AS selected_tag
        WHERE selected_tag.campaign_id = rules.campaign_id
    ) AS smart_targeting_selected_tag_rows,
    (
        SELECT COUNT(*)
        FROM campaign_audience_tag_attributions AS attribution
        WHERE attribution.campaign_id = rules.campaign_id
    ) AS smart_targeting_attribution_rows
FROM campaign_rules AS rules
CROSS JOIN stats_summary AS stats;
