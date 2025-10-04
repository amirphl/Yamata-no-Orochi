-- Migration: 0035_add_running_executed_to_sms_campaign_status.sql
-- Description: Add 'running' and 'executed' to sms_campaign_status enum

-- UP MIGRATION

-- Add new enum values if not already present
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type t JOIN pg_enum e ON t.oid = e.enumtypid WHERE t.typname = 'sms_campaign_status' AND e.enumlabel = 'running') THEN
        ALTER TYPE sms_campaign_status ADD VALUE 'running';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type t JOIN pg_enum e ON t.oid = e.enumtypid WHERE t.typname = 'sms_campaign_status' AND e.enumlabel = 'executed') THEN
        ALTER TYPE sms_campaign_status ADD VALUE 'executed';
    END IF;
END$$; 