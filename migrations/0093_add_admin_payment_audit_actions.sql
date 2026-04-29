-- Migration: 0093_add_admin_payment_audit_actions.sql
-- Description: Add audit_action_enum values for admin payment operations

BEGIN;

ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_list_transactions';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_add_invoice_to_payment_request';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_add_invoice_to_transaction';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_download_deposit_receipt_file';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_update_deposit_receipt_status';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_attach_invoice_to_transaction';

COMMIT;
