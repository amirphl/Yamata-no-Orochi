-- Migration: 0005_create_audit_log.sql
-- Description: Create audit log table for tracking customer actions

-- UP MIGRATION
CREATE TYPE audit_action_enum AS ENUM (
    'signup_initiated',
    'signup_completed',
    'email_verified',
    'mobile_verified',
    'login_success',
    'login_failed',
    'logout',
    'password_changed',
    'profile_updated',
    'account_activated',
    'account_deactivated',
    'session_created',
    'session_expired',
    'otp_generated',
    'otp_verified',
    'otp_failed'
);

CREATE TABLE audit_log (
    id SERIAL PRIMARY KEY,
    
    -- Customer reference (nullable for system actions)
    customer_id INTEGER REFERENCES customers(id) ON DELETE SET NULL,
    
    -- Action details
    action audit_action_enum NOT NULL,
    description TEXT,
    
    -- Request context
    ip_address INET,
    user_agent TEXT,
    request_id VARCHAR(255), -- For correlating with application logs
    
    -- Additional data (JSON for flexibility)
    metadata JSONB,
    
    -- Result
    success BOOLEAN DEFAULT TRUE,
    error_message TEXT,
    
    -- Timestamp
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT chk_error_when_not_success CHECK (
        (success = TRUE AND error_message IS NULL) OR
        (success = FALSE AND error_message IS NOT NULL)
    )
);

-- Create indexes for performance and querying
CREATE INDEX idx_audit_customer_id ON audit_log(customer_id) WHERE customer_id IS NOT NULL;
CREATE INDEX idx_audit_action ON audit_log(action);
CREATE INDEX idx_audit_created_at ON audit_log(created_at);
CREATE INDEX idx_audit_success ON audit_log(success);
CREATE INDEX idx_audit_ip_address ON audit_log(ip_address) WHERE ip_address IS NOT NULL;
CREATE INDEX idx_audit_request_id ON audit_log(request_id) WHERE request_id IS NOT NULL;

-- Create composite indexes for common queries
CREATE INDEX idx_audit_customer_action ON audit_log(customer_id, action) WHERE customer_id IS NOT NULL;
CREATE INDEX idx_audit_customer_date ON audit_log(customer_id, created_at) WHERE customer_id IS NOT NULL;
CREATE INDEX idx_audit_action_date ON audit_log(action, created_at);
CREATE INDEX idx_audit_failed_actions ON audit_log(success, action, created_at) WHERE success = FALSE;

-- Create GIN index for metadata JSONB queries
CREATE INDEX idx_audit_metadata ON audit_log USING GIN (metadata) WHERE metadata IS NOT NULL; 