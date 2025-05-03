-- Migration: 0003_create_otp_verifications.sql
-- Description: Create OTP verification table for signup flow

-- UP MIGRATION
CREATE TYPE otp_type_enum AS ENUM ('mobile', 'email');
CREATE TYPE otp_status_enum AS ENUM ('pending', 'verified', 'expired', 'failed');

CREATE TABLE otp_verifications (
    id SERIAL PRIMARY KEY,
    
    -- Customer reference
    customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    
    -- OTP details
    otp_code CHAR(6) NOT NULL, -- 6-digit OTP
    otp_type otp_type_enum NOT NULL,
    
    -- Target (mobile number or email)
    target_value VARCHAR(255) NOT NULL, -- The mobile number or email being verified
    
    -- Status and attempts
    status otp_status_enum DEFAULT 'pending',
    attempts_count INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    verified_at TIMESTAMP WITH TIME ZONE,
    
    -- Additional security
    ip_address INET,
    user_agent TEXT,
    
    CONSTRAINT chk_otp_code_format CHECK (otp_code ~ '^[0-9]{6}$'),
    CONSTRAINT chk_attempts_limit CHECK (attempts_count <= max_attempts),
    CONSTRAINT chk_verified_status CHECK (
        (status = 'verified' AND verified_at IS NOT NULL) OR
        (status != 'verified' AND verified_at IS NULL)
    )
);

-- Create indexes for performance and security
CREATE INDEX idx_otp_customer_id ON otp_verifications(customer_id);
CREATE INDEX idx_otp_code_target ON otp_verifications(otp_code, target_value);
CREATE INDEX idx_otp_type_status ON otp_verifications(otp_type, status);
CREATE INDEX idx_otp_created_at ON otp_verifications(created_at);
CREATE INDEX idx_otp_expires_at ON otp_verifications(expires_at);
CREATE INDEX idx_otp_ip_address ON otp_verifications(ip_address) WHERE ip_address IS NOT NULL;

-- Create composite indexes for common queries
CREATE INDEX idx_otp_customer_type_status ON otp_verifications(customer_id, otp_type, status);
CREATE INDEX idx_otp_active_codes ON otp_verifications(target_value, otp_type, status) 
    WHERE status = 'pending' AND expires_at > CURRENT_TIMESTAMP; 