-- Database initialization for Yamata no Orochi
-- This script runs when the PostgreSQL container starts for the first time

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Create pg_stat_statements extension (requires shared_preload_libraries)
-- This will only work if the extension was loaded at server startup
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_available_extensions 
        WHERE name = 'pg_stat_statements' AND installed_version IS NOT NULL
    ) THEN
        CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";
    END IF;
END $$;

-- Create audit schema for enhanced security logging
CREATE SCHEMA IF NOT EXISTS audit;

-- Grant necessary permissions to the main application user
GRANT CONNECT ON DATABASE ${DB_NAME:-yamata_no_orochi} TO ${DB_USER:-yamata_user};
GRANT USAGE ON SCHEMA public TO ${DB_USER:-yamata_user};
GRANT CREATE ON SCHEMA public TO ${DB_USER:-yamata_user};
GRANT USAGE ON SCHEMA audit TO ${DB_USER:-yamata_user};

-- Set default privileges for future tables
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ${DB_USER:-yamata_user};
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO ${DB_USER:-yamata_user};
ALTER DEFAULT PRIVILEGES IN SCHEMA audit GRANT SELECT, INSERT ON TABLES TO ${DB_USER:-yamata_user};

-- Create function for updating updated_at timestamps
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply all migrations
\echo 'Starting database migration...'

\echo 'Running 0001_create_account_types.sql...'
\i /migrations/0001_create_account_types.sql

\echo 'Running 0002_create_customers.sql...'
\i /migrations/0002_create_customers.sql

\echo 'Running 0003_create_otp_verifications.sql...'
\i /migrations/0003_create_otp_verifications.sql

\echo 'Running 0004_create_customer_sessions.sql...'
\i /migrations/0004_create_customer_sessions.sql

\echo 'Running 0005_create_audit_log.sql...'
\i /migrations/0005_create_audit_log.sql

\echo 'Running 0006_update_customer_fields.sql...'
\i /migrations/0006_update_customer_fields.sql

\echo 'Running 0007_add_missing_audit_actions.sql...'
\i /migrations/0007_add_missing_audit_actions.sql

\echo 'Running 0008_update_audit_log_success_field.sql...'
\i /migrations/0008_update_audit_log_success_field.sql

\echo 'Running 0009_add_correlation_ids.sql...'
\i /migrations/0009_add_correlation_ids.sql

\echo 'Running 0010_add_customer_uuid_and_agency_id.sql...'
\i /migrations/0010_add_customer_uuid_and_agency_id.sql

\echo 'Running 0011_add_new_audit_actions.sql...'
\i /migrations/0011_add_new_audit_actions.sql

\echo 'Running 0012_update_timestamp_defaults_to_utc.sql...'
\i /migrations/0012_update_timestamp_defaults_to_utc.sql

\echo 'All migrations completed successfully!'
\echo 'Database schema is now ready for the Yamata no Orochi signup system.'

-- Database settings for production
-- Note: pg_stat_statements extension is loaded via shared_preload_libraries in postgresql.conf
ALTER SYSTEM SET pg_stat_statements.track = 'all';
ALTER SYSTEM SET pg_stat_statements.max = 10000;

-- Configure connection pooling parameters
ALTER SYSTEM SET max_prepared_transactions = 0;
ALTER SYSTEM SET max_locks_per_transaction = 256;

-- Set timezone
SET timezone = 'UTC'; 