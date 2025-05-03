-- Migration: 0001_create_account_types.sql
-- Description: Create account types enum for user classification

-- UP MIGRATION
CREATE TYPE account_type_enum AS ENUM ('individual', 'independent_company', 'marketing_agency');

-- Create account_types reference table for better data integrity
CREATE TABLE account_types (
    id SERIAL PRIMARY KEY,
    type_name account_type_enum NOT NULL UNIQUE,
    display_name VARCHAR(50) NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Insert default account types
INSERT INTO account_types (type_name, display_name, description) VALUES
('individual', 'Individual', 'Personal individual account'),
('independent_company', 'Independent Company', 'Independent business company account'),
('marketing_agency', 'Marketing Agency', 'Marketing agency account that can manage other companies');

-- Create indexes
CREATE INDEX idx_account_types_type_name ON account_types(type_name);

-- DOWN MIGRATION
-- DROP TABLE account_types;
-- DROP TYPE account_type_enum; 