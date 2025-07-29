-- Migration: 0013_relax_name_validation.sql
-- Description: Remove name format constraints to allow any characters including Farsi

-- Remove the name format constraints
ALTER TABLE customers DROP CONSTRAINT IF EXISTS chk_representative_first_name_format;
ALTER TABLE customers DROP CONSTRAINT IF EXISTS chk_representative_last_name_format;