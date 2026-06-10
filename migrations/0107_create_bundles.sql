-- Migration: Create bundles table
-- Up migration

BEGIN;

CREATE TABLE IF NOT EXISTS bundles (
    id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    objective VARCHAR(1023) NOT NULL DEFAULT '',
    target_audience_persona VARCHAR(1023) NOT NULL DEFAULT '',
    adlink VARCHAR(2047) NULL,
    description VARCHAR(2047) NULL,
    short_link_domain VARCHAR(255) NULL,
    target_customer_name VARCHAR(255) NULL,
    category VARCHAR(255) NULL,
    job VARCHAR(255) NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    statistics JSONB NOT NULL DEFAULT '{}'::jsonb,
    customer_id INTEGER NOT NULL REFERENCES customers(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    CONSTRAINT chk_bundles_title_not_blank CHECK (length(btrim(title)) > 0)
);

COMMENT ON TABLE bundles IS 'Bundle records linked to customers';
COMMENT ON COLUMN bundles.metadata IS 'Arbitrary bundle metadata stored as JSON';
COMMENT ON COLUMN bundles.statistics IS 'Arbitrary bundle statistics stored as JSON';

COMMIT;
