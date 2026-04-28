-- Migration: 0075_create_sent_splus_messages.sql
-- Description: Create sent_splus_messages table and splus_send_status enum

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'splus_send_status') THEN
        CREATE TYPE splus_send_status AS ENUM ('pending','successful','unsuccessful');
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS sent_splus_messages (
    id BIGSERIAL PRIMARY KEY,
    processed_campaign_id BIGINT NOT NULL REFERENCES processed_campaigns(id) ON DELETE CASCADE,
    phone_number VARCHAR(20) NOT NULL,
    tracking_id VARCHAR(64) NOT NULL,
    parts_delivered INTEGER NOT NULL DEFAULT 0,
    status splus_send_status NOT NULL DEFAULT 'pending',
    server_id VARCHAR(64),
    error_code VARCHAR(64),
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sent_splus_messages_processed_campaign_id ON sent_splus_messages(processed_campaign_id);
CREATE INDEX IF NOT EXISTS idx_sent_splus_messages_phone_number ON sent_splus_messages(phone_number);
CREATE INDEX IF NOT EXISTS idx_sent_splus_messages_tracking_id ON sent_splus_messages(tracking_id);
CREATE INDEX IF NOT EXISTS idx_sent_splus_messages_status ON sent_splus_messages(status);
CREATE INDEX IF NOT EXISTS idx_sent_splus_messages_created_at ON sent_splus_messages(created_at);
