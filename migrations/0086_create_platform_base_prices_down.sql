-- Migration: 0086_create_platform_base_prices_down.sql
-- Description: Drop platform_base_prices table

BEGIN;
DROP TABLE IF EXISTS platform_base_prices;
COMMIT;
