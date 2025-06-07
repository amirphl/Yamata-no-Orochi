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

-- Database settings for production
-- Note: pg_stat_statements extension is loaded via shared_preload_libraries in postgresql.conf
ALTER SYSTEM SET pg_stat_statements.track = 'all';
ALTER SYSTEM SET pg_stat_statements.max = 10000;

-- Configure connection pooling parameters
ALTER SYSTEM SET max_prepared_transactions = 0;
ALTER SYSTEM SET max_locks_per_transaction = 256;

-- Set timezone
SET timezone = 'UTC'; 