-- Database-specific initialization for Yamata no Orochi
-- This script runs in the context of the target database after it's created

-- Create audit schema for enhanced security logging
CREATE SCHEMA IF NOT EXISTS audit;

-- Grant schema permissions to the application user
GRANT USAGE ON SCHEMA public TO "${DB_USER:-yamata_user}";
GRANT CREATE ON SCHEMA public TO "${DB_USER:-yamata_user}";
GRANT USAGE ON SCHEMA audit TO "${DB_USER:-yamata_user}";

-- Set default privileges for future tables
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "${DB_USER:-yamata_user}";
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO "${DB_USER:-yamata_user}";
ALTER DEFAULT PRIVILEGES IN SCHEMA audit GRANT SELECT, INSERT ON TABLES TO "${DB_USER:-yamata_user}";

-- Create function for updating updated_at timestamps
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Grant execute permission on the function
GRANT EXECUTE ON FUNCTION trigger_set_timestamp() TO "${DB_USER:-yamata_user}"; 