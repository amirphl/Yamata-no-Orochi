-- Migration: 0020_create_tax_wallet_down.sql
-- Description: Remove tax wallet, balance snapshot, and system tax user

-- Remove balance snapshot first (due to foreign key constraint)
DELETE FROM balance_snapshots 
WHERE wallet_id = (SELECT id FROM wallets WHERE uuid = '2672a1bf-b344-4d84-adee-5b92307a2e7c'::uuid);

-- Remove the tax wallet
DELETE FROM wallets 
WHERE uuid = '2672a1bf-b344-4d84-adee-5b92307a2e7c'::uuid;

-- Remove the system tax user
DELETE FROM customers 
WHERE email = 'tax@system.yamata-no-orochi.com'; 