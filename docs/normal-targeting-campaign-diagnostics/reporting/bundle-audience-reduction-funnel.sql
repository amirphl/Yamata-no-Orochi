-- Bundle-wide standard-targeting audience-reduction funnel
--
-- This query returns one row for every standard-targeting campaign in bundle
-- 318. Execution order is the current processed_campaigns.id ascending.
-- "Previous campaigns" therefore means campaigns with a lower current
-- processed-campaign ID that have written audience IDs to the immutable bundle
-- selection ledger. Those previous campaigns may use standard or Smart
-- Targeting because either targeting method consumes the same bundle capacity.
-- Excel campaigns do not normally write to this ledger. Campaigns without a
-- current processed row are shown last (campaign ID breaks ties); for them,
-- every processed bundle allocation is treated as previous.
--
-- Grade boundaries intentionally mirror normal-targeting execution:
--   A       => score > p66
--   B       => p33 < score <= p66
--   C       => score <= p33
--   A or B  => score > p33
--   A or C  => score <= p33 OR score > p66
--   B or C  => score <= p66
--   A/B/C   => no score filter, so it includes unscored audiences
--
-- Boundary comparisons are disjoint: p33 belongs to C, p66 belongs to B, and
-- A starts strictly above p66. Combined columns apply the scheduler's union
-- predicate directly and must not be calculated by adding individual grades.
--
-- The counts use current audience_profiles tags, colors, and scores. They are
-- a present-time capacity diagnostic, not a historical snapshot. Change only
-- params.bundle_id to inspect another bundle.

