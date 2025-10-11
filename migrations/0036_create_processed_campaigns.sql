-- Migration: 0036_create_processed_campaigns.sql
-- Description: Create processed_campaigns table to store ordered audience resolution for campaigns

-- UP MIGRATION

CREATE TABLE IF NOT EXISTS processed_campaigns (
    id BIGSERIAL PRIMARY KEY,
    campaign_id BIGINT NOT NULL,
    campaign_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    audience_ids BIGINT[] NOT NULL DEFAULT '{}',
    audience_codes TEXT[] NOT NULL DEFAULT '{}',
    last_audience_id BIGINT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_processed_campaigns_campaign_id ON processed_campaigns(campaign_id);
CREATE INDEX IF NOT EXISTS idx_processed_campaigns_created_at ON processed_campaigns(created_at); 