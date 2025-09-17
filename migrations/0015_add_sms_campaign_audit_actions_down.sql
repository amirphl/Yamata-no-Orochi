-- Migration: Add SMS campaign audit actions
-- Down: Remove new audit action enum values for SMS campaign operations
 
-- Note: PostgreSQL doesn't support removing enum values directly
-- The enum values 'campaign_created' and 'campaign_creation_failed' will remain in the type
-- but won't be used. If you need to completely remove them, you would need to recreate the enum type. 