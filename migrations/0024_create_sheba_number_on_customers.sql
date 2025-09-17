-- Migration: Add sheba_number column to customers
-- UP

BEGIN;

ALTER TABLE customers
ADD COLUMN IF NOT EXISTS sheba_number VARCHAR(255);

-- Optional: add index if queries will filter by sheba
-- CREATE INDEX IF NOT EXISTS idx_customers_sheba_number ON customers(sheba_number); 

COMMIT;