-- Migration: 0074_replicate_sms_segment_price_factors_to_other_platforms.sql
-- Description: Copy latest sms segment price factors to bale/rubika/splus when target platform+level3 is missing

WITH latest_sms AS (
    SELECT level3, price_factor
    FROM (
        SELECT
            level3,
            price_factor,
            ROW_NUMBER() OVER (PARTITION BY level3 ORDER BY created_at DESC, id DESC) AS rn
        FROM segment_price_factors
        WHERE platform = 'sms'
    ) t
    WHERE t.rn = 1
),
target_platforms AS (
    SELECT 'bale'::varchar(20) AS platform
    UNION ALL SELECT 'rubika'::varchar(20)
    UNION ALL SELECT 'splus'::varchar(20)
)
INSERT INTO segment_price_factors (platform, level3, price_factor)
SELECT tp.platform, ls.level3, ls.price_factor
FROM latest_sms ls
CROSS JOIN target_platforms tp
WHERE NOT EXISTS (
    SELECT 1
    FROM segment_price_factors spf
    WHERE spf.platform = tp.platform
      AND spf.level3 = ls.level3
);
