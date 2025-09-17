-- Migration: 0025_add_indexes_on_transactions_metadata_down.sql
-- Description: Drop expression indexes on transactions.metadata for customer_id and source

-- DOWN MIGRATION
BEGIN;

DROP INDEX IF EXISTS idx_transactions_metadata_customer_id_int;
DROP INDEX IF EXISTS idx_transactions_metadata_source;

COMMIT; 