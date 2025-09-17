-- Migration: 0023_add_credit_balance_to_balance_snapshots.sql
-- Description: Add credit_balance column to balance_snapshots

-- UP MIGRATION
BEGIN;

ALTER TABLE balance_snapshots
	ADD COLUMN IF NOT EXISTS credit_balance BIGINT NOT NULL DEFAULT 0;

COMMIT; 