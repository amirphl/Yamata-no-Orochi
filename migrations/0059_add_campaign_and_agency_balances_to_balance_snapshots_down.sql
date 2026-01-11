-- Migration: 0059_add_campaign_and_agency_balances_to_balance_snapshots_down.sql
-- Description: Remove spent_on_campaign and agency_share_with_tax columns from balance_snapshots

-- DOWN MIGRATION
BEGIN;

ALTER TABLE balance_snapshots
	DROP COLUMN IF EXISTS spent_on_campaign,
	DROP COLUMN IF EXISTS agency_share_with_tax;

COMMIT;