WITH params AS (
    SELECT 318::bigint AS bundle_id
),
standard_campaigns AS (
    SELECT
        campaign.id AS campaign_id,
        campaign.bundle_id,
        campaign.status,
        campaign.phase,
        campaign.created_at,
        processed.id AS processed_campaign_id,
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
                    FROM JSONB_ARRAY_ELEMENTS_TEXT(
                        campaign.spec->'audience_grades'
                    ) AS item(value)
                    WHERE BTRIM(item.value) <> ''
                )::text[]
            ELSE ARRAY['A', 'B', 'C']::text[]
        END AS configured_audience_grades
    FROM params
    JOIN campaigns AS campaign
      ON campaign.bundle_id = params.bundle_id
    LEFT JOIN processed_campaigns AS processed
      ON processed.campaign_id = campaign.id
     AND processed.is_current
    WHERE CASE
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
),
ordered_campaigns AS (
    SELECT
        campaign.*,
        ROW_NUMBER() OVER (
            ORDER BY
                campaign.processed_campaign_id ASC NULLS LAST,
                campaign.campaign_id ASC
        )
            AS campaign_position
    FROM standard_campaigns AS campaign
),
campaign_funnel AS (
    SELECT
        campaign.*,
        bounds.p33,
        bounds.p66,
        counts.matching_tags_count,
        counts.available_after_previous_count,
        counts.available_grade_a_count,
        counts.available_grade_b_count,
        counts.available_grade_c_count,
        counts.available_grade_ab_count,
        counts.available_grade_ac_count,
        counts.available_grade_bc_count,
        counts.available_grade_abc_count,
        counts.available_unscored_count,
        counts.available_not_black_count,
        counts.available_not_black_grade_a_count,
        counts.available_not_black_grade_b_count,
        counts.available_not_black_grade_c_count,
        counts.available_not_black_grade_ab_count,
        counts.available_not_black_grade_ac_count,
        counts.available_not_black_grade_bc_count,
        counts.available_not_black_grade_abc_count,
        counts.available_not_black_unscored_count
    FROM ordered_campaigns AS campaign
    LEFT JOIN LATERAL (
        -- This ordering mirrors GORM First in FetchPercentiles. If several
        -- matching rows have different bounds, query 1 reports that ambiguity.
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
    CROSS JOIN LATERAL (
        SELECT
            COUNT(*) AS matching_tags_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
            ) AS available_after_previous_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.normalized_score > bounds.p66
            ) AS available_grade_a_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.normalized_score > bounds.p33
                  AND audience.normalized_score <= bounds.p66
            ) AS available_grade_b_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.normalized_score <= bounds.p33
            ) AS available_grade_c_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.normalized_score > bounds.p33
            ) AS available_grade_ab_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND (
                      audience.normalized_score <= bounds.p33
                      OR audience.normalized_score > bounds.p66
                  )
            ) AS available_grade_ac_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.normalized_score <= bounds.p66
            ) AS available_grade_bc_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
            ) AS available_grade_abc_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.normalized_score IS NULL
            ) AS available_unscored_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.is_not_black
            ) AS available_not_black_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.is_not_black
                  AND audience.normalized_score > bounds.p66
            ) AS available_not_black_grade_a_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.is_not_black
                  AND audience.normalized_score > bounds.p33
                  AND audience.normalized_score <= bounds.p66
            ) AS available_not_black_grade_b_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.is_not_black
                  AND audience.normalized_score <= bounds.p33
            ) AS available_not_black_grade_c_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.is_not_black
                  AND audience.normalized_score > bounds.p33
            ) AS available_not_black_grade_ab_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.is_not_black
                  AND (
                      audience.normalized_score <= bounds.p33
                      OR audience.normalized_score > bounds.p66
                  )
            ) AS available_not_black_grade_ac_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.is_not_black
                  AND audience.normalized_score <= bounds.p66
            ) AS available_not_black_grade_bc_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.is_not_black
            ) AS available_not_black_grade_abc_count,
            COUNT(*) FILTER (
                WHERE audience.available_after_previous
                  AND audience.is_not_black
                  AND audience.normalized_score IS NULL
            ) AS available_not_black_unscored_count
        FROM (
            SELECT
                profile.normalized_score,
                LOWER(BTRIM(profile.color)) <> 'black' AS is_not_black,
                NOT EXISTS (
                    SELECT 1
                    FROM bundle_audience_selection_members AS previous_member
                    JOIN bundle_audience_selections AS previous_selection
                      ON previous_selection.id = previous_member.selection_id
                    JOIN campaigns AS previous_campaign
                      ON previous_campaign.id = previous_selection.campaign_id
                    JOIN processed_campaigns AS previous_processed
                      ON previous_processed.campaign_id = previous_campaign.id
                     AND previous_processed.is_current
                    WHERE previous_member.bundle_id = campaign.bundle_id
                      AND previous_member.audience_id = profile.id
                      AND previous_campaign.bundle_id = campaign.bundle_id
                      AND (
                          previous_processed.id
                              < campaign.processed_campaign_id
                          OR campaign.processed_campaign_id IS NULL
                      )
                ) AS available_after_previous
            FROM audience_profiles AS profile
            WHERE profile.tags && campaign.tag_ids
        ) AS audience
    ) AS counts
)
SELECT
    funnel.campaign_position,
    funnel.processed_campaign_id,
    funnel.campaign_id,
    funnel.campaign_title,
    funnel.status,
    funnel.phase,
    funnel.platform,
    funnel.created_at,
    funnel.requested_audience_count,
    funnel.tag_ids,
    CARDINALITY(funnel.tag_ids) AS configured_tag_count,
    funnel.configured_audience_grades,
    funnel.p33,
    funnel.p66,
    funnel.p33 IS NOT NULL AND funnel.p66 IS NOT NULL
        AS score_bounds_found,

    -- Main reduction flow.
    funnel.matching_tags_count,
    funnel.matching_tags_count - funnel.available_after_previous_count
        AS removed_by_previous_campaigns_count,
    funnel.available_after_previous_count,
    ROUND(
        100.0 * funnel.available_after_previous_count
        / NULLIF(funnel.matching_tags_count, 0),
        2
    ) AS available_after_previous_percent,

    -- Grade alternatives after tag matching and previous-campaign exclusion.
    funnel.available_grade_a_count,
    funnel.available_grade_b_count,
    funnel.available_grade_c_count,
    funnel.available_grade_ab_count,
    funnel.available_grade_ac_count,
    funnel.available_grade_bc_count,
    funnel.available_grade_abc_count,
    funnel.available_unscored_count,

    -- The same alternatives after also excluding black audiences.
    funnel.available_after_previous_count
        - funnel.available_not_black_count AS removed_black_count,
    funnel.available_not_black_count,
    funnel.available_not_black_grade_a_count,
    funnel.available_not_black_grade_b_count,
    funnel.available_not_black_grade_c_count,
    funnel.available_not_black_grade_ab_count,
    funnel.available_not_black_grade_ac_count,
    funnel.available_not_black_grade_bc_count,
    funnel.available_not_black_grade_abc_count,
    funnel.available_not_black_unscored_count,

    -- Useful reconciliation/capacity indicators.
    GREATEST(
        COALESCE(funnel.requested_audience_count, 0)::numeric
            - funnel.available_not_black_count,
        0
    ) AS requested_shortfall_after_not_black,
    COALESCE((
        SELECT COUNT(*)
        FROM bundle_audience_selections AS selection
        JOIN bundle_audience_selection_members AS member
          ON member.selection_id = selection.id
        WHERE selection.campaign_id = funnel.campaign_id
    ), 0) AS actual_selected_count
FROM campaign_funnel AS funnel
ORDER BY
    funnel.processed_campaign_id ASC NULLS LAST,
    funnel.campaign_id ASC;
