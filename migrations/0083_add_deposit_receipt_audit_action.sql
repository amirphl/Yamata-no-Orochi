-- Migration: 0083_add_deposit_receipt_audit_action.sql
-- Description: Add audit_action_enum value for deposit receipt submission

BEGIN;

ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'deposit_receipt_submitted';

COMMIT;
