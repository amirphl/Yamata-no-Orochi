-- Migration: 0002_create_customers.sql
-- Description: Create customers table for individuals and companies

-- UP MIGRATION
CREATE TABLE customers (
    id SERIAL PRIMARY KEY,
    
    -- Account type (references account_types table)
    account_type_id INTEGER NOT NULL REFERENCES account_types(id),
    
    -- Company fields (required for independent_company and marketing_agency)
    company_name VARCHAR(60),
    national_id CHAR(11), -- 11 digits exactly
    company_phone VARCHAR(20), -- Various phone formats
    company_address VARCHAR(255),
    postal_code CHAR(10), -- 10 digits exactly
    
    -- Representative/Individual fields (required for all types)
    representative_first_name VARCHAR(255) NOT NULL,
    representative_last_name VARCHAR(255) NOT NULL,
    representative_mobile VARCHAR(15) NOT NULL UNIQUE, -- Format: +989xxxxxxxxx

    -- Common fields (required for all types)
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    
    -- Agency relationship (optional for individuals and independent companies)
    referrer_agency_id INTEGER REFERENCES customers(id),
    
    -- Status and verification
    is_email_verified BOOLEAN DEFAULT FALSE,
    is_mobile_verified BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    email_verified_at TIMESTAMP WITH TIME ZONE,
    mobile_verified_at TIMESTAMP WITH TIME ZONE,
    last_login_at TIMESTAMP WITH TIME ZONE,
    
    -- Constraints
    CONSTRAINT chk_company_fields_for_business CHECK (
        (account_type_id = (SELECT id FROM account_types WHERE type_name = 'individual') AND 
         company_name IS NULL AND national_id IS NULL AND company_phone IS NULL AND 
         company_address IS NULL AND postal_code IS NULL) OR
        (account_type_id IN (SELECT id FROM account_types WHERE type_name IN ('independent_company', 'marketing_agency')) AND 
         company_name IS NOT NULL AND national_id IS NOT NULL AND company_phone IS NOT NULL AND 
         company_address IS NOT NULL AND postal_code IS NOT NULL)
    ),
    
    CONSTRAINT chk_agency_referrer CHECK (
        referrer_agency_id IS NULL OR 
        referrer_agency_id IN (SELECT id FROM customers WHERE account_type_id = (SELECT id FROM account_types WHERE type_name = 'marketing_agency'))
    ),
    
    CONSTRAINT chk_representative_first_name_format CHECK (representative_first_name ~ '^[A-Za-z\s]+$'),
    CONSTRAINT chk_representative_last_name_format CHECK (representative_last_name ~ '^[A-Za-z\s]+$'),
    CONSTRAINT chk_representative_mobile_format CHECK (representative_mobile ~ '^\+989[0-9]{9}$'),
    CONSTRAINT chk_company_phone_format CHECK (company_phone IS NULL OR length(company_phone) >= 10),
    CONSTRAINT chk_national_id_format CHECK (national_id IS NULL OR national_id ~ '^[0-9]{11}$'),
    CONSTRAINT chk_postal_code_format CHECK (postal_code IS NULL OR postal_code ~ '^[0-9]{10}$'),
    CONSTRAINT chk_email_format CHECK (email ~ '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$')
);

-- Create indexes for performance
CREATE INDEX idx_customers_account_type_id ON customers(account_type_id);
CREATE INDEX idx_customers_email ON customers(email);
CREATE INDEX idx_customers_representative_mobile ON customers(representative_mobile);
CREATE INDEX idx_customers_national_id ON customers(national_id) WHERE national_id IS NOT NULL;
CREATE INDEX idx_customers_referrer_agency_id ON customers(referrer_agency_id) WHERE referrer_agency_id IS NOT NULL;
CREATE INDEX idx_customers_company_name ON customers(company_name) WHERE company_name IS NOT NULL;
CREATE INDEX idx_customers_is_active ON customers(is_active);
CREATE INDEX idx_customers_created_at ON customers(created_at);
CREATE INDEX idx_customers_last_login_at ON customers(last_login_at) WHERE last_login_at IS NOT NULL;

-- Create composite indexes for common queries
CREATE INDEX idx_customers_type_active ON customers(account_type_id, is_active);
CREATE INDEX idx_customers_agency_active ON customers(referrer_agency_id, is_active) WHERE referrer_agency_id IS NOT NULL; 