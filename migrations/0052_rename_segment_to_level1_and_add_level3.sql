-- Migration: Rename `segment` -> `level1`, `subsegments` -> `level2s` and add `level3s` in `spec` JSONB
-- Up migration

BEGIN;

-- 1) Copy `segment` -> `level1` and `subsegments` -> `level2s` when present
UPDATE sms_campaigns
SET spec = (
    spec
    || CASE WHEN spec ? 'segment' THEN jsonb_build_object('level1', spec->'segment') ELSE '{}'::jsonb END
    || CASE WHEN spec ? 'subsegments' THEN jsonb_build_object('level2s', spec->'subsegments') ELSE '{}'::jsonb END
)
WHERE spec ? 'segment' OR spec ? 'subsegments';

-- 2) Remove old keys if they exist
UPDATE sms_campaigns
SET spec = spec - 'segment' - 'subsegments'
WHERE spec ? 'segment' OR spec ? 'subsegments';

-- 3) Ensure `level3s` key exists (default to empty array) for all rows that don't have it
UPDATE sms_campaigns
SET spec = spec || jsonb_build_object('level3s', '[]'::jsonb)
WHERE NOT (spec ? 'level3s');

COMMIT;

-- Notes:
-- - This migration operates on the JSONB `spec` column and is idempotent: keys are only copied if present.
-- - `level3s` is initialized to an empty JSON array when missing.
