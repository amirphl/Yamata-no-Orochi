-- Migration: 0034_create_audience_profiles.sql
-- Description: Create audience_profiles table for campaign audience targeting

-- UP MIGRATION

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS audience_profiles (
    id BIGSERIAL PRIMARY KEY,
    uid VARCHAR(255) NOT NULL UNIQUE,
    phone_number VARCHAR(20) UNIQUE,
    tag INTEGER[] NOT NULL DEFAULT '{}',
    color VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audience_profiles_uid ON audience_profiles(uid);
CREATE INDEX IF NOT EXISTS idx_audience_profiles_phone_number ON audience_profiles(phone_number);
CREATE INDEX IF NOT EXISTS idx_audience_profiles_created_at ON audience_profiles(created_at);
-- GIN indexes for array contains queries
CREATE INDEX IF NOT EXISTS idx_audience_profiles_tag_gin ON audience_profiles USING GIN (tag);
CREATE INDEX IF NOT EXISTS idx_audience_profiles_color ON audience_profiles(color); 