-- Migration: 0009_add_correlation_ids.sql
-- Description: Add correlation_id fields to immutable tables for tracking related records

-- UP MIGRATION
-- Enable UUID extension if not already enabled
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Add correlation_id to otp_verifications table
ALTER TABLE otp_verifications ADD COLUMN correlation_id UUID;

-- Create index for correlation_id lookups
CREATE INDEX idx_otp_correlation_id ON otp_verifications(correlation_id);

-- Add correlation_id to customer_sessions table
ALTER TABLE customer_sessions ADD COLUMN correlation_id UUID;

-- Create index for correlation_id lookups
CREATE INDEX idx_sessions_correlation_id ON customer_sessions(correlation_id);

-- Update existing records to have their own correlation_id (generate new UUIDs)
UPDATE otp_verifications SET correlation_id = uuid_generate_v4() WHERE correlation_id IS NULL;
UPDATE customer_sessions SET correlation_id = uuid_generate_v4() WHERE correlation_id IS NULL;

-- Make correlation_id NOT NULL after populating (only if no NULL values exist)
DO $$
BEGIN
    -- Check if all records have correlation_id
    IF NOT EXISTS (SELECT 1 FROM otp_verifications WHERE correlation_id IS NULL) THEN
        ALTER TABLE otp_verifications ALTER COLUMN correlation_id SET NOT NULL;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM customer_sessions WHERE correlation_id IS NULL) THEN
        ALTER TABLE customer_sessions ALTER COLUMN correlation_id SET NOT NULL;
    END IF;
END $$;