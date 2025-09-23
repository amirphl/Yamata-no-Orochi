-- Migration: 0031_remove_hardcoded_system_tax_data.sql
-- Description: Remove hardcoded system/tax users and wallets to be managed at runtime via config

-- UP MIGRATION
BEGIN;

-- Remove tax wallet and its owner
-- First delete transactions referencing the tax wallet or its customer
DELETE FROM transactions WHERE wallet_id IN (
    SELECT id FROM wallets WHERE uuid = '2672a1bf-b344-4d84-adee-5b92307a2e7c'::uuid
) OR customer_id IN (
    SELECT id FROM customers WHERE email = 'tax@system.yamata-no-orochi.com'
);
DELETE FROM balance_snapshots WHERE wallet_id IN (
    SELECT id FROM wallets WHERE uuid = '2672a1bf-b344-4d84-adee-5b92307a2e7c'::uuid
);
DELETE FROM wallets WHERE uuid = '2672a1bf-b344-4d84-adee-5b92307a2e7c'::uuid;
DELETE FROM customers WHERE email = 'tax@system.yamata-no-orochi.com';

-- Remove system wallet and its owner
-- First delete transactions referencing the system wallet or its customer
DELETE FROM transactions WHERE wallet_id IN (
    SELECT id FROM wallets WHERE uuid = 'b5b35e36-c873-40cd-8025-f7ea22b50bb2'::uuid
) OR customer_id IN (
    SELECT id FROM customers WHERE email = 'system@yamata-no-orochi.com'
);
DELETE FROM balance_snapshots WHERE wallet_id IN (
    SELECT id FROM wallets WHERE uuid = 'b5b35e36-c873-40cd-8025-f7ea22b50bb2'::uuid
);
DELETE FROM wallets WHERE uuid = 'b5b35e36-c873-40cd-8025-f7ea22b50bb2'::uuid;
DELETE FROM customers WHERE email = 'system@yamata-no-orochi.com';

COMMIT; 