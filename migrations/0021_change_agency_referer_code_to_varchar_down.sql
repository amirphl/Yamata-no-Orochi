-- Migration: 0021_change_agency_referer_code_to_varchar_down.sql
-- Description: Revert agency_referer_code type back to BIGINT

-- DOWN MIGRATION
BEGIN;

-- Convert back to BIGINT using cast; non-numeric values will fail, so ensure only numeric strings exist before running down
ALTER TABLE customers
	ALTER COLUMN agency_referer_code TYPE BIGINT
	USING agency_referer_code::bigint;

-- Ensure unique constraint and index exist after type change
DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint
		WHERE conname = 'uk_customers_agency_referer_code'
	) THEN
		ALTER TABLE customers ADD CONSTRAINT uk_customers_agency_referer_code UNIQUE (agency_referer_code);
	END IF;
END $$;

DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relname = 'idx_customers_agency_referer_code' AND n.nspname = 'public'
	) THEN
		CREATE INDEX idx_customers_agency_referer_code ON customers(agency_referer_code);
	END IF;
END $$;

COMMIT; 