-- Migration: 0024_create_system_company_and_wallet.sql
-- Description: Create system company customer (referrer code 'jaazebeh.ir'), its system wallet, and initial balance snapshot

-- UP MIGRATION
BEGIN;

-- Create system company customer
INSERT INTO customers (
    uuid,
    account_type_id,
    representative_first_name,
    representative_last_name,
    representative_mobile,
    email,
    password_hash,
    is_email_verified,
    is_mobile_verified,
    is_active,
    agency_referer_code,
    sheba_number,
    created_at,
    updated_at
) VALUES (
    gen_random_uuid(),
    (SELECT id FROM account_types WHERE type_name = 'marketing_agency'),
    'System',
    'Account',
    '+989000000001',
    'system@yamata-no-orochi.com',
    '$2b$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj4/LewdBPj4',
    false,
    false,
    true,
    'jaazebeh.ir',
    NULL,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
) ON CONFLICT DO NOTHING;

-- Create system wallet (empty IBAN by design for scattered settlement system share)
INSERT INTO wallets (uuid, customer_id, metadata, created_at, updated_at)
VALUES (
    'b5b35e36-c873-40cd-8025-f7ea22b50bb2'::uuid,
    (SELECT id FROM customers WHERE email = 'system@yamata-no-orochi.com'),
    '{"type": "system_wallet", "description": "System operational wallet", "created_via": "migration", "owner": "system"}',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
) ON CONFLICT DO NOTHING;

-- Create initial balance snapshot for system wallet (includes credit_balance)
INSERT INTO balance_snapshots (
    uuid,
    correlation_id,
    wallet_id,
    customer_id,
    free_balance,
    frozen_balance,
    locked_balance,
    credit_balance,
    total_balance,
    reason,
    description,
    metadata,
    created_at,
    updated_at
) VALUES (
    gen_random_uuid(),
    gen_random_uuid(),
    (SELECT id FROM wallets WHERE uuid = 'b5b35e36-c873-40cd-8025-f7ea22b50bb2'::uuid),
    (SELECT id FROM customers WHERE email = 'system@yamata-no-orochi.com'),
    0,
    0,
    0,
    0,
    0,
    'initial_balance',
    'Initial balance snapshot for system wallet',
    '{"source": "migration", "wallet_type": "system_wallet", "owner": "system"}',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
);

COMMIT; 