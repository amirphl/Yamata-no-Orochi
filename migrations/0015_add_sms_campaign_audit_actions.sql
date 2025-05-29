-- Migration: Add SMS campaign audit actions
-- Up: Add new audit action constants for SMS campaign operations

-- Add new audit action constants
INSERT INTO audit_actions (action_name, description, created_at, updated_at) VALUES
    ('campaign_created', 'SMS campaign was successfully created', NOW(), NOW()),
    ('campaign_creation_failed', 'SMS campaign creation failed', NOW(), NOW());

-- Update the enum type to include new values
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'campaign_created';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'campaign_creation_failed'; 