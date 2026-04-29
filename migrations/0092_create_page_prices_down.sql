-- Migration: 0092_create_page_prices_down.sql
-- Description: Drop page_prices table

BEGIN;

DROP TABLE IF EXISTS page_prices;

COMMIT;
