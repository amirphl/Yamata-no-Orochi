-- Bundle-wide Smart Targeting audience-reduction funnel
--
-- Change only params.bundle_id. The query returns one row for every Smart
-- Targeting campaign in the bundle; campaign status is deliberately not a
-- filter. Current processed_campaigns.id order reconstructs which materialized
-- bundle allocations existed before each campaign. Allocation rows whose
-- historical order cannot be resolved are conservatively treated as previous.
--
-- Smart Targeting differences from the standard-targeting funnel:
--   * selected tags and their order come from campaign_selected_tags;
--   * score p33/p66 are calculated from the campaign's eligible tag union;
--   * SMS accepts only exact colors 'white' and 'pink'; other platforms have
--     no color predicate;
--   * bundle_audience_exclusions applies only to Test campaigns;
--   * Test execution uses the persisted satisfied-tag subsequence, while its
--     score bounds still use the complete selected-tag union.
--
-- Color columns use the scheduler's exact stored values. NULL, differently
-- cased values, and colors other than white/pink/black are counted as other.
-- `remaining_before_*` and `remaining_after_*` are current-profile inventory
-- within that campaign's selected-tag union; they do not apply phone, platform,
-- grade, or Test-exclusion predicates. `scheduler_eligible_*` applies all those
-- predicates and is therefore the useful capacity result. For SMS campaigns,
-- scheduler_eligible_black_count is expected to be zero.
--
-- This is a present-time reconstruction. Mutable profile tags, colors, phones,
-- and scores are not historical snapshots. Test `scheduler_eligible_count` is
-- the eligible union; it cannot prove that every satisfied tag can supply a
-- complete sample_size_per_tag. The persisted actual-selected columns remain
-- authoritative for what execution allocated.

