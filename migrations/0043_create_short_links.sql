-- Migration: Create short_links table
-- Up migration

CREATE TABLE IF NOT EXISTS short_links (
    id BIGSERIAL PRIMARY KEY,
    uid VARCHAR(64) NOT NULL UNIQUE,
    campaign_id INTEGER NULL,
    phone_number VARCHAR(20) NOT NULL,
    clicks BIGINT NOT NULL DEFAULT 0,
    link TEXT NOT NULL,
    user_agent TEXT NULL,
    ip VARCHAR(64) NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_short_links_uid ON short_links(uid);
CREATE INDEX IF NOT EXISTS idx_short_links_campaign_id ON short_links(campaign_id);
CREATE INDEX IF NOT EXISTS idx_short_links_phone_number ON short_links(phone_number);
CREATE INDEX IF NOT EXISTS idx_short_links_created_at ON short_links(created_at);

-- Comments
COMMENT ON TABLE short_links IS 'Shortened links for campaigns and recipients';
COMMENT ON COLUMN short_links.uid IS 'Unique short token mapping to the original link';
COMMENT ON COLUMN short_links.campaign_id IS 'Associated campaign id (no FK)';
COMMENT ON COLUMN short_links.phone_number IS 'Recipient phone number';
COMMENT ON COLUMN short_links.clicks IS 'Total click counter';
COMMENT ON COLUMN short_links.link IS 'Original target URL';
COMMENT ON COLUMN short_links.user_agent IS 'Last known user agent';
COMMENT ON COLUMN short_links.ip IS 'Last known IP address'; 