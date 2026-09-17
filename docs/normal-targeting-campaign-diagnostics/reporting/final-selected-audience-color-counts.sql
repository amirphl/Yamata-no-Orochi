-- Final selected audience counts by color
--
-- This reads the current processed campaign's persisted audience IDs, which
-- are the scheduler's final recipient set. It therefore applies only after a
-- campaign has been prepared. Set campaign_id below before running.

WITH params AS (
    SELECT 955::bigint AS campaign_id
),
selected_audiences AS (
    SELECT DISTINCT selected.audience_id
    FROM params
    JOIN processed_campaigns AS processed
      ON processed.campaign_id = params.campaign_id
     AND processed.is_current
    CROSS JOIN LATERAL UNNEST(processed.audience_ids)
        AS selected(audience_id)
)
SELECT
    COUNT(*) FILTER (
        WHERE LOWER(BTRIM(audience.color)) = 'white'
    ) AS white_count,
    COUNT(*) FILTER (
        WHERE LOWER(BTRIM(audience.color)) = 'pink'
    ) AS pink_count,
    COUNT(*) FILTER (
        WHERE LOWER(BTRIM(audience.color)) = 'black'
    ) AS black_count
FROM selected_audiences AS selected
JOIN audience_profiles AS audience
  ON audience.id = selected.audience_id;
