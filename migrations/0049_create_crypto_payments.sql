-- 0049_create_crypto_payments.sql
-- Create tables for crypto payment requests and on-chain deposits

BEGIN;

-- crypto_payment_requests
CREATE TABLE IF NOT EXISTS crypto_payment_requests (
	id SERIAL PRIMARY KEY,
	uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
	correlation_id UUID NOT NULL,

	customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
	wallet_id INTEGER NOT NULL REFERENCES wallets(id) ON DELETE CASCADE,

	fiat_amount_toman BIGINT NOT NULL,
	fiat_currency VARCHAR(3) NOT NULL DEFAULT 'TMN',
	coin VARCHAR(16) NOT NULL,
	network VARCHAR(64) NOT NULL,
	platform VARCHAR(64) NOT NULL,
	expected_coin_amount NUMERIC(38,18) NOT NULL,
	exchange_rate NUMERIC(38,18) NOT NULL,
	rate_source VARCHAR(128),

	deposit_address VARCHAR(255),
	deposit_memo VARCHAR(255),
	provider_request_id VARCHAR(255),

	status VARCHAR(32) NOT NULL DEFAULT 'created',
	status_reason TEXT,

	expires_at TIMESTAMPTZ,
	detected_at TIMESTAMPTZ,
	confirmed_at TIMESTAMPTZ,
	credited_at TIMESTAMPTZ,

	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
	deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_crypto_payment_requests_uuid ON crypto_payment_requests(uuid);
CREATE INDEX IF NOT EXISTS idx_crypto_payment_requests_corr ON crypto_payment_requests(correlation_id);
CREATE INDEX IF NOT EXISTS idx_crypto_payment_requests_customer ON crypto_payment_requests(customer_id);
CREATE INDEX IF NOT EXISTS idx_crypto_payment_requests_wallet ON crypto_payment_requests(wallet_id);
CREATE INDEX IF NOT EXISTS idx_crypto_payment_requests_status ON crypto_payment_requests(status);
CREATE INDEX IF NOT EXISTS idx_crypto_payment_requests_coin ON crypto_payment_requests(coin);
CREATE INDEX IF NOT EXISTS idx_crypto_payment_requests_platform ON crypto_payment_requests(platform);
CREATE INDEX IF NOT EXISTS idx_crypto_payment_requests_network ON crypto_payment_requests(network);
CREATE INDEX IF NOT EXISTS idx_crypto_payment_requests_address_memo ON crypto_payment_requests(deposit_address, deposit_memo);
CREATE INDEX IF NOT EXISTS idx_crypto_payment_requests_provider_request_id ON crypto_payment_requests(provider_request_id);

-- crypto_deposits
CREATE TABLE IF NOT EXISTS crypto_deposits (
	id SERIAL PRIMARY KEY,
	uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
	correlation_id UUID NOT NULL,

	crypto_payment_request_id INTEGER REFERENCES crypto_payment_requests(id) ON DELETE SET NULL,
	customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
	wallet_id INTEGER NOT NULL REFERENCES wallets(id) ON DELETE CASCADE,

	coin VARCHAR(16) NOT NULL,
	network VARCHAR(64) NOT NULL,
	platform VARCHAR(64) NOT NULL,

	tx_hash VARCHAR(255) UNIQUE,
	from_address VARCHAR(255),
	to_address VARCHAR(255),
	destination_tag VARCHAR(255),

	amount_coin NUMERIC(38,18) NOT NULL,
	confirmations INTEGER NOT NULL DEFAULT 0,
	required_confirmations INTEGER NOT NULL DEFAULT 0,
	block_height BIGINT,

	detected_at TIMESTAMPTZ,
	confirmed_at TIMESTAMPTZ,
	credited_at TIMESTAMPTZ,

	status VARCHAR(32) NOT NULL DEFAULT 'detected',
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,

	created_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
	deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_crypto_deposits_uuid ON crypto_deposits(uuid);
CREATE INDEX IF NOT EXISTS idx_crypto_deposits_corr ON crypto_deposits(correlation_id);
CREATE INDEX IF NOT EXISTS idx_crypto_deposits_cpr ON crypto_deposits(crypto_payment_request_id);
CREATE INDEX IF NOT EXISTS idx_crypto_deposits_customer ON crypto_deposits(customer_id);
CREATE INDEX IF NOT EXISTS idx_crypto_deposits_wallet ON crypto_deposits(wallet_id);
CREATE INDEX IF NOT EXISTS idx_crypto_deposits_status ON crypto_deposits(status);
CREATE INDEX IF NOT EXISTS idx_crypto_deposits_coin ON crypto_deposits(coin);
CREATE INDEX IF NOT EXISTS idx_crypto_deposits_platform ON crypto_deposits(platform);
CREATE INDEX IF NOT EXISTS idx_crypto_deposits_network ON crypto_deposits(network);
CREATE INDEX IF NOT EXISTS idx_crypto_deposits_toaddr ON crypto_deposits(to_address, destination_tag);

COMMIT; 