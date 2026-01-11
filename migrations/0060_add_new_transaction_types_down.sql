-- Migration: 0060_add_new_transaction_types_down.sql
-- Description: Down migration for adding new transaction types to transaction_type_enum

-- DOWN MIGRATION (no-op)
-- Removing enum values is not supported safely in PostgreSQL without recreating dependent objects.
-- If rollback is needed, recreate the enum and dependent columns manually.
