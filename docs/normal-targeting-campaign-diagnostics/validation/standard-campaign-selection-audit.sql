-- Audit selected audiences for every running/executed standard campaign
--
-- One row is returned per campaign. is_valid is true only when failed_checks
-- is empty. Each audience-level rule also exposes the exact offending IDs.
-- The audit follows current processed_campaigns.id order and evaluates current
-- audience_profiles data; it cannot reconstruct tags, colors, scores, or
-- percentile bounds as they existed when an old campaign was prepared.
--
-- A selection smaller than requested_audience_count is not automatically an
-- error: the eligible audience pool may have been exhausted. Selecting more
-- than requested is an error. SMS requires exact colors white or pink, and
-- every platform requires a nonblank phone number. Grade boundaries are the
-- same disjoint boundaries used by Smart Targeting.

WITH params AS (
    SELECT 318::bigint AS bundle_id
),
target_campaigns AS (
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
        END AS audience_grades,
        processed.id AS processed_campaign_id,
        processed.bundle_audience_selection_id
            AS processed_selection_id,
        processed.audience_ids AS processed_audience_ids
    FROM params
    JOIN campaigns AS campaign
      ON campaign.bundle_id = params.bundle_id
    LEFT JOIN processed_campaigns AS processed
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
),
campaign_rules AS (
    SELECT
        campaign.*,
        'A' = ANY(campaign.audience_grades) AS has_a,
        'B' = ANY(campaign.audience_grades) AS has_b,
        'C' = ANY(campaign.audience_grades) AS has_c,
        NOT (
            CARDINALITY(campaign.audience_grades) = 0
            OR campaign.audience_grades
                @> ARRAY['A', 'B', 'C']::text[]
        ) AS grade_filter_requested
    FROM target_campaigns AS campaign
),
campaign_context AS (
    SELECT
        rules.*,
        bounds.p33,
        bounds.p66,
        rules.grade_filter_requested AS grade_filter_required,
        ROW_NUMBER() OVER (
            ORDER BY
                rules.processed_campaign_id ASC NULLS LAST,
                rules.campaign_id ASC
        ) AS execution_position
    FROM campaign_rules AS rules
    LEFT JOIN LATERAL (
        SELECT stats.p33, stats.p66
        FROM src_layer_all_stats AS stats
        WHERE stats.p33 IS NOT NULL
          AND stats.p66 IS NOT NULL
          AND (
              rules.level1 IS NULL
              OR stats.layer1_category = rules.level1
          )
          AND (
              CARDINALITY(rules.level2s) = 0
              OR stats.layer2_category = ANY(rules.level2s)
          )
          AND (
              CARDINALITY(rules.level3s) = 0
              OR stats.layer3_category = ANY(rules.level3s)
          )
        ORDER BY stats.layer1_category ASC
        LIMIT 1
    ) AS bounds ON TRUE
),
selection_records AS (
    SELECT
        campaign.campaign_id,
        COUNT(selection.id) AS selection_record_count,
        COALESCE(SUM(selection.audience_count), 0)
            AS selection_recorded_count,
        ARRAY_AGG(selection.id ORDER BY selection.id)
            FILTER (WHERE selection.id IS NOT NULL) AS selection_ids,
        COUNT(selection.id) FILTER (
            WHERE selection.bundle_id IS DISTINCT FROM campaign.bundle_id
        ) AS wrong_selection_bundle_count
    FROM campaign_context AS campaign
    LEFT JOIN bundle_audience_selections AS selection
      ON selection.campaign_id = campaign.campaign_id
    GROUP BY campaign.campaign_id
),
selection_member_structure AS (
    SELECT
        campaign.campaign_id,
        COUNT(member.id) AS selected_member_count,
        COUNT(member.id) - COUNT(DISTINCT member.audience_id)
            AS duplicate_audience_occurrence_count,
        COUNT(member.id)
            - COUNT(DISTINCT (member.selection_id, member.selection_order))
            AS duplicate_selection_order_count,
        COUNT(member.id) FILTER (
            WHERE member.bundle_id IS DISTINCT FROM campaign.bundle_id
        ) AS wrong_member_bundle_count
    FROM campaign_context AS campaign
    LEFT JOIN bundle_audience_selections AS selection
      ON selection.campaign_id = campaign.campaign_id
    LEFT JOIN bundle_audience_selection_members AS member
      ON member.selection_id = selection.id
    GROUP BY campaign.campaign_id
),
noncontiguous_selections AS (
    SELECT
        per_selection.campaign_id,
        COUNT(*) FILTER (
            WHERE per_selection.member_count > 0
              AND (
                  per_selection.minimum_order <> 0
                  OR per_selection.maximum_order
                      <> per_selection.member_count - 1
              )
        ) AS noncontiguous_selection_count
    FROM (
        SELECT
            selection.campaign_id,
            selection.id AS selection_id,
            COUNT(member.id) AS member_count,
            MIN(member.selection_order) AS minimum_order,
            MAX(member.selection_order) AS maximum_order
        FROM bundle_audience_selections AS selection
        JOIN campaign_context AS campaign
          ON campaign.campaign_id = selection.campaign_id
        LEFT JOIN bundle_audience_selection_members AS member
          ON member.selection_id = selection.id
        GROUP BY selection.campaign_id, selection.id
    ) AS per_selection
    GROUP BY per_selection.campaign_id
),
selected_audiences AS (
    SELECT
        campaign.campaign_id,
        campaign.bundle_id,
        campaign.platform,
        campaign.tag_ids,
        campaign.grade_filter_required,
        campaign.has_a,
        campaign.has_b,
        campaign.has_c,
        campaign.p33,
        campaign.p66,
        campaign.processed_campaign_id,
        selection.id AS selection_id,
        member.id AS member_id,
        member.audience_id,
        member.selection_order,
        profile.id AS profile_id,
        profile.tags AS profile_tags,
        profile.color,
        profile.phone_number,
        profile.normalized_score,
        COUNT(member.id) OVER (
            PARTITION BY campaign.campaign_id, member.audience_id
        ) AS audience_occurrences_within_campaign
    FROM campaign_context AS campaign
    JOIN bundle_audience_selections AS selection
      ON selection.campaign_id = campaign.campaign_id
    JOIN bundle_audience_selection_members AS member
      ON member.selection_id = selection.id
    LEFT JOIN audience_profiles AS profile
      ON profile.id = member.audience_id
),
evaluated_audiences AS (
    SELECT
        selected.*,
        selected.profile_id IS NOT NULL
          AND COALESCE(
              selected.profile_tags && selected.tag_ids,
              FALSE
          ) AS tags_match,
        CASE
            WHEN selected.profile_id IS NULL THEN FALSE
            WHEN NOT selected.grade_filter_required THEN TRUE
            WHEN selected.p33 IS NULL OR selected.p66 IS NULL THEN FALSE
            WHEN selected.normalized_score IS NULL THEN FALSE
            WHEN selected.has_a AND selected.has_b
                THEN selected.normalized_score > selected.p33
            WHEN selected.has_b AND selected.has_c
                THEN selected.normalized_score <= selected.p66
            WHEN selected.has_a AND selected.has_c
                THEN selected.normalized_score <= selected.p33
                  OR selected.normalized_score > selected.p66
            WHEN selected.has_a
                THEN selected.normalized_score > selected.p66
            WHEN selected.has_b
                THEN selected.normalized_score > selected.p33
                 AND selected.normalized_score <= selected.p66
            WHEN selected.has_c
                THEN selected.normalized_score <= selected.p33
            ELSE TRUE
        END AS grade_matches,
        selected.profile_id IS NOT NULL
          AND (
              selected.platform <> 'sms'
              OR COALESCE(selected.color IN ('white', 'pink'), FALSE)
          ) AS platform_color_matches,
        selected.profile_id IS NOT NULL
          AND selected.phone_number IS NOT NULL
          AND BTRIM(selected.phone_number) <> '' AS has_usable_phone,
        EXISTS (
            SELECT 1
            FROM bundle_audience_selection_members AS previous_member
            JOIN bundle_audience_selections AS previous_selection
              ON previous_selection.id = previous_member.selection_id
            JOIN campaigns AS previous_campaign
              ON previous_campaign.id = previous_selection.campaign_id
            JOIN processed_campaigns AS previous_processed
              ON previous_processed.campaign_id = previous_campaign.id
             AND previous_processed.is_current
            WHERE previous_member.bundle_id = selected.bundle_id
              AND previous_member.audience_id = selected.audience_id
              AND previous_campaign.bundle_id = selected.bundle_id
              AND previous_campaign.id <> selected.campaign_id
              AND (
                  previous_processed.id < selected.processed_campaign_id
                  OR selected.processed_campaign_id IS NULL
              )
        ) AS selected_by_previous_campaign
    FROM selected_audiences AS selected
),
audience_audit AS (
    SELECT
        evaluated.campaign_id,
        ARRAY_AGG(evaluated.audience_id ORDER BY
            evaluated.selection_id, evaluated.selection_order
        ) AS selected_audience_ids_in_order,
        COALESCE(ARRAY_AGG(DISTINCT evaluated.audience_id ORDER BY
            evaluated.audience_id
        ) FILTER (
            WHERE evaluated.audience_occurrences_within_campaign > 1
        ), '{}'::bigint[]) AS duplicate_within_campaign_ids,
        COALESCE(ARRAY_AGG(DISTINCT evaluated.audience_id ORDER BY
            evaluated.audience_id
        ) FILTER (
            WHERE evaluated.selected_by_previous_campaign
        ), '{}'::bigint[]) AS duplicate_from_previous_campaign_ids,
        COALESCE(ARRAY_AGG(DISTINCT evaluated.audience_id ORDER BY
            evaluated.audience_id
        ) FILTER (
            WHERE evaluated.profile_id IS NULL
        ), '{}'::bigint[]) AS missing_profile_ids,
        COALESCE(ARRAY_AGG(DISTINCT evaluated.audience_id ORDER BY
            evaluated.audience_id
        ) FILTER (
            WHERE evaluated.profile_id IS NOT NULL
              AND NOT evaluated.tags_match
        ), '{}'::bigint[]) AS tag_mismatch_ids,
        COALESCE(ARRAY_AGG(DISTINCT evaluated.audience_id ORDER BY
            evaluated.audience_id
        ) FILTER (
            WHERE evaluated.profile_id IS NOT NULL
              AND NOT evaluated.grade_matches
        ), '{}'::bigint[]) AS grade_mismatch_ids,
        COALESCE(ARRAY_AGG(DISTINCT evaluated.audience_id ORDER BY
            evaluated.audience_id
        ) FILTER (
            WHERE evaluated.profile_id IS NOT NULL
              AND NOT evaluated.platform_color_matches
        ), '{}'::bigint[]) AS platform_color_mismatch_ids,
        COALESCE(ARRAY_AGG(DISTINCT evaluated.audience_id ORDER BY
            evaluated.audience_id
        ) FILTER (
            WHERE evaluated.profile_id IS NOT NULL
              AND NOT evaluated.has_usable_phone
        ), '{}'::bigint[]) AS missing_phone_ids
    FROM evaluated_audiences AS evaluated
    GROUP BY evaluated.campaign_id
),
processed_array_audit AS (
    SELECT
        campaign.campaign_id,
        COALESCE((
            SELECT ARRAY_AGG(duplicate.audience_id ORDER BY
                duplicate.audience_id
            )
            FROM (
                SELECT processed_id.audience_id
                FROM UNNEST(COALESCE(
                    campaign.processed_audience_ids,
                    '{}'::bigint[]
                )) AS processed_id(audience_id)
                GROUP BY processed_id.audience_id
                HAVING COUNT(*) > 1
            ) AS duplicate
        ), '{}'::bigint[]) AS duplicate_processed_audience_ids,
        COALESCE((
            SELECT ARRAY_AGG(DISTINCT processed_id.audience_id ORDER BY
                processed_id.audience_id
            )
            FROM UNNEST(COALESCE(
                campaign.processed_audience_ids,
                '{}'::bigint[]
            )) AS processed_id(audience_id)
            WHERE NOT EXISTS (
                SELECT 1
                FROM bundle_audience_selections AS selection
                JOIN bundle_audience_selection_members AS member
                  ON member.selection_id = selection.id
                WHERE selection.campaign_id = campaign.campaign_id
                  AND member.audience_id = processed_id.audience_id
            )
        ), '{}'::bigint[]) AS processed_only_audience_ids,
        COALESCE((
            SELECT ARRAY_AGG(DISTINCT member.audience_id ORDER BY
                member.audience_id
            )
            FROM bundle_audience_selections AS selection
            JOIN bundle_audience_selection_members AS member
              ON member.selection_id = selection.id
            WHERE selection.campaign_id = campaign.campaign_id
              AND NOT (
                  member.audience_id = ANY(COALESCE(
                      campaign.processed_audience_ids,
                      '{}'::bigint[]
                  ))
              )
        ), '{}'::bigint[]) AS selection_only_audience_ids
    FROM campaign_context AS campaign
),
audit_inputs AS (
    SELECT
        campaign.*,
        records.selection_record_count,
        records.selection_recorded_count,
        records.selection_ids,
        records.wrong_selection_bundle_count,
        structure.selected_member_count,
        structure.duplicate_audience_occurrence_count,
        structure.duplicate_selection_order_count,
        structure.wrong_member_bundle_count,
        COALESCE(contiguous.noncontiguous_selection_count, 0)
            AS noncontiguous_selection_count,
        COALESCE(audience.selected_audience_ids_in_order, '{}'::bigint[])
            AS selected_audience_ids_in_order,
        COALESCE(audience.duplicate_within_campaign_ids, '{}'::bigint[])
            AS duplicate_within_campaign_ids,
        COALESCE(audience.duplicate_from_previous_campaign_ids, '{}'::bigint[])
            AS duplicate_from_previous_campaign_ids,
        COALESCE(audience.missing_profile_ids, '{}'::bigint[])
            AS missing_profile_ids,
        COALESCE(audience.tag_mismatch_ids, '{}'::bigint[])
            AS tag_mismatch_ids,
        COALESCE(audience.grade_mismatch_ids, '{}'::bigint[])
            AS grade_mismatch_ids,
        COALESCE(audience.platform_color_mismatch_ids, '{}'::bigint[])
            AS platform_color_mismatch_ids,
        COALESCE(audience.missing_phone_ids, '{}'::bigint[])
            AS missing_phone_ids,
        processed_array.duplicate_processed_audience_ids,
        processed_array.processed_only_audience_ids,
        processed_array.selection_only_audience_ids
    FROM campaign_context AS campaign
    JOIN selection_records AS records
      ON records.campaign_id = campaign.campaign_id
    JOIN selection_member_structure AS structure
      ON structure.campaign_id = campaign.campaign_id
    LEFT JOIN noncontiguous_selections AS contiguous
      ON contiguous.campaign_id = campaign.campaign_id
    LEFT JOIN audience_audit AS audience
      ON audience.campaign_id = campaign.campaign_id
    JOIN processed_array_audit AS processed_array
      ON processed_array.campaign_id = campaign.campaign_id
),
audit_results AS (
    SELECT
        audit.*,
        ARRAY_REMOVE(ARRAY[
            CASE WHEN audit.processed_campaign_id IS NULL
                THEN 'missing_current_processed_campaign' END,
            CASE WHEN audit.selection_record_count <> 1
                THEN 'selection_record_count_not_one' END,
            CASE WHEN audit.selection_recorded_count
                    <> audit.selected_member_count
                THEN 'selection_audience_count_mismatch' END,
            CASE WHEN audit.wrong_selection_bundle_count > 0
                THEN 'selection_bundle_mismatch' END,
            CASE WHEN audit.wrong_member_bundle_count > 0
                THEN 'selection_member_bundle_mismatch' END,
            CASE WHEN audit.duplicate_audience_occurrence_count > 0
                THEN 'duplicate_audience_within_campaign' END,
            CASE WHEN audit.duplicate_selection_order_count > 0
                THEN 'duplicate_selection_order' END,
            CASE WHEN audit.noncontiguous_selection_count > 0
                THEN 'noncontiguous_selection_order' END,
            CASE WHEN CARDINALITY(
                    audit.duplicate_processed_audience_ids
                ) > 0
                THEN 'duplicate_audience_in_processed_checkpoint' END,
            CASE WHEN CARDINALITY(
                    audit.duplicate_from_previous_campaign_ids
                ) > 0
                THEN 'audience_reused_from_previous_campaign' END,
            CASE WHEN CARDINALITY(audit.missing_profile_ids) > 0
                THEN 'audience_profile_missing' END,
            CASE WHEN CARDINALITY(audit.tag_mismatch_ids) > 0
                THEN 'campaign_tags_mismatch' END,
            CASE WHEN audit.grade_filter_required
                    AND (audit.p33 IS NULL OR audit.p66 IS NULL)
                THEN 'required_score_bounds_missing' END,
            CASE WHEN CARDINALITY(audit.grade_mismatch_ids) > 0
                THEN 'campaign_grades_mismatch' END,
            CASE WHEN CARDINALITY(audit.platform_color_mismatch_ids) > 0
                THEN 'platform_color_mismatch' END,
            CASE WHEN CARDINALITY(audit.missing_phone_ids) > 0
                THEN 'missing_or_blank_phone' END,
            CASE WHEN audit.requested_audience_count IS NOT NULL
                    AND audit.selected_member_count
                        > audit.requested_audience_count
                THEN 'selected_more_than_requested' END,
            CASE WHEN audit.selection_record_count = 1
                    AND audit.processed_selection_id IS DISTINCT FROM
                        audit.selection_ids[1]
                THEN 'processed_selection_reference_mismatch' END,
            CASE WHEN audit.selection_record_count = 1
                    AND audit.processed_audience_ids IS DISTINCT FROM
                        audit.selected_audience_ids_in_order
                THEN 'processed_audience_ids_or_order_mismatch' END
        ], NULL)::text[] AS failed_checks
    FROM audit_inputs AS audit
)
SELECT
    result.execution_position,
    result.processed_campaign_id,
    result.campaign_id,
    result.campaign_title,
    result.status,
    result.phase,
    result.platform,
    result.requested_audience_count,
    result.selected_member_count AS selected_audience_count,
    CARDINALITY(result.failed_checks) = 0 AS is_valid,
    result.failed_checks,
    result.tag_ids,
    result.audience_grades,
    result.p33,
    result.p66,
    result.selection_record_count,
    result.selection_recorded_count,
    result.duplicate_within_campaign_ids,
    result.duplicate_processed_audience_ids,
    result.duplicate_from_previous_campaign_ids,
    result.processed_only_audience_ids,
    result.selection_only_audience_ids,
    result.missing_profile_ids,
    result.tag_mismatch_ids,
    result.grade_mismatch_ids,
    result.platform_color_mismatch_ids,
    result.missing_phone_ids,
    result.duplicate_selection_order_count,
    result.noncontiguous_selection_count,
    result.wrong_selection_bundle_count,
    result.wrong_member_bundle_count,
    result.processed_selection_id,
    result.selection_ids,
    result.processed_audience_ids IS NOT DISTINCT FROM
        result.selected_audience_ids_in_order
        AS processed_audience_ids_and_order_match
FROM audit_results AS result
ORDER BY
    result.processed_campaign_id ASC NULLS LAST,
    result.campaign_id ASC;



