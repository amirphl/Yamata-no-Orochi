-- Migration: 0084_add_invoice_number_to_deposit_receipts_down.sql
-- Description: Remove invoice_number column from deposit_receipts

BEGIN;
ALTER TABLE IF EXISTS deposit_receipts DROP COLUMN IF EXISTS invoice_number;
COMMIT;
