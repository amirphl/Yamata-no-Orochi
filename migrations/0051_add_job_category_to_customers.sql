-- Migration: 0051_add_job_category_to_customers.sql
-- Description: Add job and category columns to customers table

-- UP MIGRATION
ALTER TABLE customers ADD COLUMN job VARCHAR(255);
ALTER TABLE customers ADD COLUMN category VARCHAR(255); 