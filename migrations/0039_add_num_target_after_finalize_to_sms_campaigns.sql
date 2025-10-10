-- Migration: 0039_add_num_target_after_finalize_to_sms_campaigns.sql
-- Description: Add num_audience column to sms_campaigns

-- UP MIGRATION

ALTER TABLE sms_campaigns
ADD COLUMN IF NOT EXISTS num_audience BIGINT; 