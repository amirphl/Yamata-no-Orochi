-- Migration: 0009_add_correlation_ids_down.sql
-- Description: Remove correlation_id fields from immutable tables

-- DOWN MIGRATION
-- Drop indexes first
DROP INDEX IF EXISTS idx_otp_correlation_id;
DROP INDEX IF EXISTS idx_sessions_correlation_id;

-- Remove correlation_id columns
ALTER TABLE otp_verifications DROP COLUMN IF EXISTS correlation_id;
ALTER TABLE customer_sessions DROP COLUMN IF EXISTS correlation_id; 