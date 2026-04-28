-- Migration: 0092_create_page_prices.sql
-- Description: Create page_prices table (append-only historical records) and seed defaults

BEGIN;

CREATE TABLE IF NOT EXISTS page_prices (
    id SERIAL PRIMARY KEY,
    platform VARCHAR(50) NOT NULL,
    price BIGINT NOT NULL CHECK (price > 0),
    created_by_admin_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_page_prices_platform_created_at
    ON page_prices (platform, created_at DESC, id DESC);

INSERT INTO page_prices (platform, price) VALUES
('bale', 200),
('rubika', 200),
('splus', 200),
('sms', 200);

COMMIT;
