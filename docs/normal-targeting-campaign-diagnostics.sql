-- Normal-targeting campaign diagnostics
--
-- Change campaign_id in each params CTE before running a query.
-- These queries intentionally read normal-targeting inputs from campaigns.spec:
--   * tags determine audience membership;
--   * level1/level2s/level3s resolve p33 and p66 in src_layer_all_stats;
--   * audience_grades determine the normalized_score predicate.
--
-- campaign_selected_tags and campaign_audience_tag_attributions belong to Smart
-- Targeting and are therefore not used as the source of truth here. Normal
-- targeting does not persist a per-audience assigned tag or score snapshot, so
-- membership and grade checks use the current audience_profiles rows.


-- 1. Campaign configuration, configured-tag scope, and score-bound resolution
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
        CARDINALITY(context.tag_ids) > 0
            AND NOT EXISTS (
                SELECT 1
                FROM UNNEST(context.tag_ids) AS configured(tag_id)
                WHERE configured.tag_id <> 17358
            ) AS grade_exempt,
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
    rules.grade_exempt,
    rules.grade_filter_requested AND NOT rules.grade_exempt
        AS grade_filter_required,
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


-- 2. Selected audience counts by configured tag and level
--
-- A normal-targeting audience is selected from the union of campaign tags.
-- An audience can contain several configured tags, so rows intentionally use
-- COUNT(DISTINCT audience_id), and totals across rows are not additive.
-- Grade counts use the newest statistics row for that exact level path for
-- analysis. Query 3 separately checks the single threshold used by execution.

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
        WHERE audience.normalized_score >= level.p66
    ) AS grade_a_count,
    COUNT(DISTINCT audience.id) FILTER (
        WHERE audience.normalized_score >= level.p33
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


-- 3. Validate selected profiles against normal-targeting execution rules
--
-- Normal-targeting grade boundaries are inclusive, matching the scheduler:
--   A   => score >= p66
--   B   => p33 <= score <= p66
--   C   => score <= p33
--   A+B => score >= p33
--   B+C => score <= p66
--   A+C => score <= p33 OR score >= p66
-- Empty grades, A+B+C, or a campaign containing only tag 17358 has no score
-- predicate. SMS additionally requires exactly white or pink color.

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
        CARDINALITY(context.tag_ids) > 0
            AND NOT EXISTS (
                SELECT 1
                FROM UNNEST(context.tag_ids) AS configured(tag_id)
                WHERE configured.tag_id <> 17358
            ) AS grade_exempt,
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
        rules.grade_exempt,
        rules.grade_filter_requested,
        bounds.p33,
        bounds.p66,
        CASE
            WHEN rules.grade_exempt OR NOT rules.grade_filter_requested THEN TRUE
            WHEN bounds.p33 IS NULL OR bounds.p66 IS NULL THEN FALSE
            WHEN rules.has_a AND rules.has_b
                THEN audience.normalized_score >= bounds.p33
            WHEN rules.has_b AND rules.has_c
                THEN audience.normalized_score <= bounds.p66
            WHEN rules.has_a AND rules.has_c
                THEN audience.normalized_score <= bounds.p33
                  OR audience.normalized_score >= bounds.p66
            WHEN rules.has_a
                THEN audience.normalized_score >= bounds.p66
            WHEN rules.has_b
                THEN audience.normalized_score >= bounds.p33
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
        WHERE normalized_score >= p66
    ) AS grade_a_count,
    COUNT(*) FILTER (
        WHERE normalized_score >= p33
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


-- 4. Reconcile campaign-wide audience counts
--
-- For a fully prepared and sent normal-targeting campaign, requested,
-- selection-member, processed-ID, processed-code, and provider-send counts
-- should agree. Smart-targeting attribution rows should be zero.

WITH params AS (
    SELECT 955::bigint AS campaign_id
),
campaign_context AS (
    SELECT
        campaign.id AS campaign_id,
        campaign.num_audience,
        LOWER(BTRIM(campaign.spec->>'platform')) AS platform
    FROM params
    JOIN campaigns AS campaign
      ON campaign.id = params.campaign_id
),
current_processed AS (
    SELECT processed.*
    FROM campaign_context AS context
    JOIN processed_campaigns AS processed
      ON processed.campaign_id = context.campaign_id
     AND processed.is_current
),
send_rows AS (
    SELECT 'sms'::text AS platform, processed_campaign_id
    FROM sent_sms

    UNION ALL

    SELECT 'bale', processed_campaign_id
    FROM sent_bale_messages
    WHERE is_current

    UNION ALL

    SELECT 'rubika', processed_campaign_id
    FROM sent_rubika_messages

    UNION ALL

    SELECT 'splus', processed_campaign_id
    FROM sent_splus_messages
)
SELECT
    context.num_audience AS requested_count,
    (
        SELECT COUNT(*)
        FROM bundle_audience_selections AS selection
        WHERE selection.campaign_id = context.campaign_id
    ) AS selection_record_count,
    COALESCE((
        SELECT SUM(selection.audience_count)
        FROM bundle_audience_selections AS selection
        WHERE selection.campaign_id = context.campaign_id
    ), 0) AS selection_recorded_count,
    (
        SELECT COUNT(*)
        FROM bundle_audience_selections AS selection
        JOIN bundle_audience_selection_members AS member
          ON member.selection_id = selection.id
        WHERE selection.campaign_id = context.campaign_id
    ) AS selection_member_count,
    (
        SELECT COUNT(*)
        FROM campaign_audience_tag_attributions AS attribution
        WHERE attribution.campaign_id = context.campaign_id
    ) AS smart_targeting_attribution_count,
    (SELECT COUNT(*) FROM current_processed)
        AS current_processed_record_count,
    COALESCE((
        SELECT SUM(CARDINALITY(processed.audience_ids))
        FROM current_processed AS processed
    ), 0) AS processed_audience_id_count,
    COALESCE((
        SELECT SUM(CARDINALITY(processed.audience_codes))
        FROM current_processed AS processed
    ), 0) AS processed_audience_code_count,
    (
        SELECT COUNT(*)
        FROM send_rows AS sent
        JOIN current_processed AS processed
          ON processed.id = sent.processed_campaign_id
        WHERE sent.platform = context.platform
    ) AS provider_send_row_count
FROM campaign_context AS context;


-- 5. Validate selection-ledger scope, uniqueness, ordering, and bundle reuse
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


-- 6. Validate the current processed-campaign checkpoint
--
-- For a completely prepared and sent campaign, all mismatch columns should be
-- false. The last-audience check is only expected to be false after the whole
-- audience has been processed.

WITH params AS (
    SELECT 955::bigint AS campaign_id
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


-- 7. Provider-send status, recipient-set consistency, and duplicates
--
-- For a fully executed campaign, missing_send_count, extra_send_count,
-- duplicate_tracking_id_count, and duplicate_phone_count should be zero.
-- Pending rows can be valid while provider status collection is unfinished.

WITH params AS (
    SELECT 955::bigint AS campaign_id
),
campaign_context AS (
    SELECT
        campaign.id AS campaign_id,
        LOWER(BTRIM(campaign.spec->>'platform')) AS platform
    FROM params
    JOIN campaigns AS campaign
      ON campaign.id = params.campaign_id
),
current_processed AS (
    SELECT processed.id
    FROM campaign_context AS context
    JOIN processed_campaigns AS processed
      ON processed.campaign_id = context.campaign_id
     AND processed.is_current
),
expected_phones AS (
    SELECT BTRIM(audience.phone_number) AS phone_number
    FROM campaign_context AS context
    JOIN bundle_audience_selections AS selection
      ON selection.campaign_id = context.campaign_id
    JOIN bundle_audience_selection_members AS member
      ON member.selection_id = selection.id
    LEFT JOIN audience_profiles AS audience
      ON audience.id = member.audience_id
),
all_sends AS (
    SELECT
        'sms'::text AS platform,
        processed_campaign_id,
        phone_number,
        tracking_id::text AS tracking_id,
        status::text AS status,
        parts_delivered
    FROM sent_sms

    UNION ALL

    SELECT
        'bale',
        processed_campaign_id,
        phone_number,
        tracking_id::text,
        status::text,
        parts_delivered
    FROM sent_bale_messages
    WHERE is_current

    UNION ALL

    SELECT
        'rubika',
        processed_campaign_id,
        phone_number,
        tracking_id::text,
        status::text,
        parts_delivered
    FROM sent_rubika_messages

    UNION ALL

    SELECT
        'splus',
        processed_campaign_id,
        phone_number,
        tracking_id::text,
        status::text,
        parts_delivered
    FROM sent_splus_messages
),
campaign_sends AS (
    SELECT sent.*
    FROM campaign_context AS context
    JOIN current_processed AS processed ON TRUE
    JOIN all_sends AS sent
      ON sent.processed_campaign_id = processed.id
     AND sent.platform = context.platform
),
actual_phones AS (
    SELECT BTRIM(phone_number) AS phone_number
    FROM campaign_sends
),
missing AS (
    SELECT phone_number FROM expected_phones
    EXCEPT ALL
    SELECT phone_number FROM actual_phones
),
extra AS (
    SELECT phone_number FROM actual_phones
    EXCEPT ALL
    SELECT phone_number FROM expected_phones
),
duplicate_tracking AS (
    SELECT tracking_id, COUNT(*) AS occurrences
    FROM campaign_sends
    GROUP BY tracking_id
    HAVING COUNT(*) > 1
),
duplicate_phones AS (
    SELECT BTRIM(phone_number) AS phone_number, COUNT(*) AS occurrences
    FROM campaign_sends
    GROUP BY BTRIM(phone_number)
    HAVING COUNT(*) > 1
)
SELECT
    (SELECT COUNT(*) FROM expected_phones) AS expected_phone_count,
    (SELECT COUNT(*) FROM campaign_sends) AS provider_send_count,
    (SELECT COUNT(*) FROM campaign_sends WHERE status = 'successful')
        AS successful_count,
    (SELECT COUNT(*) FROM campaign_sends WHERE status = 'pending')
        AS pending_count,
    (SELECT COUNT(*) FROM campaign_sends WHERE status = 'unsuccessful')
        AS unsuccessful_count,
    (SELECT COUNT(*) FROM campaign_sends
        WHERE tracking_id IS NULL OR BTRIM(tracking_id) = '')
        AS missing_tracking_id_count,
    (SELECT COUNT(*) FROM campaign_sends WHERE parts_delivered < 0)
        AS invalid_parts_delivered_count,
    (SELECT COUNT(*) FROM missing) AS missing_send_count,
    (SELECT COUNT(*) FROM extra) AS extra_send_count,
    COALESCE((
        SELECT SUM(occurrences - 1)
        FROM duplicate_tracking
    ), 0) AS duplicate_tracking_id_count,
    COALESCE((
        SELECT SUM(occurrences - 1)
        FROM duplicate_phones
    ), 0) AS duplicate_phone_count;
