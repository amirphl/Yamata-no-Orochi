-- Migration: 0094_add_campaign_report_export_audit_actions.sql
-- Description: Add audit_action_enum values for campaign report export operations

BEGIN;

ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'campaign_report_exported';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'campaign_report_export_failed';

COMMIT;
