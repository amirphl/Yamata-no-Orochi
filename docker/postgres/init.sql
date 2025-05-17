-- Database initialization for Yamata no Orochi
-- This script runs when the PostgreSQL container starts for the first time

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Create application user with limited privileges
-- Note: The main application user is created by PostgreSQL with environment variables
-- This script creates additional roles if needed

-- Create audit schema for enhanced security logging
CREATE SCHEMA IF NOT EXISTS audit;

-- Grant necessary permissions to the main application user
GRANT CONNECT ON DATABASE yamata_no_orochi TO yamata_user;
GRANT USAGE ON SCHEMA public TO yamata_user;
GRANT CREATE ON SCHEMA public TO yamata_user;
GRANT USAGE ON SCHEMA audit TO yamata_user;

-- Set default privileges for future tables
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO yamata_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO yamata_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA audit GRANT SELECT, INSERT ON TABLES TO yamata_user;

-- Create function for updating updated_at timestamps
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Performance and monitoring views
CREATE OR REPLACE VIEW pg_stat_statements_top AS
SELECT 
    query,
    calls,
    total_time,
    mean_time,
    stddev_time,
    rows,
    100.0 * shared_blks_hit / nullif(shared_blks_hit + shared_blks_read, 0) AS hit_percent
FROM pg_stat_statements 
ORDER BY total_time DESC 
LIMIT 20;

-- Database settings for production
ALTER SYSTEM SET shared_preload_libraries = 'pg_stat_statements';
ALTER SYSTEM SET pg_stat_statements.track = 'all';
ALTER SYSTEM SET pg_stat_statements.max = 10000;

-- Configure connection pooling parameters
ALTER SYSTEM SET max_prepared_transactions = 0;
ALTER SYSTEM SET max_locks_per_transaction = 256;

-- Set timezone
SET timezone = 'UTC'; 