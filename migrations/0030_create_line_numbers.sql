-- Migration: 0030_create_line_numbers.sql
-- Description: Create line_numbers table for messaging/campaign pricing and selection

-- UP MIGRATION

-- Ensure pgcrypto for UUID generation
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS line_numbers (
    id SERIAL PRIMARY KEY,
    uuid UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),

    name VARCHAR(255),
    line_number VARCHAR(50) NOT NULL UNIQUE,
    price_factor NUMERIC(10,4) NOT NULL,
    priority INTEGER,

    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_line_numbers_uuid ON line_numbers(uuid);
CREATE INDEX IF NOT EXISTS idx_line_numbers_is_active ON line_numbers(is_active);
CREATE INDEX IF NOT EXISTS idx_line_numbers_created_at ON line_numbers(created_at);
CREATE INDEX IF NOT EXISTS idx_line_numbers_priority ON line_numbers(priority);
CREATE INDEX IF NOT EXISTS idx_line_numbers_value ON line_numbers(line_number); 