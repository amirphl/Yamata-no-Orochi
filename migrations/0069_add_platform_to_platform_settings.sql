-- Migration: 0069_add_platform_to_platform_settings.sql
-- Description: Add platform column to platform_settings

-- UP MIGRATION

ALTER TABLE platform_settings
ADD COLUMN IF NOT EXISTS platform VARCHAR(20) NOT NULL DEFAULT 'rubika';

CREATE INDEX IF NOT EXISTS idx_platform_settings_platform ON platform_settings(platform);
