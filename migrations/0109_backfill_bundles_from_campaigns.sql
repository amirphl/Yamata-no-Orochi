-- Description: Backfill one bundle per existing campaign

BEGIN;

INSERT INTO bundles (
    title,
    objective,
    target_audience_persona,
    adlink,
    short_link_domain,
    target_customer_name,
    category,
    job,
    metadata,
    statistics,
    customer_id,
    created_at,
    updated_at
)
SELECT
    COALESCE(NULLIF(BTRIM(c.spec->>'title'), ''), 'Campaign ' || c.id::text) AS title,
    '' AS objective,
    '' AS target_audience_persona,
    NULLIF(BTRIM(c.spec->>'adlink'), '') AS adlink,
    NULLIF(BTRIM(c.spec->>'short_link_domain'), '') AS short_link_domain,
    NULL AS target_customer_name,
    NULLIF(BTRIM(c.spec->>'category'), '') AS category,
    NULLIF(BTRIM(c.spec->>'job'), '') AS job,
    jsonb_build_object('_backfill_campaign_id', c.id) AS metadata,
    COALESCE(c.statistics, '{}'::jsonb) AS statistics,
    c.customer_id,
    c.created_at,
    COALESCE(c.updated_at, c.created_at)
FROM campaigns c
WHERE NOT EXISTS (
    SELECT 1
    FROM bundles b
    WHERE b.metadata->>'_backfill_campaign_id' = c.id::text
);

COMMIT;
