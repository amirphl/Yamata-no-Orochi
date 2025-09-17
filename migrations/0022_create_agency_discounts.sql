-- Migration: 0022_create_agency_discounts.sql
-- Description: Create table to track per-agency customer discounts with optional expiration

-- UP MIGRATION
BEGIN;

CREATE TABLE IF NOT EXISTS agency_discounts (
    id SERIAL PRIMARY KEY,
    uuid UUID NOT NULL UNIQUE,
    agency_id INTEGER NOT NULL REFERENCES customers(id),
    customer_id INTEGER NOT NULL REFERENCES customers(id),
    discount_rate NUMERIC(5,4) NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE,
    reason VARCHAR(255),
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    CONSTRAINT chk_discount_rate_range CHECK (discount_rate >= 0 AND discount_rate <= 0.5)
);

-- Indexes for faster lookup
CREATE INDEX IF NOT EXISTS idx_agency_discounts_agency_id ON agency_discounts(agency_id);
CREATE INDEX IF NOT EXISTS idx_agency_discounts_customer_id ON agency_discounts(customer_id);
CREATE INDEX IF NOT EXISTS idx_agency_discounts_expires_at ON agency_discounts(expires_at);

-- Ensure only one active (non-expired) discount per agency-customer pair
CREATE UNIQUE INDEX IF NOT EXISTS uk_agency_discounts_agency_customer_active
ON agency_discounts(agency_id, customer_id)
WHERE expires_at IS NULL;

COMMIT; 