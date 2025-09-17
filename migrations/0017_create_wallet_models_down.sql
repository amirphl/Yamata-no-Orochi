-- Rollback migration: Drop wallet, transaction, balance snapshot, and payment request models

-- Drop tables in reverse order (due to foreign key constraints)
DROP TABLE IF EXISTS payment_requests;
DROP TABLE IF EXISTS balance_snapshots;
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS wallets;

-- Drop enums
DROP TYPE IF EXISTS payment_request_status_enum;
DROP TYPE IF EXISTS transaction_status_enum;
DROP TYPE IF EXISTS transaction_type_enum; 