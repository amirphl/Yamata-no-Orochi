-- Migration: 0029_create_admins.sql
-- Description: Create admins table for platform administrators

-- UP MIGRATION

-- Ensure pgcrypto is available for gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS admins (
    id SERIAL PRIMARY KEY,
    uuid UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    username VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMP WITH TIME ZONE
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_admins_uuid ON admins(uuid);
CREATE INDEX IF NOT EXISTS idx_admins_username ON admins(username);
CREATE INDEX IF NOT EXISTS idx_admins_is_active ON admins(is_active);
CREATE INDEX IF NOT EXISTS idx_admins_created_at ON admins(created_at);
CREATE INDEX IF NOT EXISTS idx_admins_last_login_at ON admins(last_login_at) WHERE last_login_at IS NOT NULL; 