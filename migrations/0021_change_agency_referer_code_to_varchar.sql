-- Migration: 0021_change_agency_referer_code_to_varchar.sql
-- Description: Change agency_referer_code type from BIGINT to VARCHAR(255)

-- UP MIGRATION
BEGIN;

-- Drop dependent index/constraint if type change requires (Postgres supports USING for type change)

-- Alter column type with USING clause to preserve data
ALTER TABLE customers
	ALTER COLUMN agency_referer_code TYPE VARCHAR(255)
	USING agency_referer_code::text;

-- Ensure uniqueness constraint and index remain; recreate if missing
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