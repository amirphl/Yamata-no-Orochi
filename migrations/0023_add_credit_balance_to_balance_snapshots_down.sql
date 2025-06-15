-- Migration: 0023_add_credit_balance_to_balance_snapshots_down.sql
-- Description: Remove credit_balance column from balance_snapshots

-- DOWN MIGRATION
BEGIN;

ALTER TABLE balance_snapshots
	DROP COLUMN IF EXISTS credit_balance;

COMMIT; 