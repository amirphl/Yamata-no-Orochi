-- Migration: 0037_create_sent_sms.sql
-- Description: Create sent_sms table and sent_sms_status enum

-- UP MIGRATION

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'sent_sms_status') THEN
        CREATE TYPE sent_sms_status AS ENUM ('pending','successful','unsuccessful');
    END IF;
END$$;

-- Ensure pgcrypto or uuid-ossp if you plan to default-generate UUIDs (optional)
-- CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS sent_sms (
    id BIGSERIAL PRIMARY KEY,
    processed_campaign_id BIGINT NOT NULL REFERENCES processed_campaigns(id) ON DELETE CASCADE,
    phone_number VARCHAR(20) NOT NULL,
    tracking_id UUID NOT NULL,
    parts_delivered INTEGER NOT NULL DEFAULT 0,
    status sent_sms_status NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sent_sms_processed_campaign_id ON sent_sms(processed_campaign_id);
CREATE INDEX IF NOT EXISTS idx_sent_sms_phone_number ON sent_sms(phone_number);
CREATE INDEX IF NOT EXISTS idx_sent_sms_tracking_id ON sent_sms(tracking_id);
CREATE INDEX IF NOT EXISTS idx_sent_sms_status ON sent_sms(status);
CREATE INDEX IF NOT EXISTS idx_sent_sms_created_at ON sent_sms(created_at); 