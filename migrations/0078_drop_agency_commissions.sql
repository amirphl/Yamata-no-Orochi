-- Migration: 0078_drop_agency_commissions.sql
-- Description: Drop agency_commissions and commission_rates tables and related enums

BEGIN;
-- Drop dependent tables if they exist
DROP TABLE IF EXISTS agency_commissions CASCADE;
DROP TABLE IF EXISTS commission_rates CASCADE;

-- Drop enums
DROP TYPE IF EXISTS commission_status_enum;
DROP TYPE IF EXISTS commission_type_enum;
COMMIT;
