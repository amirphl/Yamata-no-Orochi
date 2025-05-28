-- Migration: Create SMS campaigns table
-- Down migration

-- Drop indexes first
DROP INDEX IF EXISTS idx_sms_campaigns_updated_at;
DROP INDEX IF EXISTS idx_sms_campaigns_created_at;
DROP INDEX IF EXISTS idx_sms_campaigns_status;
DROP INDEX IF EXISTS idx_sms_campaigns_customer_id;

-- Drop the table
DROP TABLE IF EXISTS sms_campaigns;

-- Drop the enum type
DROP TYPE IF EXISTS sms_campaign_status; 