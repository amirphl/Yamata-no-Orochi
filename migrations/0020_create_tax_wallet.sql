-- Create tax wallet for collecting transaction taxes
-- This wallet will receive 10% of all payment amounts as tax

INSERT INTO wallets (uuid, customer_id, metadata, created_at, updated_at)
VALUES (
    '2672a1bf-b344-4d84-adee-5b92307a2e7c'::uuid,
    0, -- No customer ID for system wallet
    '{"type": "tax_wallet", "description": "System wallet for collecting transaction taxes", "default_tax_rate": 0.10, "created_via": "migration"}',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
);

-- Create initial balance snapshot for tax wallet
INSERT INTO balance_snapshots (uuid, correlation_id, wallet_id, customer_id, free_balance, frozen_balance, locked_balance, total_balance, reason, description, metadata, created_at, updated_at)
VALUES (
    gen_random_uuid(),
    gen_random_uuid(),
    (SELECT id FROM wallets WHERE uuid = '2672a1bf-b344-4d84-adee-5b92307a2e7c'::uuid),
    0,
    0, -- Initial tax balance starts at 0
    0,
    0,
    0,
    'initial_balance',
    'Initial balance snapshot for tax wallet',
    '{"source": "migration", "wallet_type": "tax_wallet"}',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
); 