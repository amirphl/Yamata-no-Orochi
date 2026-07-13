-- Description: Make the inferred audience targeting method explicit for existing campaigns

BEGIN;

-- Preserve valid explicit choices. For legacy specs, use the same deterministic
-- priority as the application: Smart Targeting selections, Excel audience
-- file, then standard level-based targeting.
UPDATE campaigns AS c
SET spec = jsonb_set(
    CASE
        WHEN jsonb_typeof(c.spec) = 'object' THEN c.spec
        ELSE '{}'::jsonb
    END,
    '{audience_targeting_method}',
    to_jsonb(
        CASE
            WHEN EXISTS (
                SELECT 1
                FROM campaign_selected_tags AS cst
                WHERE cst.campaign_id = c.id
            ) THEN 'smart_targeting'
            WHEN NULLIF(BTRIM(c.spec->>'target_audience_excel_file_uuid'), '') IS NOT NULL THEN 'excel'
            ELSE 'standard'
        END
    ),
    true
)
WHERE NULLIF(BTRIM(c.spec->>'audience_targeting_method'), '') IS NULL;

COMMIT;
