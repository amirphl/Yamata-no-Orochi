-- Migration: 0071_add_cancelled_by_admin_status_to_sms_campaigns.sql
-- Description: Add 'cancelled-by-admin' status to sms_campaign_status enum

-- UP MIGRATION
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_type t
        JOIN pg_enum e ON t.oid = e.enumtypid
        WHERE t.typname = 'sms_campaign_status' AND e.enumlabel = 'cancelled-by-admin'
    ) THEN
        ALTER TYPE sms_campaign_status ADD VALUE 'cancelled-by-admin';
    END IF;
END$$;