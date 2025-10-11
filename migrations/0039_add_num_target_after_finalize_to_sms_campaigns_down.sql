-- Migration: 0039_add_num_target_after_finalize_to_sms_campaigns_down.sql
-- Description: Drop num_audience column from sms_campaigns

-- DOWN MIGRATION

ALTER TABLE sms_campaigns
DROP COLUMN IF EXISTS num_audience; 