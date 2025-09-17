-- Migration: 0025_add_indexes_on_transactions_metadata.sql
-- Description: Add expression indexes on transactions.metadata for customer_id and source used in reporting joins/filters

-- UP MIGRATION
BEGIN;

-- Index to speed up join on (t.metadata->>'customer_id')::int
CREATE INDEX IF NOT EXISTS idx_transactions_metadata_customer_id_int
ON transactions (((metadata->>'customer_id')::int));

-- Index to speed up filter on t.metadata->>'source'
CREATE INDEX IF NOT EXISTS idx_transactions_metadata_source
ON transactions ((metadata->>'source'));

COMMIT; 