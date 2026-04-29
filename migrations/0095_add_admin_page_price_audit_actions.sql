-- Migration: 0095_add_admin_page_price_audit_actions.sql
-- Description: Add audit_action_enum values for admin page price operations

BEGIN;

ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_get_page_prices';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_update_page_price';

COMMIT;
