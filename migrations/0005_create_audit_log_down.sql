-- Migration: 0005_create_audit_log_down.sql
-- Description: Rollback audit log table

-- DOWN MIGRATION
DROP TABLE IF EXISTS audit_log CASCADE;
DROP TYPE IF EXISTS audit_action_enum CASCADE; 