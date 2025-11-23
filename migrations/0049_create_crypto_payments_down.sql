-- 0049_create_crypto_payments_down.sql
-- Drop crypto payment related tables

BEGIN;

DROP TABLE IF EXISTS crypto_deposits;
DROP TABLE IF EXISTS crypto_payment_requests;

COMMIT; 