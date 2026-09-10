BEGIN;

DROP TABLE IF EXISTS external_short_link_sync_state;
DROP INDEX IF EXISTS uk_short_link_clicks_source_external_id;

ALTER TABLE short_link_clicks
    DROP COLUMN IF EXISTS referer,
    DROP COLUMN IF EXISTS external_click_id,
    DROP COLUMN IF EXISTS source;

DROP INDEX IF EXISTS idx_short_links_external_unpublished;
ALTER TABLE short_links DROP COLUMN IF EXISTS external_published_at;

COMMIT;
