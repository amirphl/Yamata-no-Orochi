-- Migration: 0090_add_expired_status_to_sms_campaigns.sql
-- Description: Add 'expired' status to sms_campaign_status enum

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_type t
        JOIN pg_enum e ON t.oid = e.enumtypid
        WHERE t.typname = 'sms_campaign_status' AND e.enumlabel = 'expired'
    ) THEN
        ALTER TYPE sms_campaign_status ADD VALUE 'expired';
    END IF;
END$$;
