-- Migration: Create wallet, transaction, balance snapshot, and payment request models
-- This migration implements an immutable accounting system with correlation IDs

-- Create wallets table
CREATE TABLE IF NOT EXISTS wallets (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    customer_id BIGINT NOT NULL UNIQUE,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    CONSTRAINT fk_wallets_customer_id FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE
);

-- Create indexes for wallets
CREATE INDEX IF NOT EXISTS idx_wallets_customer_id ON wallets(customer_id);
CREATE INDEX IF NOT EXISTS idx_wallets_deleted_at ON wallets(deleted_at);

-- Create transaction_types enum
CREATE TYPE transaction_type_enum AS ENUM (
    'deposit',
    'withdrawal', 
    'freeze',
    'unfreeze',
    'lock',
    'unlock',
    'refund',
    'fee',
    'adjustment'
);

-- Create transaction_status_enum
CREATE TYPE transaction_status_enum AS ENUM (
    'pending',
    'completed',
    'failed',
    'cancelled',
    'reversed'
);

-- Create transactions table
CREATE TABLE IF NOT EXISTS transactions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    correlation_id UUID NOT NULL,
    type transaction_type_enum NOT NULL,
    status transaction_status_enum NOT NULL DEFAULT 'pending',
    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'IRR',
    wallet_id BIGINT NOT NULL,
    customer_id BIGINT NOT NULL,
    balance_before JSONB NOT NULL,
    balance_after JSONB NOT NULL,
    external_reference VARCHAR(255),
    external_trace VARCHAR(255),
    external_rrn VARCHAR(255),
    external_masked_pan VARCHAR(255),
    description TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    CONSTRAINT fk_transactions_wallet_id FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE CASCADE,
    CONSTRAINT fk_transactions_customer_id FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE
);

-- Create indexes for transactions
CREATE INDEX IF NOT EXISTS idx_transactions_correlation_id ON transactions(correlation_id);
CREATE INDEX IF NOT EXISTS idx_transactions_type ON transactions(type);
CREATE INDEX IF NOT EXISTS idx_transactions_status ON transactions(status);
CREATE INDEX IF NOT EXISTS idx_transactions_wallet_id ON transactions(wallet_id);
CREATE INDEX IF NOT EXISTS idx_transactions_customer_id ON transactions(customer_id);
CREATE INDEX IF NOT EXISTS idx_transactions_external_reference ON transactions(external_reference);
CREATE INDEX IF NOT EXISTS idx_transactions_deleted_at ON transactions(deleted_at);

-- Create balance_snapshots table
CREATE TABLE IF NOT EXISTS balance_snapshots (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    correlation_id UUID NOT NULL,
    wallet_id BIGINT NOT NULL,
    customer_id BIGINT NOT NULL,
    free_balance BIGINT NOT NULL,
    frozen_balance BIGINT NOT NULL,
    locked_balance BIGINT NOT NULL,
    total_balance BIGINT NOT NULL,
    reason VARCHAR(100) NOT NULL,
    description TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    CONSTRAINT fk_balance_snapshots_wallet_id FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE CASCADE,
    CONSTRAINT fk_balance_snapshots_customer_id FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE
);

-- Create indexes for balance_snapshots
CREATE INDEX IF NOT EXISTS idx_balance_snapshots_correlation_id ON balance_snapshots(correlation_id);
CREATE INDEX IF NOT EXISTS idx_balance_snapshots_wallet_id ON balance_snapshots(wallet_id);
CREATE INDEX IF NOT EXISTS idx_balance_snapshots_customer_id ON balance_snapshots(customer_id);
CREATE INDEX IF NOT EXISTS idx_balance_snapshots_deleted_at ON balance_snapshots(deleted_at);

-- Create payment_request_status_enum
CREATE TYPE payment_request_status_enum AS ENUM (
    'created',
    'tokenized',
    'pending',
    'completed',
    'failed',
    'cancelled',
    'expired',
    'refunded'
);

-- Create payment_requests table
CREATE TABLE IF NOT EXISTS payment_requests (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    correlation_id UUID NOT NULL,
    customer_id BIGINT NOT NULL,
    wallet_id BIGINT NOT NULL,
    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'IRR',
    description TEXT,
    invoice_number VARCHAR(255) UNIQUE NOT NULL,
    cell_number VARCHAR(20),
    redirect_url TEXT NOT NULL,
    atipay_token VARCHAR(255),
    atipay_status VARCHAR(50),
    payment_state VARCHAR(50),
    payment_status VARCHAR(50),
    payment_reference VARCHAR(255),
    payment_reservation VARCHAR(255),
    payment_terminal VARCHAR(50),
    payment_trace VARCHAR(255),
    payment_masked_pan VARCHAR(255),
    payment_rrn VARCHAR(255),
    status payment_request_status_enum NOT NULL DEFAULT 'created',
    status_reason TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    
    CONSTRAINT fk_payment_requests_customer_id FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE,
    CONSTRAINT fk_payment_requests_wallet_id FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE CASCADE
);

-- Create indexes for payment_requests
CREATE INDEX IF NOT EXISTS idx_payment_requests_correlation_id ON payment_requests(correlation_id);
CREATE INDEX IF NOT EXISTS idx_payment_requests_customer_id ON payment_requests(customer_id);
CREATE INDEX IF NOT EXISTS idx_payment_requests_wallet_id ON payment_requests(wallet_id);
CREATE INDEX IF NOT EXISTS idx_payment_requests_invoice_number ON payment_requests(invoice_number);
CREATE INDEX IF NOT EXISTS idx_payment_requests_atipay_token ON payment_requests(atipay_token);
CREATE INDEX IF NOT EXISTS idx_payment_requests_payment_reference ON payment_requests(payment_reference);
CREATE INDEX IF NOT EXISTS idx_payment_requests_status ON payment_requests(status);
CREATE INDEX IF NOT EXISTS idx_payment_requests_expires_at ON payment_requests(expires_at);
CREATE INDEX IF NOT EXISTS idx_payment_requests_deleted_at ON payment_requests(deleted_at);

-- Add comments for documentation
COMMENT ON TABLE wallets IS 'Customer wallet references (immutable design - balances stored in balance_snapshots)';
COMMENT ON TABLE transactions IS 'Immutable financial transactions with correlation IDs for audit trail';
COMMENT ON TABLE balance_snapshots IS 'Immutable balance snapshots - source of truth for wallet balances at any point in time';
COMMENT ON TABLE payment_requests IS 'Atipay payment requests for wallet recharge with full lifecycle tracking';

COMMENT ON COLUMN transactions.correlation_id IS 'Links related transactions (e.g., freeze/unfreeze pair)';
COMMENT ON COLUMN transactions.balance_before IS 'JSON snapshot of wallet balances before transaction';
COMMENT ON COLUMN transactions.balance_after IS 'JSON snapshot of wallet balances after transaction';
COMMENT ON COLUMN balance_snapshots.correlation_id IS 'Links snapshot to the transaction that caused it';
COMMENT ON COLUMN payment_requests.correlation_id IS 'Links payment request to resulting transactions and snapshots'; 