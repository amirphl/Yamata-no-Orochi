-- Migration: 0001_create_account_types_down.sql
-- Description: Rollback account types enum and table

-- DOWN MIGRATION
DROP TABLE IF EXISTS account_types CASCADE;
DROP TYPE IF EXISTS account_type_enum CASCADE; 