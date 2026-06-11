-- Migration: 0089_add_unique_customer_name_to_platform_settings.sql
-- Description: Enforce unique platform settings name per customer

-- UP MIGRATION

CREATE UNIQUE INDEX IF NOT EXISTS uk_platform_settings_customer_name
ON platform_settings(customer_id, name)
WHERE name IS NOT NULL;
