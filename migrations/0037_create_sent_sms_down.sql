-- Migration: 0037_create_sent_sms_down.sql
-- Description: Drop sent_sms table (enum left intact for safety)

-- DOWN MIGRATION

DROP TABLE IF EXISTS sent_sms; 