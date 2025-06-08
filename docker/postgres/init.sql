-- Database initialization for Yamata no Orochi
-- This script runs when the PostgreSQL container starts for the first time

-- Enable required extensions in the postgres database
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

-- Create the target database if it doesn't exist
-- Note: This runs in the postgres database context
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = '${DB_NAME:-yamata_no_orochi}') THEN
        CREATE DATABASE "${DB_NAME:-yamata_no_orochi}";
    END IF;
END $$;

-- Grant necessary permissions to the main application user
GRANT CONNECT ON DATABASE "${DB_NAME:-yamata_no_orochi}" TO "${DB_USER:-yamata_user}";

-- Database settings for production
-- Note: pg_stat_statements extension is loaded via shared_preload_libraries in postgresql.conf
ALTER SYSTEM SET pg_stat_statements.track = 'all';
ALTER SYSTEM SET pg_stat_statements.max = 10000;

-- Configure connection pooling parameters
ALTER SYSTEM SET max_prepared_transactions = 0;
ALTER SYSTEM SET max_locks_per_transaction = 256;

-- Set timezone
SET timezone = 'UTC'; 