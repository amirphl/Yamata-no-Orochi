-- Add lang column to payment_requests for storing language preference
ALTER TABLE IF EXISTS payment_requests
    ADD COLUMN IF NOT EXISTS lang VARCHAR(2) NOT NULL DEFAULT 'EN';

COMMENT ON COLUMN payment_requests.lang IS 'Language code for the payment request (e.g., EN, FA)';
