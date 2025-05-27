-- Migration: 0012_update_timestamp_defaults_to_utc_down.sql
-- Description: Revert timestamp defaults to server timezone

-- DOWN MIGRATION (revert to server timezone)
-- Update account_types table
ALTER TABLE account_types 
    ALTER COLUMN created_at SET DEFAULT CURRENT_TIMESTAMP,
    ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP;

-- Update customers table
ALTER TABLE customers 
    ALTER COLUMN created_at SET DEFAULT CURRENT_TIMESTAMP,
    ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP;

-- Update otp_verifications table
ALTER TABLE otp_verifications 
    ALTER COLUMN created_at SET DEFAULT CURRENT_TIMESTAMP;

-- Update customer_sessions table
ALTER TABLE customer_sessions 
    ALTER COLUMN created_at SET DEFAULT CURRENT_TIMESTAMP,
    ALTER COLUMN last_accessed_at SET DEFAULT CURRENT_TIMESTAMP;

-- Update audit_log table
ALTER TABLE audit_log 
    ALTER COLUMN created_at SET DEFAULT CURRENT_TIMESTAMP; 