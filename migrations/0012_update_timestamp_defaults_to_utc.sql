-- Migration: 0012_update_timestamp_defaults_to_utc.sql
-- Description: Update all timestamp defaults to use UTC time instead of server timezone

-- UP MIGRATION

-- Update account_types table
ALTER TABLE account_types 
    ALTER COLUMN created_at SET DEFAULT (now() AT TIME ZONE 'UTC'),
    ALTER COLUMN updated_at SET DEFAULT (now() AT TIME ZONE 'UTC');

-- Update customers table
ALTER TABLE customers 
    ALTER COLUMN created_at SET DEFAULT (now() AT TIME ZONE 'UTC'),
    ALTER COLUMN updated_at SET DEFAULT (now() AT TIME ZONE 'UTC');

-- Update otp_verifications table
ALTER TABLE otp_verifications 
    ALTER COLUMN created_at SET DEFAULT (now() AT TIME ZONE 'UTC');

-- Update customer_sessions table
ALTER TABLE customer_sessions 
    ALTER COLUMN created_at SET DEFAULT (now() AT TIME ZONE 'UTC'),
    ALTER COLUMN last_accessed_at SET DEFAULT (now() AT TIME ZONE 'UTC');

-- Update audit_log table
ALTER TABLE audit_log 
    ALTER COLUMN created_at SET DEFAULT (now() AT TIME ZONE 'UTC'); 