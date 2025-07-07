-- Migration: 0028_add_launch_campaign_transaction_type_down.sql
-- Description: Down migration for adding 'launch_campaign' to transaction_type_enum
 
-- DOWN MIGRATION (no-op)
-- PostgreSQL does not support removing enum values safely.
-- To rollback, you would need to recreate the enum type and dependent columns, which is unsafe.
-- Leaving as no-op intentionally. 