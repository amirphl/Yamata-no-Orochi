-- Migration: 0002_create_customers_down.sql
-- Description: Rollback customers table

-- DOWN MIGRATION
DROP TABLE IF EXISTS customers CASCADE; 