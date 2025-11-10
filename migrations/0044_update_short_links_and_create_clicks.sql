BEGIN;

-- 1) Update short_links schema
--    - Remove clicks
--    - Rename link -> long_link
--    - Add short_link (full short URL)
--    - Add client_id (nullable)
--    - Make phone_number nullable

-- Drop clicks column if exists
ALTER TABLE short_links
    DROP COLUMN IF EXISTS clicks;

-- Rename link -> long_link (only if long_link doesn't already exist)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'short_links' AND column_name = 'link'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'short_links' AND column_name = 'long_link'
    ) THEN
        EXECUTE 'ALTER TABLE short_links RENAME COLUMN link TO long_link';
    END IF;
END $$;

-- Add short_link column if not exists
ALTER TABLE short_links
    ADD COLUMN IF NOT EXISTS short_link TEXT NOT NULL DEFAULT '';

-- Add client_id (nullable)
ALTER TABLE short_links
    ADD COLUMN IF NOT EXISTS client_id INTEGER NULL;

-- Make phone_number nullable if currently NOT NULL
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'short_links' AND column_name = 'phone_number' AND is_nullable = 'NO'
    ) THEN
        EXECUTE 'ALTER TABLE short_links ALTER COLUMN phone_number DROP NOT NULL';
    END IF;
END $$;

-- Create index for client_id
CREATE INDEX IF NOT EXISTS idx_short_links_client_id ON short_links(client_id);

-- Update comments
COMMENT ON COLUMN short_links.long_link IS 'Original target URL';
COMMENT ON COLUMN short_links.short_link IS 'Full short URL to share';
COMMENT ON COLUMN short_links.client_id IS 'Optional client identifier (no FK)';
COMMENT ON COLUMN short_links.phone_number IS 'Recipient phone number (nullable)';

-- 2) Create short_link_clicks table
CREATE TABLE IF NOT EXISTS short_link_clicks (
    id BIGSERIAL PRIMARY KEY,
    short_link_id BIGINT NOT NULL,
    user_agent TEXT NULL,
    ip VARCHAR(64) NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);

-- Indexes for short_link_clicks
CREATE INDEX IF NOT EXISTS idx_short_link_clicks_short_link_id ON short_link_clicks(short_link_id);
CREATE INDEX IF NOT EXISTS idx_short_link_clicks_created_at ON short_link_clicks(created_at);

-- Comments for short_link_clicks
COMMENT ON TABLE short_link_clicks IS 'Click events for short links (one row per click)';
COMMENT ON COLUMN short_link_clicks.short_link_id IS 'Reference to short_links.id (no FK)';
COMMENT ON COLUMN short_link_clicks.user_agent IS 'User agent at click time';
COMMENT ON COLUMN short_link_clicks.ip IS 'IP address at click time';

COMMIT;
 