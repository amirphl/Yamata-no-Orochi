-- Migration: Create SMS campaigns table
-- Up migration

CREATE TYPE sms_campaign_status AS ENUM (
    'initiated',
    'in-progress',
    'waiting-for-approval',
    'approved',
    'rejected'
);

CREATE TABLE sms_campaigns (
    id SERIAL PRIMARY KEY,
    uuid UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    customer_id INTEGER NOT NULL,
    status sms_campaign_status NOT NULL DEFAULT 'initiated',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    spec JSONB NOT NULL DEFAULT '{}',
    
    -- Foreign key constraint
    CONSTRAINT fk_sms_campaigns_customer 
        FOREIGN KEY (customer_id) 
        REFERENCES customers(id) 
        ON DELETE CASCADE,
    
    -- Indexes for performance
    CONSTRAINT idx_sms_campaigns_uuid UNIQUE (uuid),
    CONSTRAINT idx_sms_campaigns_customer_id (customer_id),
    CONSTRAINT idx_sms_campaigns_status (status),
    CONSTRAINT idx_sms_campaigns_created_at (created_at)
);

-- Create indexes for better query performance
CREATE INDEX idx_sms_campaigns_customer_id ON sms_campaigns(customer_id);
CREATE INDEX idx_sms_campaigns_status ON sms_campaigns(status);
CREATE INDEX idx_sms_campaigns_created_at ON sms_campaigns(created_at);
CREATE INDEX idx_sms_campaigns_updated_at ON sms_campaigns(updated_at);

-- Add comment to table
COMMENT ON TABLE sms_campaigns IS 'SMS campaign management table for tracking campaign status and specifications';
COMMENT ON COLUMN sms_campaigns.uuid IS 'Unique identifier for the SMS campaign';
COMMENT ON COLUMN sms_campaigns.customer_id IS 'Reference to the customer who created the campaign';
COMMENT ON COLUMN sms_campaigns.status IS 'Current status of the SMS campaign';
COMMENT ON COLUMN sms_campaigns.spec IS 'JSON specification containing campaign details';
COMMENT ON COLUMN sms_campaigns.created_at IS 'Timestamp when the campaign was created';
COMMENT ON COLUMN sms_campaigns.updated_at IS 'Timestamp when the campaign was last updated'; 