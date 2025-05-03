-- Migration: 0004_create_customer_sessions.sql
-- Description: Create customer sessions table for authentication tokens

-- UP MIGRATION
CREATE TABLE customer_sessions (
    id SERIAL PRIMARY KEY,
    
    -- Customer reference
    customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    
    -- Session details
    session_token VARCHAR(255) NOT NULL UNIQUE,
    refresh_token VARCHAR(255) UNIQUE,
    
    -- Device and location info
    device_info JSONB,
    ip_address INET,
    user_agent TEXT,
    
    -- Session status
    is_active BOOLEAN DEFAULT TRUE,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_accessed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    
    CONSTRAINT chk_session_token_length CHECK (length(session_token) >= 32),
    CONSTRAINT chk_refresh_token_length CHECK (refresh_token IS NULL OR length(refresh_token) >= 32),
    CONSTRAINT chk_expires_future CHECK (expires_at > created_at)
);

-- Create indexes for performance
CREATE INDEX idx_sessions_customer_id ON customer_sessions(customer_id);
CREATE INDEX idx_sessions_session_token ON customer_sessions(session_token);
CREATE INDEX idx_sessions_refresh_token ON customer_sessions(refresh_token) WHERE refresh_token IS NOT NULL;
CREATE INDEX idx_sessions_is_active ON customer_sessions(is_active);
CREATE INDEX idx_sessions_expires_at ON customer_sessions(expires_at);
CREATE INDEX idx_sessions_last_accessed ON customer_sessions(last_accessed_at);
CREATE INDEX idx_sessions_ip_address ON customer_sessions(ip_address) WHERE ip_address IS NOT NULL;

-- Create composite indexes for common queries
CREATE INDEX idx_sessions_customer_active ON customer_sessions(customer_id, is_active);
CREATE INDEX idx_sessions_active_not_expired ON customer_sessions(is_active, expires_at) 
    WHERE is_active = TRUE AND expires_at > CURRENT_TIMESTAMP; 