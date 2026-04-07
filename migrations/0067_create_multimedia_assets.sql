-- Migration: 0067_create_multimedia_assets.sql
-- Description: Create multimedia_assets table to store uploaded images and videos

-- UP MIGRATION

-- Ensure pgcrypto is available for gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS multimedia_assets (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    customer_id BIGINT NOT NULL,
    original_filename VARCHAR(255) NOT NULL,
    stored_path TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    media_type VARCHAR(20) NOT NULL,
    extension VARCHAR(20) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_multimedia_assets_customer_id FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_multimedia_assets_uuid ON multimedia_assets(uuid);
CREATE INDEX IF NOT EXISTS idx_multimedia_assets_customer_id ON multimedia_assets(customer_id);
CREATE INDEX IF NOT EXISTS idx_multimedia_assets_media_type ON multimedia_assets(media_type);
CREATE INDEX IF NOT EXISTS idx_multimedia_assets_created_at ON multimedia_assets(created_at);
