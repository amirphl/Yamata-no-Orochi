-- Migration: 0106_add_admin_preview_wallet_charge_impact_audit_actions.sql
-- Description: Add audit_action_enum values for admin wallet charge impact preview operations

BEGIN;

ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_preview_wallet_charge_impact_succeeded';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_preview_wallet_charge_impact_failed';

COMMIT;
