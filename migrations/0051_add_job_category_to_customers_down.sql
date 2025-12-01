-- Migration: 0051_add_job_category_to_customers_down.sql
-- Description: Remove job and category columns from customers table

-- DOWN MIGRATION
ALTER TABLE customers DROP COLUMN IF EXISTS category;
ALTER TABLE customers DROP COLUMN IF EXISTS job; 