-- Migration: 0010_add_customer_uuid_and_agency_id_down.sql
-- Description: Remove UUID and agency_referer_code columns from customers table

-- DOWN MIGRATION
-- Drop unique constraints
ALTER TABLE customers DROP CONSTRAINT IF EXISTS uk_customers_agency_referer_code;
ALTER TABLE customers DROP CONSTRAINT IF EXISTS uk_customers_uuid;

-- Drop indexes
DROP INDEX IF EXISTS idx_customers_agency_referer_code;
DROP INDEX IF EXISTS idx_customers_uuid;

-- Drop columns
ALTER TABLE customers DROP COLUMN IF EXISTS agency_referer_code;
ALTER TABLE customers DROP COLUMN IF EXISTS uuid; 