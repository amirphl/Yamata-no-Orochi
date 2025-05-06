-- Migration: 0008_update_audit_log_success_field_down.sql
-- Description: Revert audit_log success field back to non-nullable

-- DOWN MIGRATION
-- Drop the new constraint
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS chk_error_when_not_success;

-- Change success field back to non-nullable with default
ALTER TABLE audit_log ALTER COLUMN success SET NOT NULL;
ALTER TABLE audit_log ALTER COLUMN success SET DEFAULT TRUE;

-- Add back the original constraint
ALTER TABLE audit_log ADD CONSTRAINT chk_error_when_not_success CHECK (
    (success = TRUE AND error_message IS NULL) OR
    (success = FALSE AND error_message IS NOT NULL)
);

-- Recreate the original indexes
DROP INDEX IF EXISTS idx_audit_success;
CREATE INDEX idx_audit_success ON audit_log(success);

DROP INDEX IF EXISTS idx_audit_failed_actions;
CREATE INDEX idx_audit_failed_actions ON audit_log(success, action, created_at) WHERE success = FALSE; 