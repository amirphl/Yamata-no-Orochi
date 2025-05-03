-- Migration: 0006_update_customer_fields_down.sql
-- Description: Revert customer table field sizes and constraints

-- DOWN MIGRATION

-- Drop new constraints
ALTER TABLE customers DROP CONSTRAINT IF EXISTS chk_representative_mobile_format;
ALTER TABLE customers DROP CONSTRAINT IF EXISTS chk_company_phone_format;

-- Revert column sizes
ALTER TABLE customers ALTER COLUMN representative_first_name TYPE VARCHAR(30);
ALTER TABLE customers ALTER COLUMN representative_last_name TYPE VARCHAR(30);
ALTER TABLE customers ALTER COLUMN representative_mobile TYPE CHAR(11);
ALTER TABLE customers ALTER COLUMN company_phone TYPE VARCHAR(15);
ALTER TABLE customers ALTER COLUMN company_address TYPE VARCHAR(120);

-- Add original constraints
ALTER TABLE customers ADD CONSTRAINT chk_representative_mobile_format 
    CHECK (representative_mobile ~ '^09[0-9]{9}$');

ALTER TABLE customers ADD CONSTRAINT chk_company_phone_format 
    CHECK (company_phone IS NULL OR company_phone ~ '^(021[0-9]{8}|0[0-9]{10})$'); 