BEGIN;

-- Drop clicks table
DROP TABLE IF EXISTS short_link_clicks;

-- Revert short_links schema changes
-- Remove client_id index
DROP INDEX IF EXISTS idx_short_links_client_id;

-- Drop client_id column
ALTER TABLE short_links
    DROP COLUMN IF EXISTS client_id;

-- Make phone_number NOT NULL (if it exists)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'short_links' AND column_name = 'phone_number' AND is_nullable = 'YES'
    ) THEN
        EXECUTE 'ALTER TABLE short_links ALTER COLUMN phone_number SET NOT NULL';
    END IF;
END $$;

-- Drop short_link column
ALTER TABLE short_links
    DROP COLUMN IF EXISTS short_link;

-- Rename long_link -> link (if exists)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'short_links' AND column_name = 'long_link'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'short_links' AND column_name = 'link'
    ) THEN
        EXECUTE 'ALTER TABLE short_links RENAME COLUMN long_link TO link';
    END IF;
END $$;

-- Restore clicks column
ALTER TABLE short_links
    ADD COLUMN IF NOT EXISTS clicks BIGINT NOT NULL DEFAULT 0;

COMMIT; 