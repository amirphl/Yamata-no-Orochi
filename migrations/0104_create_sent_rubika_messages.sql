-- Migration: 0104_create_sent_rubika_messages.sql
-- Description: Create sent_rubika_messages table and rubika_send_status enum

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'rubika_send_status') THEN
        CREATE TYPE rubika_send_status AS ENUM ('pending','successful','unsuccessful');
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS sent_rubika_messages (
    id BIGSERIAL PRIMARY KEY,
    processed_campaign_id BIGINT NOT NULL REFERENCES processed_campaigns(id) ON DELETE CASCADE,
    phone_number VARCHAR(20) NOT NULL,
    tracking_id VARCHAR(64) NOT NULL,
    parts_delivered INTEGER NOT NULL DEFAULT 0,
    status rubika_send_status NOT NULL DEFAULT 'pending',
    server_id VARCHAR(64),
    error_code VARCHAR(64),
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sent_rubika_messages_processed_campaign_id ON sent_rubika_messages(processed_campaign_id);
CREATE INDEX IF NOT EXISTS idx_sent_rubika_messages_phone_number ON sent_rubika_messages(phone_number);
CREATE INDEX IF NOT EXISTS idx_sent_rubika_messages_tracking_id ON sent_rubika_messages(tracking_id);
CREATE INDEX IF NOT EXISTS idx_sent_rubika_messages_status ON sent_rubika_messages(status);
CREATE INDEX IF NOT EXISTS idx_sent_rubika_messages_created_at ON sent_rubika_messages(created_at);
