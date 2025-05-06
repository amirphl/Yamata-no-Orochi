-- Migration: 0007_add_missing_audit_actions.sql
-- Description: Add missing audit log actions to the audit_action_enum

-- UP MIGRATION
-- Add missing audit actions to the enum
ALTER TYPE audit_action_enum ADD VALUE 'login_successful';
ALTER TYPE audit_action_enum ADD VALUE 'password_reset_requested';
ALTER TYPE audit_action_enum ADD VALUE 'password_reset_completed';
ALTER TYPE audit_action_enum ADD VALUE 'password_reset_failed';

-- DOWN MIGRATION
-- Note: PostgreSQL doesn't support removing enum values directly
-- This would require recreating the enum and updating the table
-- For now, we'll leave the enum values in place as they don't cause issues
-- If removal is needed in the future, a more complex migration would be required

-- Alternative approach for down migration (commented out as it's destructive):
-- 1. Create new enum without the values
-- 2. Update table to use new enum
-- 3. Drop old enum
-- 4. Rename new enum to old name
-- 
-- CREATE TYPE audit_action_enum_new AS ENUM (
--     'signup_initiated',
--     'signup_completed',
--     'email_verified',
--     'mobile_verified',
--     'login_success',
--     'login_failed',
--     'logout',
--     'password_changed',
--     'profile_updated',
--     'account_activated',
--     'account_deactivated',
--     'session_created',
--     'session_expired',
--     'otp_generated',
--     'otp_verified',
--     'otp_failed'
-- );
-- 
-- ALTER TABLE audit_log 
--     ALTER COLUMN action TYPE audit_action_enum_new 
--     USING action::text::audit_action_enum_new;
-- 
-- DROP TYPE audit_action_enum;
-- ALTER TYPE audit_action_enum_new RENAME TO audit_action_enum; 