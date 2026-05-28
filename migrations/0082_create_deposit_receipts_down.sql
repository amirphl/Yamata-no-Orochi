-- Migration: 0082_create_deposit_receipts_down.sql
-- Description: Drop deposit_receipts table.

BEGIN;
DROP TABLE IF EXISTS deposit_receipts CASCADE;
COMMIT;
