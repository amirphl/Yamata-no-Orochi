-- Migration: 0086_create_platform_base_prices.sql
-- Description: Create platform_base_prices table and seed initial rows

BEGIN;

CREATE TABLE IF NOT EXISTS platform_base_prices (
    id SERIAL PRIMARY KEY,
    platform VARCHAR(50) NOT NULL,
    price BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_platform_base_price_platform ON platform_base_prices(platform) WHERE deleted_at IS NULL;

INSERT INTO platform_base_prices (platform, price) VALUES
('bale', 150),
('rubika', 200),
('splus', 200),
('sms', 200);

COMMIT;
