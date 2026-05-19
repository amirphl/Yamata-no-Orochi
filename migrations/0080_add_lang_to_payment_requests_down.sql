-- Remove lang column from payment_requests
ALTER TABLE IF EXISTS payment_requests
    DROP COLUMN IF EXISTS lang;
