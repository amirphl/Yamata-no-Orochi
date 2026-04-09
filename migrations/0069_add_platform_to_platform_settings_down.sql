-- Migration: 0069_add_platform_to_platform_settings_down.sql
-- Description: Drop platform column from platform_settings

-- DOWN MIGRATION

DROP INDEX IF EXISTS idx_platform_settings_platform;
ALTER TABLE platform_settings DROP COLUMN IF EXISTS platform;
