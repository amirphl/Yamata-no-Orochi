-- Migration: 0091_add_admin_platform_base_price_audit_actions.sql
-- Description: Add audit_action_enum values for admin platform base price operations

BEGIN;

ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_platform_base_price_list';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_platform_base_price_update';

COMMIT;
