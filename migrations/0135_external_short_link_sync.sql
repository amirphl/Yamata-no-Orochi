-- External short-link publication state and idempotent click import metadata.

BEGIN;

ALTER TABLE short_links
    ADD COLUMN IF NOT EXISTS external_published_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_short_links_external_unpublished
    ON short_links (id)
    WHERE external_published_at IS NULL;

ALTER TABLE short_link_clicks
    ADD COLUMN IF NOT EXISTS source VARCHAR(64) NOT NULL DEFAULT 'local',
    ADD COLUMN IF NOT EXISTS external_click_id BIGINT,
    ADD COLUMN IF NOT EXISTS referer TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS uk_short_link_clicks_source_external_id
    ON short_link_clicks (source, external_click_id);

CREATE TABLE IF NOT EXISTS external_short_link_sync_state (
    source VARCHAR(64) PRIMARY KEY,
    last_click_id BIGINT NOT NULL DEFAULT 0 CHECK (last_click_id >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO external_short_link_sync_state (source, last_click_id)
VALUES ('external_shortlink', 0)
ON CONFLICT (source) DO NOTHING;

COMMENT ON COLUMN short_links.external_published_at IS
    'Last acknowledgement that this immutable mapping exists on the external redirect service';
COMMENT ON COLUMN short_link_clicks.external_click_id IS
    'Monotonic click ID assigned by the external redirect service';
COMMENT ON TABLE external_short_link_sync_state IS
    'Durable ID cursors for inbound external short-link click sources';

COMMIT;

