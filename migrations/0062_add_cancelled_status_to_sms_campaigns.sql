-- Migration: 0062_add_cancelled_status_to_sms_campaigns.sql
-- Description: Add 'cancelled' status to sms_campaign_status enum

-- UP MIGRATION
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_type t
        JOIN pg_enum e ON t.oid = e.enumtypid
        WHERE t.typname = 'sms_campaign_status' AND e.enumlabel = 'cancelled'
    ) THEN
        ALTER TYPE sms_campaign_status ADD VALUE 'cancelled';
    END IF;
END$$;
