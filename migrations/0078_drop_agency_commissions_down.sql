-- Down Migration: recreate commission enums, commission_rates, and agency_commissions tables

BEGIN;

-- Enums
CREATE TYPE commission_status_enum AS ENUM (
    'pending',
    'paid',
    'failed',
    'cancelled'
);

CREATE TYPE commission_type_enum AS ENUM (
    'campaign_creation',
    'campaign_rejection',
    'referral',
    'service'
);

-- commission_rates table
CREATE TABLE IF NOT EXISTS commission_rates (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    agency_id BIGINT NOT NULL,
    transaction_type VARCHAR(30) NOT NULL,
    rate DECIMAL(5,4) NOT NULL,
    min_amount BIGINT NOT NULL DEFAULT 0,
    max_amount BIGINT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    description TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT fk_commission_rates_agency_id FOREIGN KEY (agency_id) REFERENCES customers(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_commission_rates_agency_id ON commission_rates(agency_id);
CREATE INDEX IF NOT EXISTS idx_commission_rates_transaction_type ON commission_rates(transaction_type);
CREATE INDEX IF NOT EXISTS idx_commission_rates_is_active ON commission_rates(is_active);
CREATE INDEX IF NOT EXISTS idx_commission_rates_deleted_at ON commission_rates(deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_commission_rates_agency_transaction_type 
ON commission_rates(agency_id, transaction_type) WHERE deleted_at IS NULL;

-- agency_commissions table
CREATE TABLE IF NOT EXISTS agency_commissions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    correlation_id UUID NOT NULL,
    agency_id BIGINT NOT NULL,
    customer_id BIGINT NOT NULL,
    wallet_id BIGINT NOT NULL,
    type commission_type_enum NOT NULL,
    status commission_status_enum NOT NULL DEFAULT 'pending',
    amount BIGINT NOT NULL,
    percentage DECIMAL(5,4) NOT NULL,
    base_amount BIGINT NOT NULL,
    source_transaction_id BIGINT,
    source_campaign_id BIGINT,
    description TEXT,
    metadata JSONB DEFAULT '{}',
    paid_at TIMESTAMP WITH TIME ZONE,
    payment_transaction_id BIGINT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT fk_agency_commissions_agency_id FOREIGN KEY (agency_id) REFERENCES customers(id) ON DELETE CASCADE,
    CONSTRAINT fk_agency_commissions_customer_id FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE,
    CONSTRAINT fk_agency_commissions_wallet_id FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE CASCADE,
    CONSTRAINT fk_agency_commissions_source_transaction_id FOREIGN KEY (source_transaction_id) REFERENCES transactions(id) ON DELETE SET NULL,
    CONSTRAINT fk_agency_commissions_source_campaign_id FOREIGN KEY (source_campaign_id) REFERENCES campaigns(id) ON DELETE SET NULL,
    CONSTRAINT fk_agency_commissions_payment_transaction_id FOREIGN KEY (payment_transaction_id) REFERENCES transactions(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_agency_commissions_correlation_id ON agency_commissions(correlation_id);
CREATE INDEX IF NOT EXISTS idx_agency_commissions_agency_id ON agency_commissions(agency_id);
CREATE INDEX IF NOT EXISTS idx_agency_commissions_customer_id ON agency_commissions(customer_id);
CREATE INDEX IF NOT EXISTS idx_agency_commissions_wallet_id ON agency_commissions(wallet_id);
CREATE INDEX IF NOT EXISTS idx_agency_commissions_type ON agency_commissions(type);
CREATE INDEX IF NOT EXISTS idx_agency_commissions_status ON agency_commissions(status);
CREATE INDEX IF NOT EXISTS idx_agency_commissions_source_transaction_id ON agency_commissions(source_transaction_id);
CREATE INDEX IF NOT EXISTS idx_agency_commissions_source_campaign_id ON agency_commissions(source_campaign_id);
CREATE INDEX IF NOT EXISTS idx_agency_commissions_paid_at ON agency_commissions(paid_at);
CREATE INDEX IF NOT EXISTS idx_agency_commissions_payment_transaction_id ON agency_commissions(payment_transaction_id);
CREATE INDEX IF NOT EXISTS idx_agency_commissions_deleted_at ON agency_commissions(deleted_at);

COMMIT;
