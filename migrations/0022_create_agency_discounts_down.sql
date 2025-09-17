-- Migration: 0022_create_agency_discounts_down.sql
-- Description: Drop agency_discounts table and related indexes

-- DOWN MIGRATION
BEGIN;

DROP INDEX IF EXISTS uk_agency_discounts_agency_customer_active;
DROP INDEX IF EXISTS idx_agency_discounts_expires_at;
DROP INDEX IF EXISTS idx_agency_discounts_customer_id;
DROP INDEX IF EXISTS idx_agency_discounts_agency_id;
DROP TABLE IF EXISTS agency_discounts;

COMMIT; 