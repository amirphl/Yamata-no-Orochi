-- Migration: 0096_add_campaign_refund_reconcile_failed_audit_action.sql
-- Description: Add audit_action_enum value for campaign refund reconciliation failures

BEGIN;

ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'campaign_refund_reconcile_failed';

COMMIT;
