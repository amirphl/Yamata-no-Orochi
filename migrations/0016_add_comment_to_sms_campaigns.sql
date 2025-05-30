-- 0016_add_comment_to_sms_campaigns.sql
-- Add nullable comment column to sms_campaigns for admin rejection notes
ALTER TABLE sms_campaigns
	ADD COLUMN IF NOT EXISTS comment TEXT; 