-- Rollback migration: Drop agency commission and commission rate models

-- Drop tables in reverse order (due to foreign key constraints)
DROP TABLE IF EXISTS agency_commissions;
DROP TABLE IF EXISTS commission_rates;

-- Drop enums
DROP TYPE IF EXISTS commission_type_enum;
DROP TYPE IF EXISTS commission_status_enum; 