-- Migration: 0008_update_audit_log_success_field.sql
-- Description: Update audit_log success field to be nullable and fix constraint

-- UP MIGRATION
-- Drop the existing constraint first
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS chk_error_when_not_success;

-- Change success field to be nullable (remove DEFAULT TRUE)
ALTER TABLE audit_log ALTER COLUMN success DROP DEFAULT;
ALTER TABLE audit_log ALTER COLUMN success DROP NOT NULL;

-- Update existing records to have success = true where it's currently null
-- (This maintains backward compatibility for existing data)
UPDATE audit_log SET success = true WHERE success IS NULL;

-- Add new constraint that handles nullable success field
ALTER TABLE audit_log ADD CONSTRAINT chk_error_when_not_success CHECK (
    (success IS NULL AND error_message IS NULL) OR
    (success = TRUE AND error_message IS NULL) OR
    (success = FALSE AND error_message IS NOT NULL)
);

-- Update the index to handle nullable values
DROP INDEX IF EXISTS idx_audit_success;
CREATE INDEX idx_audit_success ON audit_log(success) WHERE success IS NOT NULL;

-- Update the failed actions index
DROP INDEX IF EXISTS idx_audit_failed_actions;
CREATE INDEX idx_audit_failed_actions ON audit_log(success, action, created_at) WHERE success = FALSE; 