-- Migration: 0068_create_platform_settings.sql
-- Description: Create platform_settings table

-- UP MIGRATION

-- Ensure pgcrypto is available for gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS platform_settings (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    customer_id BIGINT NOT NULL,
    name VARCHAR(255),
    description TEXT,
    multimedia_id BIGINT,
    status VARCHAR(20) NOT NULL DEFAULT 'initialized',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_platform_settings_customer_id FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE,
    CONSTRAINT fk_platform_settings_multimedia_id FOREIGN KEY (multimedia_id) REFERENCES multimedia_assets(id) ON DELETE SET NULL
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_platform_settings_uuid ON platform_settings(uuid);
CREATE INDEX IF NOT EXISTS idx_platform_settings_customer_id ON platform_settings(customer_id);
CREATE INDEX IF NOT EXISTS idx_platform_settings_status ON platform_settings(status);
CREATE INDEX IF NOT EXISTS idx_platform_settings_multimedia_id ON platform_settings(multimedia_id);
CREATE INDEX IF NOT EXISTS idx_platform_settings_created_at ON platform_settings(created_at);
