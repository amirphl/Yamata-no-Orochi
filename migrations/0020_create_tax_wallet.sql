-- Migration: 0020_create_tax_wallet.sql
-- Description: Create system user for tax collection and tax wallet

-- Create system user for tax collection
-- This user will own the tax wallet and receive transaction taxes
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
    created_at,
    updated_at
) VALUES (
    gen_random_uuid(),
    (SELECT id FROM account_types WHERE type_name = 'individual'),
    'Tax',
    'Collector',
    '+989000000000', -- System phone number
    'tax@system.yamata-no-orochi.com', -- System email
    '$2b$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj4/LewdBPj4', -- System password hash (not used for login)
    false, -- Email verified
    false, -- Mobile verified
    false, -- Active
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
);

-- Create tax wallet for collecting transaction taxes
-- This wallet will receive 10% of all payment amounts as tax
INSERT INTO wallets (uuid, customer_id, metadata, created_at, updated_at)
VALUES (
    '2672a1bf-b344-4d84-adee-5b92307a2e7c'::uuid,
    (SELECT id FROM customers WHERE email = 'tax@system.yamata-no-orochi.com'),
    '{"type": "tax_wallet", "description": "System wallet for collecting transaction taxes", "default_tax_rate": 0.10, "created_via": "migration", "owner": "system"}',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
);

-- Create initial balance snapshot for tax wallet
INSERT INTO balance_snapshots (uuid, correlation_id, wallet_id, customer_id, free_balance, frozen_balance, locked_balance, total_balance, reason, description, metadata, created_at, updated_at)
VALUES (
    gen_random_uuid(),
    gen_random_uuid(),
    (SELECT id FROM wallets WHERE uuid = '2672a1bf-b344-4d84-adee-5b92307a2e7c'::uuid),
    (SELECT id FROM customers WHERE email = 'tax@system.yamata-no-orochi.com'),
    0, -- Initial tax balance starts at 0
    0,
    0,
    0,
    'initial_balance',
    'Initial balance snapshot for tax wallet',
    '{"source": "migration", "wallet_type": "tax_wallet", "owner": "system"}',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
);