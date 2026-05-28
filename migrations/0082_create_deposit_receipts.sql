-- Migration: 0082_create_deposit_receipts.sql
-- Description: Create table for offline deposit receipts (manual bank deposit uploads).

BEGIN;

CREATE TABLE IF NOT EXISTS deposit_receipts (
    id              SERIAL PRIMARY KEY,
    uuid            UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    customer_id     INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    amount          BIGINT NOT NULL,
    currency        VARCHAR(3) NOT NULL DEFAULT 'TMN',
    status          VARCHAR(16) NOT NULL DEFAULT 'pending',
    status_reason   TEXT,
    reviewer_id     INTEGER,
    rejection_note  TEXT,

    file_name       VARCHAR(255) NOT NULL,
    content_type    VARCHAR(120) NOT NULL,
    file_size       BIGINT NOT NULL,
    file_data       BYTEA NOT NULL,

    lang            VARCHAR(2) NOT NULL DEFAULT 'EN',
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_deposit_receipts_customer_id ON deposit_receipts(customer_id);
CREATE INDEX IF NOT EXISTS idx_deposit_receipts_status ON deposit_receipts(status);
CREATE INDEX IF NOT EXISTS idx_deposit_receipts_lang ON deposit_receipts(lang);
CREATE INDEX IF NOT EXISTS idx_deposit_receipts_created_at ON deposit_receipts(created_at);

COMMIT;
