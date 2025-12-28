-- Migration: Revert rename of `level1` -> `segment`, `level2s` -> `subsegments` and remove `level3s` from `spec` JSONB
-- Down migration

BEGIN;

-- 1) Copy `level1` -> `segment` and `level2s` -> `subsegments` when present
UPDATE sms_campaigns
SET spec = (
    spec
    || CASE WHEN spec ? 'level1' THEN jsonb_build_object('segment', spec->'level1') ELSE '{}'::jsonb END
    || CASE WHEN spec ? 'level2s' THEN jsonb_build_object('subsegments', spec->'level2s') ELSE '{}'::jsonb END
)
WHERE spec ? 'level1' OR spec ? 'level2s';

-- 2) Remove new keys `level1`, `level2s` and `level3s`
UPDATE sms_campaigns
SET spec = spec - 'level1' - 'level2s' - 'level3s'
WHERE spec ? 'level1' OR spec ? 'level2s' OR spec ? 'level3s';

COMMIT;

-- Notes:
-- - This down migration restores the original keys and removes `level3s` if present.