WITH params AS (
    SELECT 371::bigint AS bundle_id
),
smart_campaigns AS MATERIALIZED (
    SELECT
        campaign.id AS campaign_id,
        campaign.bundle_id,
        campaign.status,
        campaign.phase,
        campaign.created_at,
        processed.id AS processed_campaign_id,
        campaign.num_audience AS requested_audience_count,
        campaign.sample_size_per_tag,
        NULLIF(BTRIM(campaign.spec->>'title'), '') AS campaign_title,
        LOWER(BTRIM(campaign.spec->>'platform')) AS platform,
        selected.selected_tag_ids,
        selected.selected_tag_titles,
        COALESCE(
            campaign.smart_targeting_test_satisfied_tag_ids,
            '{}'::integer[]
        ) AS test_satisfied_tag_ids,
        CASE
            WHEN CARDINALITY(grades.configured_audience_grades) = 0
                THEN ARRAY['A', 'B', 'C']::text[]
            ELSE grades.configured_audience_grades
        END AS configured_audience_grades
    FROM params
    JOIN campaigns AS campaign
      ON campaign.bundle_id = params.bundle_id
    LEFT JOIN processed_campaigns AS processed
      ON processed.campaign_id = campaign.id
     AND processed.is_current
    LEFT JOIN LATERAL (
        SELECT
            COALESCE(
                ARRAY_AGG(
                    chosen.tag_id
                    ORDER BY chosen.selection_order, chosen.id
                ),
                '{}'::integer[]
            ) AS selected_tag_ids,
            COALESCE(
                ARRAY_AGG(
                    COALESCE(
                        chosen.tag_display_title_snapshot,
                        live_tag.display_title,
                        live_tag.name
                    )
                    ORDER BY chosen.selection_order, chosen.id
                ),
                '{}'::text[]
            ) AS selected_tag_titles
        FROM campaign_selected_tags AS chosen
        LEFT JOIN tags AS live_tag
          ON live_tag.id = chosen.tag_id
        WHERE chosen.campaign_id = campaign.id
    ) AS selected ON TRUE
    CROSS JOIN LATERAL (
        SELECT ARRAY(
            SELECT UPPER(BTRIM(item.value))
            FROM JSONB_ARRAY_ELEMENTS_TEXT(
                CASE
                    WHEN JSONB_TYPEOF(campaign.spec->'audience_grades')
                            = 'array'
                        THEN campaign.spec->'audience_grades'
                    ELSE '[]'::jsonb
                END
            ) AS item(value)
            WHERE UPPER(BTRIM(item.value)) IN ('A', 'B', 'C')
            ORDER BY CASE UPPER(BTRIM(item.value))
                WHEN 'A' THEN 1
                WHEN 'B' THEN 2
                WHEN 'C' THEN 3
            END
        )::text[] AS configured_audience_grades
    ) AS grades
    WHERE CASE
        WHEN LOWER(BTRIM(COALESCE(
                campaign.spec->>'audience_targeting_method', ''
            ))) IN ('standard', 'smart_targeting', 'excel')
            THEN LOWER(BTRIM(
                campaign.spec->>'audience_targeting_method'
            ))
        WHEN EXISTS (
            SELECT 1
            FROM campaign_selected_tags AS legacy_selection
            WHERE legacy_selection.campaign_id = campaign.id
        ) THEN 'smart_targeting'
        WHEN NULLIF(BTRIM(
                campaign.spec->>'target_audience_excel_file_uuid'
            ), '') IS NOT NULL
            THEN 'excel'
        ELSE 'standard'
    END = 'smart_targeting'
),
ordered_campaigns AS MATERIALIZED (
    SELECT
        campaign.*,
        ROW_NUMBER() OVER (
            ORDER BY
                campaign.processed_campaign_id ASC NULLS LAST,
                campaign.campaign_id ASC
        ) AS campaign_position,
        'A' = ANY(campaign.configured_audience_grades) AS has_a,
        'B' = ANY(campaign.configured_audience_grades) AS has_b,
        'C' = ANY(campaign.configured_audience_grades) AS has_c,
        campaign.configured_audience_grades
            @> ARRAY['A', 'B', 'C']::text[] AS all_score_classes,
        CASE
            WHEN campaign.phase = 'test'::campaign_phase
                THEN campaign.test_satisfied_tag_ids
            ELSE campaign.selected_tag_ids
        END AS execution_tag_ids
    FROM smart_campaigns AS campaign
),
profile_flags AS MATERIALIZED (
    SELECT
        campaign.campaign_id,
        profile.id AS audience_id,
        profile.color,
        profile.normalized_score,
        profile.phone_number IS NOT NULL
          AND BTRIM(profile.phone_number) <> '' AS has_usable_phone,
        campaign.platform <> 'sms'
          OR profile.color IN ('white', 'pink') AS platform_color_matches,
        profile.tags && campaign.execution_tag_ids
            AS execution_tag_matches,
        campaign.phase = 'test'::campaign_phase
          AND EXISTS (
              SELECT 1
              FROM bundle_audience_exclusions AS excluded
              WHERE excluded.bundle_id = campaign.bundle_id
                AND excluded.audience_id = profile.id
          ) AS excluded_from_test,
        EXISTS (
            SELECT 1
            FROM bundle_audience_selection_members AS used
            JOIN bundle_audience_selections AS allocation
              ON allocation.id = used.selection_id
            LEFT JOIN processed_campaigns AS allocation_processed
              ON allocation_processed.campaign_id = allocation.campaign_id
             AND allocation_processed.is_current
            WHERE used.bundle_id = campaign.bundle_id
              AND used.audience_id = profile.id
              AND allocation.campaign_id IS DISTINCT FROM campaign.campaign_id
              AND (
                  allocation.campaign_id IS NULL
                  OR campaign.processed_campaign_id IS NULL
                  OR allocation_processed.id IS NULL
                  OR allocation_processed.id < campaign.processed_campaign_id
              )
        ) AS used_before_campaign,
        EXISTS (
            SELECT 1
            FROM bundle_audience_selection_members AS used
            JOIN bundle_audience_selections AS allocation
              ON allocation.id = used.selection_id
            WHERE used.bundle_id = campaign.bundle_id
              AND used.audience_id = profile.id
              AND allocation.campaign_id = campaign.campaign_id
        ) AS used_by_campaign
    FROM ordered_campaigns AS campaign
    JOIN audience_profiles AS profile
      ON profile.tags && campaign.selected_tag_ids
),
candidate_population AS MATERIALIZED (
    -- This is the population from which Smart Targeting calculates p33/p66.
    -- For Test campaigns it intentionally uses all selected tags, not only the
    -- persisted satisfied-tag execution subsequence.
    SELECT flags.*
    FROM profile_flags AS flags
    WHERE flags.has_usable_phone
      AND flags.platform_color_matches
      AND NOT flags.used_before_campaign
      AND NOT flags.excluded_from_test
),
score_bounds AS MATERIALIZED (
    SELECT
        percentiles.campaign_id,
        percentiles.values[1] AS p33,
        percentiles.values[2] AS p66
    FROM (
        SELECT
            population.campaign_id,
            PERCENTILE_DISC(
                ARRAY[0.33, 0.66]::double precision[]
            ) WITHIN GROUP (
                ORDER BY population.normalized_score
            ) AS values
        FROM candidate_population AS population
        WHERE population.normalized_score IS NOT NULL
        GROUP BY population.campaign_id
    ) AS percentiles
),
evaluated_profiles AS MATERIALIZED (
    SELECT
        flags.*,
        bounds.p33,
        bounds.p66,
        CASE
            WHEN campaign.all_score_classes THEN TRUE
            WHEN bounds.p33 IS NULL OR bounds.p66 IS NULL THEN FALSE
            WHEN flags.normalized_score IS NULL THEN FALSE
            WHEN campaign.has_a AND campaign.has_b
                THEN flags.normalized_score > bounds.p33
            WHEN campaign.has_b AND campaign.has_c
                THEN flags.normalized_score <= bounds.p66
            WHEN campaign.has_a AND campaign.has_c
                THEN flags.normalized_score <= bounds.p33
                  OR flags.normalized_score > bounds.p66
            WHEN campaign.has_a
                THEN flags.normalized_score > bounds.p66
            WHEN campaign.has_b
                THEN flags.normalized_score > bounds.p33
                 AND flags.normalized_score <= bounds.p66
            WHEN campaign.has_c
                THEN flags.normalized_score <= bounds.p33
            ELSE FALSE
        END AS grade_matches
    FROM profile_flags AS flags
    JOIN ordered_campaigns AS campaign
      ON campaign.campaign_id = flags.campaign_id
    LEFT JOIN score_bounds AS bounds
      ON bounds.campaign_id = flags.campaign_id
),
funnel_counts AS MATERIALIZED (
    SELECT
        campaign.campaign_id,
        COUNT(profile.audience_id) AS selected_tag_union_count,
        COUNT(profile.audience_id) FILTER (
            WHERE profile.color = 'white'
        ) AS tag_union_white_count,
        COUNT(profile.audience_id) FILTER (
            WHERE profile.color = 'pink'
        ) AS tag_union_pink_count,
        COUNT(profile.audience_id) FILTER (
            WHERE profile.color = 'black'
        ) AS tag_union_black_count,
        COUNT(profile.audience_id) FILTER (
            WHERE profile.color IS NULL
               OR profile.color NOT IN ('white', 'pink', 'black')
        ) AS tag_union_other_color_count,

        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
        ) AS remaining_before_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND profile.color = 'white'
        ) AS remaining_before_white_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND profile.color = 'pink'
        ) AS remaining_before_pink_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND profile.color = 'black'
        ) AS remaining_before_black_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND (
                  profile.color IS NULL
                  OR profile.color NOT IN ('white', 'pink', 'black')
              )
        ) AS remaining_before_other_color_count,

        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND profile.has_usable_phone
        ) AS after_usable_phone_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND profile.has_usable_phone
              AND profile.platform_color_matches
        ) AS after_platform_color_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND profile.has_usable_phone
              AND profile.platform_color_matches
              AND NOT profile.excluded_from_test
        ) AS before_grade_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND profile.has_usable_phone
              AND profile.platform_color_matches
              AND NOT profile.excluded_from_test
              AND profile.execution_tag_matches
              AND profile.grade_matches
        ) AS scheduler_eligible_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND profile.has_usable_phone
              AND profile.platform_color_matches
              AND NOT profile.excluded_from_test
              AND profile.execution_tag_matches
              AND profile.grade_matches
              AND profile.color = 'white'
        ) AS scheduler_eligible_white_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND profile.has_usable_phone
              AND profile.platform_color_matches
              AND NOT profile.excluded_from_test
              AND profile.execution_tag_matches
              AND profile.grade_matches
              AND profile.color = 'pink'
        ) AS scheduler_eligible_pink_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND profile.has_usable_phone
              AND profile.platform_color_matches
              AND NOT profile.excluded_from_test
              AND profile.execution_tag_matches
              AND profile.grade_matches
              AND profile.color = 'black'
        ) AS scheduler_eligible_black_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND profile.has_usable_phone
              AND profile.platform_color_matches
              AND NOT profile.excluded_from_test
              AND profile.execution_tag_matches
              AND profile.grade_matches
              AND (
                  profile.color IS NULL
                  OR profile.color NOT IN ('white', 'pink', 'black')
              )
        ) AS scheduler_eligible_other_color_count,

        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND NOT profile.used_by_campaign
        ) AS remaining_after_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND NOT profile.used_by_campaign
              AND profile.color = 'white'
        ) AS remaining_after_white_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND NOT profile.used_by_campaign
              AND profile.color = 'pink'
        ) AS remaining_after_pink_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND NOT profile.used_by_campaign
              AND profile.color = 'black'
        ) AS remaining_after_black_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND NOT profile.used_by_campaign
              AND (
                  profile.color IS NULL
                  OR profile.color NOT IN ('white', 'pink', 'black')
              )
        ) AS remaining_after_other_color_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND NOT profile.used_by_campaign
              AND profile.has_usable_phone
              AND profile.platform_color_matches
              AND NOT profile.excluded_from_test
              AND profile.execution_tag_matches
              AND profile.grade_matches
        ) AS scheduler_eligible_after_campaign_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND NOT profile.used_by_campaign
              AND profile.has_usable_phone
              AND profile.platform_color_matches
              AND NOT profile.excluded_from_test
              AND profile.execution_tag_matches
              AND profile.grade_matches
              AND profile.color = 'white'
        ) AS scheduler_eligible_after_campaign_white_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND NOT profile.used_by_campaign
              AND profile.has_usable_phone
              AND profile.platform_color_matches
              AND NOT profile.excluded_from_test
              AND profile.execution_tag_matches
              AND profile.grade_matches
              AND profile.color = 'pink'
        ) AS scheduler_eligible_after_campaign_pink_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND NOT profile.used_by_campaign
              AND profile.has_usable_phone
              AND profile.platform_color_matches
              AND NOT profile.excluded_from_test
              AND profile.execution_tag_matches
              AND profile.grade_matches
              AND profile.color = 'black'
        ) AS scheduler_eligible_after_campaign_black_count,
        COUNT(profile.audience_id) FILTER (
            WHERE NOT profile.used_before_campaign
              AND NOT profile.used_by_campaign
              AND profile.has_usable_phone
              AND profile.platform_color_matches
              AND NOT profile.excluded_from_test
              AND profile.execution_tag_matches
              AND profile.grade_matches
              AND (
                  profile.color IS NULL
                  OR profile.color NOT IN ('white', 'pink', 'black')
              )
        ) AS scheduler_eligible_after_campaign_other_color_count
    FROM ordered_campaigns AS campaign
    LEFT JOIN evaluated_profiles AS profile
      ON profile.campaign_id = campaign.campaign_id
    GROUP BY campaign.campaign_id
),
actual_allocations AS MATERIALIZED (
    SELECT
        campaign.campaign_id,
        COUNT(member.audience_id) AS actual_selected_count,
        COUNT(member.audience_id) FILTER (
            WHERE profile.color = 'white'
        ) AS actual_selected_white_count,
        COUNT(member.audience_id) FILTER (
            WHERE profile.color = 'pink'
        ) AS actual_selected_pink_count,
        COUNT(member.audience_id) FILTER (
            WHERE profile.color = 'black'
        ) AS actual_selected_black_count,
        COUNT(member.audience_id) FILTER (
            WHERE profile.id IS NOT NULL
              AND (
                  profile.color IS NULL
                  OR profile.color NOT IN ('white', 'pink', 'black')
              )
        ) AS actual_selected_other_color_count,
        COUNT(member.audience_id) FILTER (
            WHERE profile.id IS NULL
        ) AS selected_missing_profile_count
    FROM ordered_campaigns AS campaign
    LEFT JOIN bundle_audience_selections AS allocation
      ON allocation.campaign_id = campaign.campaign_id
    LEFT JOIN bundle_audience_selection_members AS member
      ON member.selection_id = allocation.id
    LEFT JOIN audience_profiles AS profile
      ON profile.id = member.audience_id
    GROUP BY campaign.campaign_id
)
SELECT
    campaign.campaign_position,
    campaign.processed_campaign_id,
    campaign.campaign_id,
    campaign.campaign_title,
    campaign.status,
    campaign.phase,
    campaign.platform,
    campaign.created_at,
    campaign.requested_audience_count,
    campaign.sample_size_per_tag,
    campaign.selected_tag_ids,
    campaign.selected_tag_titles,
    CARDINALITY(campaign.selected_tag_ids) AS selected_tag_count,
    campaign.test_satisfied_tag_ids,
    campaign.execution_tag_ids,
    campaign.configured_audience_grades,
    CASE WHEN campaign.all_score_classes THEN NULL ELSE bounds.p33 END AS p33,
    CASE WHEN campaign.all_score_classes THEN NULL ELSE bounds.p66 END AS p66,
    campaign.all_score_classes
      OR (bounds.p33 IS NOT NULL AND bounds.p66 IS NOT NULL)
        AS score_rule_is_computable,

    funnel.selected_tag_union_count,
    funnel.tag_union_white_count,
    funnel.tag_union_pink_count,
    funnel.tag_union_black_count,
    funnel.tag_union_other_color_count,

    funnel.selected_tag_union_count - funnel.remaining_before_count
        AS removed_by_previous_allocations_count,
    funnel.remaining_before_count,
    funnel.remaining_before_white_count,
    funnel.remaining_before_pink_count,
    funnel.remaining_before_black_count,
    funnel.remaining_before_other_color_count,

    funnel.after_usable_phone_count,
    funnel.remaining_before_count - funnel.after_usable_phone_count
        AS removed_without_usable_phone_count,
    funnel.after_platform_color_count,
    funnel.after_usable_phone_count - funnel.after_platform_color_count
        AS removed_by_platform_color_count,
    funnel.before_grade_count,
    funnel.after_platform_color_count - funnel.before_grade_count
        AS removed_by_test_exclusion_count,
    funnel.scheduler_eligible_count,
    funnel.before_grade_count - funnel.scheduler_eligible_count
        AS removed_by_execution_tags_or_grade_count,
    funnel.scheduler_eligible_white_count,
    funnel.scheduler_eligible_pink_count,
    funnel.scheduler_eligible_black_count,
    funnel.scheduler_eligible_other_color_count,
    GREATEST(
        COALESCE(campaign.requested_audience_count, 0)::bigint
            - funnel.scheduler_eligible_count,
        0
    ) AS requested_shortfall,
    funnel.scheduler_eligible_count
        >= COALESCE(campaign.requested_audience_count, 0)::bigint
        AS enough_union_capacity,

    actual.actual_selected_count,
    actual.actual_selected_white_count,
    actual.actual_selected_pink_count,
    actual.actual_selected_black_count,
    actual.actual_selected_other_color_count,
    actual.selected_missing_profile_count,

    funnel.remaining_after_count,
    funnel.remaining_after_white_count,
    funnel.remaining_after_pink_count,
    funnel.remaining_after_black_count,
    funnel.remaining_after_other_color_count,
    funnel.scheduler_eligible_after_campaign_count,
    funnel.scheduler_eligible_after_campaign_white_count,
    funnel.scheduler_eligible_after_campaign_pink_count,
    funnel.scheduler_eligible_after_campaign_black_count,
    funnel.scheduler_eligible_after_campaign_other_color_count
FROM ordered_campaigns AS campaign
JOIN funnel_counts AS funnel
  ON funnel.campaign_id = campaign.campaign_id
JOIN actual_allocations AS actual
  ON actual.campaign_id = campaign.campaign_id
LEFT JOIN score_bounds AS bounds
  ON bounds.campaign_id = campaign.campaign_id
ORDER BY
    campaign.processed_campaign_id ASC NULLS LAST,
    campaign.campaign_id ASC;
