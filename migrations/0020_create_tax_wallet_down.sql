-- Remove tax wallet and its balance snapshot

-- Remove balance snapshot first (due to foreign key constraint)
DELETE FROM balance_snapshots 
WHERE wallet_id = (SELECT id FROM wallets WHERE uuid = '2672a1bf-b344-4d84-adee-5b92307a2e7c'::uuid);

-- Remove the tax wallet
DELETE FROM wallets 
WHERE uuid = '2672a1bf-b344-4d84-adee-5b92307a2e7c'::uuid; 