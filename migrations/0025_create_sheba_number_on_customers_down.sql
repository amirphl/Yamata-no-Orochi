-- Migration: Remove sheba_number column from customers
-- DOWN
ALTER TABLE customers
DROP COLUMN IF EXISTS sheba_number; 