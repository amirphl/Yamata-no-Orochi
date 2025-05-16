-- Migration: 0007_add_missing_audit_actions_down.sql
-- Description: Down migration for adding missing audit log actions

-- DOWN MIGRATION
-- Note: PostgreSQL doesn't support removing enum values directly
-- This is a destructive operation that requires recreating the enum

-- Create new enum without the added values
CREATE TYPE audit_action_enum_new AS ENUM (
    'signup_initiated',
    'signup_completed',
    'email_verified',
    'mobile_verified',
    'login_success',
    'login_failed',
    'logout',
    'password_changed',
    'profile_updated',
    'account_activated',
    'account_deactivated',
    'session_created',
    'session_expired',
    'otp_generated',
    'otp_verified',
    'otp_failed'
);

-- Update table to use new enum
ALTER TABLE audit_log
    ALTER COLUMN action TYPE audit_action_enum_new
    USING action::text::audit_action_enum_new;

-- Drop old enum
DROP TYPE audit_action_enum;

-- Rename new enum to original name
ALTER TYPE audit_action_enum_new RENAME TO audit_action_enum;