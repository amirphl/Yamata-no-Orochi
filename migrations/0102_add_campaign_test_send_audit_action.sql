-- Migration: 0102_add_campaign_test_send_audit_action.sql
-- Description: Add audit_action_enum value for campaign test-send operation

BEGIN;

ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'campaign_test_send';

COMMIT;
