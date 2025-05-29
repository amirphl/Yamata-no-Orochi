-- Migration: Add SMS campaign audit actions
-- Down: Remove new audit action constants for SMS campaign operations

-- Remove the audit action constants
DELETE FROM audit_actions WHERE action_name IN ('campaign_created', 'campaign_creation_failed');

-- Note: PostgreSQL doesn't support removing enum values directly
-- The enum values will remain in the type but won't be used
-- If you need to completely remove them, you would need to recreate the enum type 