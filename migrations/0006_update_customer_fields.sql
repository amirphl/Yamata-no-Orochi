-- Migration: 0006_update_customer_fields.sql
-- Description: Update customer table field sizes and constraints

-- UP MIGRATION

-- Drop existing constraints first
ALTER TABLE customers DROP CONSTRAINT IF EXISTS chk_representative_mobile_format;
ALTER TABLE customers DROP CONSTRAINT IF EXISTS chk_company_phone_format;

-- Alter column sizes
ALTER TABLE customers ALTER COLUMN representative_first_name TYPE VARCHAR(255);
ALTER TABLE customers ALTER COLUMN representative_last_name TYPE VARCHAR(255);
ALTER TABLE customers ALTER COLUMN representative_mobile TYPE VARCHAR(15);
ALTER TABLE customers ALTER COLUMN company_phone TYPE VARCHAR(20);
ALTER TABLE customers ALTER COLUMN company_address TYPE VARCHAR(255);

-- Add updated constraints
ALTER TABLE customers ADD CONSTRAINT chk_representative_mobile_format 
    CHECK (representative_mobile ~ '^\+989[0-9]{9}$');

ALTER TABLE customers ADD CONSTRAINT chk_company_phone_format 
    CHECK (company_phone IS NULL OR length(company_phone) >= 10);