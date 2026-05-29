-- Migration: 0084_add_invoice_number_to_deposit_receipts.sql
-- Description: Add invoice_number column to deposit_receipts

BEGIN;

ALTER TABLE IF EXISTS deposit_receipts
    ADD COLUMN IF NOT EXISTS invoice_number VARCHAR(255) NOT NULL DEFAULT gen_random_uuid()::text;

CREATE UNIQUE INDEX IF NOT EXISTS uk_deposit_receipts_invoice_number ON deposit_receipts(invoice_number);

COMMIT;
