-- Migration: 0013_relax_name_validation_down.sql
-- Description: Down migration for relaxing name validation

-- Re-add the name format constraints (restore original behavior)
ALTER TABLE customers ADD CONSTRAINT chk_representative_first_name_format CHECK (representative_first_name ~ '^[A-Za-z\s]+$');
ALTER TABLE customers ADD CONSTRAINT chk_representative_last_name_format CHECK (representative_last_name ~ '^[A-Za-z\s]+$'); 