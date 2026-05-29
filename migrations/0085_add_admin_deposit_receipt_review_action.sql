-- Migration: 0085_add_admin_deposit_receipt_review_action.sql
-- Description: Add audit action for admin deposit receipt review

BEGIN;

ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_deposit_receipt_reviewed';

COMMIT;
