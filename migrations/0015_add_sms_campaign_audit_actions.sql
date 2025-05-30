-- Migration: Add SMS campaign audit actions
-- Up: Add new audit action enum values for SMS campaign operations

-- Add new enum values to audit_action_enum type
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'campaign_created';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'campaign_creation_failed'; 
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'campaign_updated';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'campaign_update_failed';