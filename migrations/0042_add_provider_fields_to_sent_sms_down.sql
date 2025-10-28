-- Migration: 0042_add_provider_fields_to_sent_sms_down.sql
-- Description: Revert adding optional provider response fields to sent_sms

-- DOWN MIGRATION

ALTER TABLE sent_sms
	DROP COLUMN IF EXISTS server_id,
	DROP COLUMN IF EXISTS error_code,
	DROP COLUMN IF EXISTS description; 