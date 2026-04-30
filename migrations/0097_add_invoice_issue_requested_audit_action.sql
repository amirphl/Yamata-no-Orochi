-- Migration: 0097_add_invoice_issue_requested_audit_action.sql
-- Description: Add audit_action_enum value for invoice issue request operation

BEGIN;

ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'invoice_issue_requested';

COMMIT;
