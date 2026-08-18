BEGIN;

CREATE TABLE IF NOT EXISTS links (
    link_id BIGSERIAL PRIMARY KEY,
    code VARCHAR(64) NOT NULL UNIQUE,
    long_url VARCHAR(4096) NOT NULL,
    short_url VARCHAR(4096),
    source_link_id BIGINT,
    campaign_id BIGINT,
    client_id BIGINT,
    scenario_id BIGINT,
    scenario_name VARCHAR(512),
    phone_number VARCHAR(32),
    source_created_at TIMESTAMPTZ,
    source_updated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT links_code_not_reserved CHECK (code NOT IN ('api', 'healthz', 'readyz', 'metrics')),
    CONSTRAINT links_http_destination CHECK (long_url ~* '^https?://')
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_links_code ON links (code);

CREATE TABLE IF NOT EXISTS clicks (
    click_id BIGSERIAL PRIMARY KEY,
    event_id UUID NOT NULL UNIQUE,
    short_code VARCHAR(64) NOT NULL,
    link_id BIGINT NOT NULL,
    long_url VARCHAR(4096) NOT NULL,
    short_url VARCHAR(4096),
    source_link_id BIGINT,
    campaign_id BIGINT,
    client_id BIGINT,
    scenario_id BIGINT,
    scenario_name VARCHAR(512),
    phone_number VARCHAR(32),
    link_created_at TIMESTAMPTZ,
    link_updated_at TIMESTAMPTZ,
    clicked_at TIMESTAMPTZ NOT NULL,
    client_ip VARCHAR(64),
    user_agent VARCHAR(1024),
    referer VARCHAR(2048)
);

CREATE INDEX IF NOT EXISTS idx_clicks_click_id ON clicks (click_id);
CREATE INDEX IF NOT EXISTS idx_clicks_link_id ON clicks (link_id);
CREATE INDEX IF NOT EXISTS idx_clicks_clicked_at ON clicks (clicked_at);

CREATE TABLE IF NOT EXISTS click_acknowledgements (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    through_click_id BIGINT NOT NULL DEFAULT 0,
    acknowledged_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO click_acknowledgements(singleton, through_click_id)
VALUES (TRUE, 0)
ON CONFLICT (singleton) DO NOTHING;

COMMIT;

