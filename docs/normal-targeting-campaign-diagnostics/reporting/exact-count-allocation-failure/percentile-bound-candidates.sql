-- Show every percentile pair matching campaign 984's configured levels. If
-- distinct_bound_pair_count > 1, compare the scheduler log's
-- resolveScoreConstraint p33/p66 values with audience-reduction-funnel.sql.

WITH params AS (
    SELECT 1108::bigint AS campaign_id
),
campaign_levels AS (
    SELECT
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
        )::text[] AS level3s
    FROM params
    JOIN campaigns AS campaign
      ON campaign.id = params.campaign_id
)
SELECT
    stats.p33,
    stats.p66,
    COUNT(*) AS matching_row_count,
    COUNT(*) OVER () AS distinct_bound_pair_count,
    MIN(stats.calculated_at) AS earliest_calculated_at,
    MAX(stats.calculated_at) AS latest_calculated_at
FROM campaign_levels AS levels
JOIN src_layer_all_stats AS stats
  ON stats.p33 IS NOT NULL
 AND stats.p66 IS NOT NULL
 AND (levels.level1 IS NULL OR stats.layer1_category = levels.level1)
 AND (
     CARDINALITY(levels.level2s) = 0
     OR stats.layer2_category = ANY(levels.level2s)
 )
 AND (
     CARDINALITY(levels.level3s) = 0
     OR stats.layer3_category = ANY(levels.level3s)
 )
GROUP BY stats.p33, stats.p66
ORDER BY stats.p33, stats.p66;

