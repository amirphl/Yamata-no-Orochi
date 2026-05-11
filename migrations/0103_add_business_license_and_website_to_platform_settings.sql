-- Migration: 0103_add_business_license_and_website_to_platform_settings.sql
-- Description: Add business_license_id and website columns to platform_settings

ALTER TABLE platform_settings
    ADD COLUMN IF NOT EXISTS business_license_id BIGINT,
    ADD COLUMN IF NOT EXISTS website TEXT;

ALTER TABLE platform_settings
    ADD CONSTRAINT fk_platform_settings_business_license_id FOREIGN KEY (business_license_id) REFERENCES multimedia_assets(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_platform_settings_business_license_id ON platform_settings(business_license_id);
