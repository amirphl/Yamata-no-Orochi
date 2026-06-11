-- Migration: 0089_add_unique_customer_name_to_platform_settings_down.sql
-- Description: Drop unique platform settings name per customer index

-- DOWN MIGRATION

DROP INDEX IF EXISTS uk_platform_settings_customer_name;
