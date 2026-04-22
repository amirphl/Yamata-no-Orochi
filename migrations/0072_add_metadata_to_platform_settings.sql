-- Migration: 0072_add_metadata_to_platform_settings.sql
-- Description: Add metadata jsonb column to platform_settings

ALTER TABLE platform_settings
    ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
