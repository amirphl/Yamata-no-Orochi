-- Migration: 0072_add_metadata_to_platform_settings_down.sql
-- Description: Drop metadata column from platform_settings

ALTER TABLE platform_settings
    DROP COLUMN IF EXISTS metadata;
