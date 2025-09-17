-- Migration: 0027_add_discount_audit_actions.sql
-- Description: Add audit actions for agency discount creation success and failure

-- UP MIGRATION
BEGIN;

ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'create_discount_by_agency_completed';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'create_discount_by_agency_failed';

COMMIT; 