-- Migration: 0103_add_business_license_and_website_to_platform_settings_down.sql
-- Description: Drop business_license_id and website columns from platform_settings

DROP INDEX IF EXISTS idx_platform_settings_business_license_id;

ALTER TABLE platform_settings
    DROP CONSTRAINT IF EXISTS fk_platform_settings_business_license_id,
    DROP COLUMN IF EXISTS business_license_id,
    DROP COLUMN IF EXISTS website;
