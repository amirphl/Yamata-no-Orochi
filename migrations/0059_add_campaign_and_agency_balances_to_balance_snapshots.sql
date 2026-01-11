-- Migration: 0059_add_campaign_and_agency_balances_to_balance_snapshots.sql
-- Description: Add spent_on_campaign and agency_share_with_tax columns to balance_snapshots

-- UP MIGRATION
BEGIN;

ALTER TABLE balance_snapshots
	ADD COLUMN IF NOT EXISTS spent_on_campaign BIGINT NOT NULL DEFAULT 0,
	ADD COLUMN IF NOT EXISTS agency_share_with_tax BIGINT NOT NULL DEFAULT 0;

COMMIT;
