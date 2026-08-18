-- Exact standard-SMS reduction funnel for one failed campaign.
--
-- This mirrors scheduler predicates, including exact white/pink color, active
-- tags, configured grades, a usable phone, and exclusion of EVERY allocation
-- already present in bundle_audience_selection_members for bundle 318. It does
-- not use processed-campaign order for exclusion because the scheduler does
-- not use it either. Change params.campaign_id to 985 or 986 to cross-check;
-- identical targeting_signature values from configuration-and-persistence.sql
-- should yield the same result.
--
-- The tag-overlap GIN index limits the audience_profiles work to profiles that
-- match at least one configured tag. Run EXPLAIN (ANALYZE, BUFFERS) on this
-- query separately if its production plan needs investigation.

WITH params AS (
    SELECT 984::bigint AS campaign_id
),
campaign_config AS (
    SELECT
        campaign.id AS campaign_id,
        campaign.bundle_id,
        campaign.num_audience AS requested_audience_count,
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
        )::integer[] AS requested_tag_ids,
        CASE
            WHEN JSONB_TYPEOF(campaign.spec->'audience_grades') = 'array'
                THEN ARRAY(
                    SELECT UPPER(BTRIM(item.value))
                    FROM JSONB_ARRAY_ELEMENTS_TEXT(
                        campaign.spec->'audience_grades'
                    ) AS item(value)
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
        config.*,
        ARRAY(
            SELECT configured.tag_id
            FROM UNNEST(config.requested_tag_ids) AS configured(tag_id)
            JOIN tags AS active_tag
              ON active_tag.id = configured.tag_id
             AND active_tag.is_active IS TRUE
            ORDER BY configured.tag_id
        )::integer[] AS active_tag_ids,
        'A' = ANY(config.audience_grades) AS has_a,
        'B' = ANY(config.audience_grades) AS has_b,
        'C' = ANY(config.audience_grades) AS has_c,
        NOT (
            CARDINALITY(config.audience_grades) = 0
            OR config.audience_grades
                @> ARRAY['A', 'B', 'C']::text[]
        ) AS grade_filter_requested
    FROM campaign_config AS config
),
campaign_context AS (
    SELECT
        rules.*,
        CARDINALITY(rules.active_tag_ids)
            = CARDINALITY(rules.requested_tag_ids) AS all_tags_active,
        bounds.p33,
        bounds.p66
    FROM campaign_rules AS rules
    LEFT JOIN LATERAL (
        -- FetchPercentiles uses First. The scheduler log is authoritative if
        -- percentile-bound-candidates.sql reports more than one distinct pair.
        SELECT stats.p33, stats.p66
        FROM src_layer_all_stats AS stats
        WHERE stats.p33 IS NOT NULL
          AND stats.p66 IS NOT NULL
          AND (rules.level1 IS NULL
               OR stats.layer1_category = rules.level1)
          AND (CARDINALITY(rules.level2s) = 0
               OR stats.layer2_category = ANY(rules.level2s))
          AND (CARDINALITY(rules.level3s) = 0
               OR stats.layer3_category = ANY(rules.level3s))
        LIMIT 1
    ) AS bounds ON TRUE
),
candidate_flags AS (
    SELECT
        audience.id AS audience_id,
        audience.color IN ('white', 'pink') AS color_matches,
        CASE
            WHEN NOT context.grade_filter_requested THEN TRUE
            WHEN context.p33 IS NULL OR context.p66 IS NULL THEN FALSE
            WHEN audience.normalized_score IS NULL THEN FALSE
            WHEN context.has_a AND context.has_b
                THEN audience.normalized_score > context.p33
            WHEN context.has_b AND context.has_c
                THEN audience.normalized_score <= context.p66
            WHEN context.has_a AND context.has_c
                THEN audience.normalized_score <= context.p33
                  OR audience.normalized_score > context.p66
            WHEN context.has_a
                THEN audience.normalized_score > context.p66
            WHEN context.has_b
                THEN audience.normalized_score > context.p33
                 AND audience.normalized_score <= context.p66
            WHEN context.has_c
                THEN audience.normalized_score <= context.p33
            ELSE TRUE
        END AS grade_matches,
        audience.phone_number IS NOT NULL
          AND BTRIM(audience.phone_number) <> '' AS phone_matches,
        EXISTS (
            SELECT 1
            FROM bundle_audience_selection_members AS used
            WHERE used.bundle_id = context.bundle_id
              AND used.audience_id = audience.id
        ) AS already_used_in_bundle
    FROM campaign_context AS context
    JOIN audience_profiles AS audience
      ON audience.tags && context.active_tag_ids
    WHERE context.all_tags_active
),
counts AS (
    SELECT
        context.campaign_id,
        context.bundle_id,
        context.requested_audience_count,
        context.requested_tag_ids,
        context.active_tag_ids,
        context.all_tags_active,
        context.audience_grades,
        context.p33,
        context.p66,
        COUNT(flags.audience_id) AS tag_match_count,
        COUNT(flags.audience_id) FILTER (
            WHERE flags.color_matches
        ) AS tag_and_color_count,
        COUNT(flags.audience_id) FILTER (
            WHERE flags.color_matches AND flags.grade_matches
        ) AS tag_color_grade_count,
        COUNT(flags.audience_id) FILTER (
            WHERE flags.color_matches
              AND flags.grade_matches
              AND flags.phone_matches
        ) AS before_bundle_exclusion_count,
        COUNT(flags.audience_id) FILTER (
            WHERE flags.color_matches
              AND flags.grade_matches
              AND flags.phone_matches
              AND flags.already_used_in_bundle
        ) AS removed_by_bundle_ledger_count,
        COUNT(flags.audience_id) FILTER (
            WHERE flags.color_matches
              AND flags.grade_matches
              AND flags.phone_matches
              AND NOT flags.already_used_in_bundle
        ) AS scheduler_eligible_count,
        COUNT(flags.audience_id) FILTER (
            WHERE flags.color_matches
              AND flags.grade_matches
              AND flags.phone_matches
              AND NOT flags.already_used_in_bundle
              AND flags.audience_id IS NOT NULL
        ) AS final_count
    FROM campaign_context AS context
    LEFT JOIN candidate_flags AS flags ON TRUE
    GROUP BY
        context.campaign_id,
        context.bundle_id,
        context.requested_audience_count,
        context.requested_tag_ids,
        context.active_tag_ids,
        context.all_tags_active,
        context.audience_grades,
        context.p33,
        context.p66
)
SELECT
    counts.campaign_id,
    counts.bundle_id,
    counts.requested_audience_count,
    counts.requested_tag_ids,
    counts.active_tag_ids,
    counts.all_tags_active,
    counts.audience_grades,
    counts.p33,
    counts.p66,
    stage.stage_order,
    stage.stage,
    stage.audience_count,
    stage.removed_at_stage,
    counts.requested_audience_count - stage.audience_count
        AS shortfall_vs_requested,
    stage.audience_count >= counts.requested_audience_count
        AS enough_for_exact_allocation
FROM counts
CROSS JOIN LATERAL (
    VALUES
        (1, 'configured tag overlap',
            counts.tag_match_count, 0::bigint),
        (2, 'plus exact white/pink color',
            counts.tag_and_color_count,
            counts.tag_match_count - counts.tag_and_color_count),
        (3, 'plus configured audience grades',
            counts.tag_color_grade_count,
            counts.tag_and_color_count - counts.tag_color_grade_count),
        (4, 'plus usable phone',
            counts.before_bundle_exclusion_count,
            counts.tag_color_grade_count
                - counts.before_bundle_exclusion_count),
        (5, 'minus every prior bundle allocation',
            counts.scheduler_eligible_count,
            counts.removed_by_bundle_ledger_count)
) AS stage(stage_order, stage, audience_count, removed_at_stage)
ORDER BY stage.stage_order;
