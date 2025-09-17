-- Migration: Create agency commission and commission rate models
-- This migration adds support for agency commission distribution

-- Create commission_status_enum
CREATE TYPE commission_status_enum AS ENUM (
    'pending',
    'paid',
    'failed',
    'cancelled'
);

-- Create commission_type_enum
CREATE TYPE commission_type_enum AS ENUM (
    'campaign_creation',
    'campaign_rejection',
    'referral',
    'service'
);

-- Create commission_rates table
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

-- Create indexes for commission_rates
CREATE INDEX IF NOT EXISTS idx_commission_rates_agency_id ON commission_rates(agency_id);
CREATE INDEX IF NOT EXISTS idx_commission_rates_transaction_type ON commission_rates(transaction_type);
CREATE INDEX IF NOT EXISTS idx_commission_rates_is_active ON commission_rates(is_active);
CREATE INDEX IF NOT EXISTS idx_commission_rates_deleted_at ON commission_rates(deleted_at);

-- Create unique constraint for agency + transaction type combination
CREATE UNIQUE INDEX IF NOT EXISTS idx_commission_rates_agency_transaction_type 
ON commission_rates(agency_id, transaction_type) WHERE deleted_at IS NULL;

-- Create agency_commissions table
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
    CONSTRAINT fk_agency_commissions_source_campaign_id FOREIGN KEY (source_campaign_id) REFERENCES sms_campaigns(id) ON DELETE SET NULL,
    CONSTRAINT fk_agency_commissions_payment_transaction_id FOREIGN KEY (payment_transaction_id) REFERENCES transactions(id) ON DELETE SET NULL
);

-- Create indexes for agency_commissions
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

-- Add comments for documentation
COMMENT ON TABLE commission_rates IS 'Commission rate configuration for agencies by transaction type';
COMMENT ON TABLE agency_commissions IS 'Agency commission tracking and distribution';

COMMENT ON COLUMN commission_rates.rate IS 'Commission rate as decimal (e.g., 0.15 for 15%)';
COMMENT ON COLUMN commission_rates.min_amount IS 'Minimum transaction amount for commission to apply';
COMMENT ON COLUMN commission_rates.max_amount IS 'Maximum transaction amount for commission (null = no limit)';

COMMENT ON COLUMN agency_commissions.correlation_id IS 'Links commission to source transaction for audit trail';
COMMENT ON COLUMN agency_commissions.percentage IS 'Commission percentage applied (e.g., 0.15 for 15%)';
COMMENT ON COLUMN agency_commissions.base_amount IS 'Original transaction amount that generated commission';
COMMENT ON COLUMN agency_commissions.source_transaction_id IS 'Transaction that generated commission (e.g., campaign creation)';
COMMENT ON COLUMN agency_commissions.source_campaign_id IS 'SMS campaign if commission is campaign-related';
COMMENT ON COLUMN agency_commissions.payment_transaction_id IS 'Transaction that paid the commission to agency wallet'; 