-- 0016_add_comment_to_sms_campaigns_down.sql
-- Remove comment column from sms_campaigns
ALTER TABLE sms_campaigns
	DROP COLUMN IF EXISTS comment; 