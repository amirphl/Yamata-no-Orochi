-- Migration: 0026_add_index_on_transactions_metadata_agency_discount_id_down.sql
-- Description: Drop expression index on transactions.metadata for agency_discount_id

-- DOWN MIGRATION
BEGIN;

DROP INDEX IF EXISTS idx_transactions_metadata_agency_discount_id;

COMMIT; 