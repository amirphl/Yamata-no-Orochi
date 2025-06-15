-- Migration: 0024_create_system_company_and_wallet_down.sql
-- Description: Remove system company customer and its system wallet

-- DOWN MIGRATION
BEGIN;

-- Delete system wallet snapshots
DELETE FROM balance_snapshots WHERE wallet_id IN (
    SELECT id FROM wallets WHERE uuid = 'b5b35e36-c873-40cd-8025-f7ea22b50bb2'::uuid
);

-- Delete system wallet
DELETE FROM wallets WHERE uuid = 'b5b35e36-c873-40cd-8025-f7ea22b50bb2'::uuid;

-- Delete system customer
DELETE FROM customers WHERE email = 'system@yamata-no-orochi.com';

COMMIT; 