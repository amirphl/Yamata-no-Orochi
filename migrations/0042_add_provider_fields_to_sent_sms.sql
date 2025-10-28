-- Migration: 0042_add_provider_fields_to_sent_sms.sql
-- Description: Add optional provider response fields to sent_sms

-- UP MIGRATION

ALTER TABLE sent_sms
	ADD COLUMN IF NOT EXISTS server_id VARCHAR(64) NULL,
	ADD COLUMN IF NOT EXISTS error_code VARCHAR(64) NULL,
	ADD COLUMN IF NOT EXISTS description TEXT NULL;

-- Indexes might not be necessary; add only if you plan to query by these fields frequently
-- CREATE INDEX IF NOT EXISTS idx_sent_sms_server_id ON sent_sms(server_id);
-- CREATE INDEX IF NOT EXISTS idx_sent_sms_error_code ON sent_sms(error_code); 