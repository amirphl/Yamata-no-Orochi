-- Migration: 0010_add_customer_uuid_and_agency_id.sql
-- Description: Add UUID and agency_referer_code columns to customers table for agency referral functionality

-- UP MIGRATION
-- Enable UUID extension if not already enabled
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Add UUID column to customers table
ALTER TABLE customers ADD COLUMN uuid UUID;

-- Add agency_referer_code column to customers table
ALTER TABLE customers ADD COLUMN agency_referer_code BIGINT;

-- Create index for UUID lookups
CREATE INDEX idx_customers_uuid ON customers(uuid);

-- Create index for agency_referer_code lookups
CREATE INDEX idx_customers_agency_referer_code ON customers(agency_referer_code);

-- Update existing records to have UUIDs and random 10-digit agency_referer_codes
UPDATE customers SET 
    uuid = uuid_generate_v4(),
    agency_referer_code = FLOOR(RANDOM() * 9000000000) + 1000000000
WHERE uuid IS NULL;

-- Make UUID NOT NULL after populating
ALTER TABLE customers ALTER COLUMN uuid SET NOT NULL;

-- Add unique constraint on UUID
ALTER TABLE customers ADD CONSTRAINT uk_customers_uuid UNIQUE (uuid);

-- Add unique constraint on agency_referer_code
ALTER TABLE customers ADD CONSTRAINT uk_customers_agency_referer_code UNIQUE (agency_referer_code); 