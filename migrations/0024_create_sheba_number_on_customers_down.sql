-- Migration: Remove sheba_number column from customers
-- DOWN
BEGIN;

ALTER TABLE customers
DROP COLUMN IF EXISTS sheba_number; 

COMMIT;