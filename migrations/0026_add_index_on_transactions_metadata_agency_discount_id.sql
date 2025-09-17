-- Migration: 0026_add_index_on_transactions_metadata_agency_discount_id.sql
-- Description: Add expression index on transactions.metadata for agency_discount_id used in aggregates

-- UP MIGRATION
BEGIN;

CREATE INDEX IF NOT EXISTS idx_transactions_metadata_agency_discount_id
ON transactions (((metadata->>'agency_discount_id')::bigint));

COMMIT; 