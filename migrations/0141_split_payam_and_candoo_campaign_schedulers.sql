-- Split campaign execution persistence by SMS gateway.  Generic SMS tables
-- remain immutable legacy history; new scheduler writes are provider-owned.
BEGIN;

CREATE TABLE IF NOT EXISTS payam_processed_campaigns (
    id BIGSERIAL PRIMARY KEY,
    campaign_id BIGINT NOT NULL,
    is_current BOOLEAN NOT NULL DEFAULT TRUE,
    campaign_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    audience_ids BIGINT[] NOT NULL DEFAULT '{}',
    audience_codes TEXT[] NOT NULL DEFAULT '{}',
    last_audience_id BIGINT,
    statistics JSONB NOT NULL DEFAULT '{}'::jsonb,
    bundle_audience_selection_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_payam_processed_campaigns_campaign_current
    ON payam_processed_campaigns (campaign_id) WHERE is_current;

CREATE TABLE IF NOT EXISTS candoo_processed_campaigns (LIKE payam_processed_campaigns INCLUDING DEFAULTS);
CREATE SEQUENCE IF NOT EXISTS candoo_processed_campaigns_id_seq AS BIGINT;
ALTER SEQUENCE candoo_processed_campaigns_id_seq OWNED BY candoo_processed_campaigns.id;
ALTER TABLE candoo_processed_campaigns ALTER COLUMN id SET DEFAULT nextval('candoo_processed_campaigns_id_seq');
ALTER TABLE candoo_processed_campaigns ADD PRIMARY KEY (id);
CREATE UNIQUE INDEX IF NOT EXISTS uk_candoo_processed_campaigns_campaign_current
    ON candoo_processed_campaigns (campaign_id) WHERE is_current;

CREATE TABLE IF NOT EXISTS payam_sent_sms (
    id BIGSERIAL PRIMARY KEY,
    processed_campaign_id BIGINT NOT NULL REFERENCES payam_processed_campaigns(id) ON DELETE CASCADE,
    phone_number VARCHAR(20) NOT NULL,
    tracking_id VARCHAR(64) NOT NULL,
    provider VARCHAR(32) NOT NULL DEFAULT 'payamsms' CHECK (provider = 'payamsms'),
    provider_customer_id BIGINT,
    parts_delivered INTEGER NOT NULL DEFAULT 0,
    status sent_sms_status NOT NULL DEFAULT 'pending',
    server_id VARCHAR(64), error_code VARCHAR(64), description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);
CREATE INDEX IF NOT EXISTS idx_payam_sent_sms_processed ON payam_sent_sms(processed_campaign_id, id);
CREATE INDEX IF NOT EXISTS idx_payam_sent_sms_tracking ON payam_sent_sms(processed_campaign_id, tracking_id);

CREATE TABLE IF NOT EXISTS candoo_sent_sms (
    id BIGSERIAL PRIMARY KEY,
    processed_campaign_id BIGINT NOT NULL REFERENCES candoo_processed_campaigns(id) ON DELETE CASCADE,
    phone_number VARCHAR(20) NOT NULL,
    tracking_id VARCHAR(64) NOT NULL,
    provider VARCHAR(32) NOT NULL DEFAULT 'candoo' CHECK (provider = 'candoo'),
    provider_customer_id BIGINT NOT NULL,
    parts_delivered INTEGER NOT NULL DEFAULT 0,
    status sent_sms_status NOT NULL DEFAULT 'pending',
    server_id VARCHAR(64), error_code VARCHAR(64), description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);
CREATE INDEX IF NOT EXISTS idx_candoo_sent_sms_processed ON candoo_sent_sms(processed_campaign_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS uk_candoo_sent_sms_customer ON candoo_sent_sms(provider_customer_id);
CREATE INDEX IF NOT EXISTS idx_candoo_sent_sms_tracking ON candoo_sent_sms(processed_campaign_id, tracking_id);

CREATE TABLE IF NOT EXISTS payam_sms_status_jobs (
    id BIGSERIAL PRIMARY KEY,
    correlation_id VARCHAR(64) NOT NULL,
    processed_campaign_id BIGINT NOT NULL REFERENCES payam_processed_campaigns(id) ON DELETE CASCADE,
    platform VARCHAR(20) NOT NULL DEFAULT 'sms',
    provider VARCHAR(32) NOT NULL DEFAULT 'payamsms' CHECK (provider = 'payamsms'),
    tracking_ids TEXT[] NOT NULL,
    retry_count INTEGER NOT NULL DEFAULT 0,
    scheduled_at TIMESTAMPTZ NOT NULL,
    executed_at TIMESTAMPTZ, error TEXT, raw_provider_response TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);
CREATE INDEX IF NOT EXISTS idx_payam_sms_status_jobs_due ON payam_sms_status_jobs(scheduled_at, retry_count) WHERE executed_at IS NULL;

CREATE TABLE IF NOT EXISTS candoo_sms_status_jobs (LIKE payam_sms_status_jobs INCLUDING DEFAULTS);
CREATE SEQUENCE IF NOT EXISTS candoo_sms_status_jobs_id_seq AS BIGINT;
ALTER SEQUENCE candoo_sms_status_jobs_id_seq OWNED BY candoo_sms_status_jobs.id;
ALTER TABLE candoo_sms_status_jobs ALTER COLUMN id SET DEFAULT nextval('candoo_sms_status_jobs_id_seq');
ALTER TABLE candoo_sms_status_jobs ADD PRIMARY KEY (id);
ALTER TABLE candoo_sms_status_jobs ADD CONSTRAINT candoo_sms_status_jobs_provider_check CHECK (provider = 'candoo');
ALTER TABLE candoo_sms_status_jobs ALTER COLUMN provider SET DEFAULT 'candoo';
ALTER TABLE candoo_sms_status_jobs ADD CONSTRAINT candoo_sms_status_jobs_processed_campaign_id_fkey FOREIGN KEY (processed_campaign_id) REFERENCES candoo_processed_campaigns(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_candoo_sms_status_jobs_due ON candoo_sms_status_jobs(scheduled_at, retry_count) WHERE executed_at IS NULL;

CREATE TABLE IF NOT EXISTS payam_sms_status_results (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES payam_sms_status_jobs(id) ON DELETE CASCADE,
    processed_campaign_id BIGINT NOT NULL REFERENCES payam_processed_campaigns(id) ON DELETE CASCADE,
    tracking_id TEXT NOT NULL, server_id TEXT,
    provider VARCHAR(32) NOT NULL DEFAULT 'payamsms' CHECK (provider = 'payamsms'),
    provider_status_code TEXT, provider_status_text TEXT, internal_status sent_sms_status,
    total_parts BIGINT, total_delivered_parts BIGINT, total_undelivered_parts BIGINT, total_unknown_parts BIGINT,
    status TEXT, metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    UNIQUE(processed_campaign_id, tracking_id)
);

CREATE TABLE IF NOT EXISTS candoo_sms_status_results (LIKE payam_sms_status_results INCLUDING DEFAULTS);
CREATE SEQUENCE IF NOT EXISTS candoo_sms_status_results_id_seq AS BIGINT;
ALTER SEQUENCE candoo_sms_status_results_id_seq OWNED BY candoo_sms_status_results.id;
ALTER TABLE candoo_sms_status_results ALTER COLUMN id SET DEFAULT nextval('candoo_sms_status_results_id_seq');
ALTER TABLE candoo_sms_status_results ADD PRIMARY KEY (id);
ALTER TABLE candoo_sms_status_results ADD CONSTRAINT candoo_sms_status_results_provider_check CHECK (provider = 'candoo');
ALTER TABLE candoo_sms_status_results ALTER COLUMN provider SET DEFAULT 'candoo';
ALTER TABLE candoo_sms_status_results ADD CONSTRAINT candoo_sms_status_results_job_id_fkey FOREIGN KEY (job_id) REFERENCES candoo_sms_status_jobs(id) ON DELETE CASCADE;
ALTER TABLE candoo_sms_status_results ADD CONSTRAINT candoo_sms_status_results_processed_campaign_id_fkey FOREIGN KEY (processed_campaign_id) REFERENCES candoo_processed_campaigns(id) ON DELETE CASCADE;
ALTER TABLE candoo_sms_status_results ADD CONSTRAINT candoo_sms_status_results_processed_tracking_key UNIQUE (processed_campaign_id, tracking_id);

CREATE TABLE IF NOT EXISTS payam_sms_send_attempts (
    id BIGSERIAL PRIMARY KEY,
    processed_campaign_id BIGINT NOT NULL REFERENCES payam_processed_campaigns(id) ON DELETE CASCADE,
	provider VARCHAR(32) NOT NULL DEFAULT 'payamsms' CHECK (provider = 'payamsms'),
    tracking_ids TEXT[] NOT NULL, http_status_code INTEGER, response_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    response_body TEXT, error TEXT, attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);
CREATE TABLE IF NOT EXISTS candoo_sms_send_attempts (
    id BIGSERIAL PRIMARY KEY,
    processed_campaign_id BIGINT NOT NULL REFERENCES candoo_processed_campaigns(id) ON DELETE CASCADE,
	provider VARCHAR(32) NOT NULL DEFAULT 'candoo' CHECK (provider = 'candoo'),
    tracking_ids TEXT[] NOT NULL, http_status_code INTEGER, response_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    response_body TEXT, error TEXT, attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);

CREATE SEQUENCE IF NOT EXISTS candoo_scheduler_customer_id_seq AS BIGINT MINVALUE 1 START WITH 1;

-- New sender lines must make gateway ownership explicit. Existing rows were
-- normalized to Payam by 0137 and retain that historical assignment.
ALTER TABLE line_numbers ALTER COLUMN provider DROP DEFAULT;

-- A single old processed campaign must never have incomplete jobs owned by
-- both providers. Stop the release and repair the legacy rows if this fails.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM campaign_status_jobs
        WHERE platform = 'sms'
          AND executed_at IS NULL
          AND retry_count < 3
          AND COALESCE(NULLIF(BTRIM(provider), ''), 'payamsms') NOT IN ('payamsms', 'candoo')
    ) THEN
        RAISE EXCEPTION 'legacy SMS handoff found an unsupported provider';
    END IF;

    IF EXISTS (
        SELECT processed_campaign_id
        FROM campaign_status_jobs
        WHERE platform = 'sms' AND executed_at IS NULL AND retry_count < 3
        GROUP BY processed_campaign_id
        HAVING COUNT(DISTINCT COALESCE(NULLIF(BTRIM(provider), ''), 'payamsms')) > 1
    ) THEN
        RAISE EXCEPTION 'legacy SMS handoff found a processed campaign with multiple providers';
    END IF;
END $$;

CREATE TEMP TABLE active_sms_handoff ON COMMIT DROP AS
SELECT DISTINCT j.processed_campaign_id,
       COALESCE(NULLIF(BTRIM(j.provider), ''), 'payamsms') AS provider
FROM campaign_status_jobs AS j
WHERE j.platform = 'sms' AND j.executed_at IS NULL AND j.retry_count < 3;

INSERT INTO payam_processed_campaigns (id, campaign_id, is_current, campaign_json, audience_ids, audience_codes, last_audience_id, statistics, bundle_audience_selection_id, created_at, updated_at)
SELECT p.id, p.campaign_id, p.is_current, p.campaign_json, p.audience_ids, p.audience_codes, p.last_audience_id, p.statistics, p.bundle_audience_selection_id, p.created_at, p.updated_at
FROM processed_campaigns p JOIN active_sms_handoff h ON h.processed_campaign_id = p.id AND h.provider = 'payamsms'
ON CONFLICT (id) DO NOTHING;
INSERT INTO candoo_processed_campaigns (id, campaign_id, is_current, campaign_json, audience_ids, audience_codes, last_audience_id, statistics, bundle_audience_selection_id, created_at, updated_at)
SELECT p.id, p.campaign_id, p.is_current, p.campaign_json, p.audience_ids, p.audience_codes, p.last_audience_id, p.statistics, p.bundle_audience_selection_id, p.created_at, p.updated_at
FROM processed_campaigns p JOIN active_sms_handoff h ON h.processed_campaign_id = p.id AND h.provider = 'candoo'
ON CONFLICT (id) DO NOTHING;

INSERT INTO payam_sent_sms (id, processed_campaign_id, phone_number, tracking_id, provider, provider_customer_id, parts_delivered, status, server_id, error_code, description, created_at, updated_at)
SELECT s.id, s.processed_campaign_id, s.phone_number, s.tracking_id, 'payamsms', s.provider_customer_id, s.parts_delivered, s.status, s.server_id, s.error_code, s.description, s.created_at, s.updated_at
FROM sent_sms s JOIN active_sms_handoff h ON h.processed_campaign_id = s.processed_campaign_id AND h.provider = 'payamsms'
ON CONFLICT (id) DO NOTHING;
INSERT INTO candoo_sent_sms (id, processed_campaign_id, phone_number, tracking_id, provider, provider_customer_id, parts_delivered, status, server_id, error_code, description, created_at, updated_at)
SELECT s.id, s.processed_campaign_id, s.phone_number, s.tracking_id, 'candoo', s.provider_customer_id, s.parts_delivered, s.status, s.server_id, s.error_code, s.description, s.created_at, s.updated_at
FROM sent_sms s JOIN active_sms_handoff h ON h.processed_campaign_id = s.processed_campaign_id AND h.provider = 'candoo'
WHERE s.provider_customer_id IS NOT NULL
ON CONFLICT (id) DO NOTHING;

INSERT INTO payam_sms_status_jobs (id, correlation_id, processed_campaign_id, platform, provider, tracking_ids, retry_count, scheduled_at, executed_at, error, raw_provider_response, created_at, updated_at)
SELECT j.id, j.correlation_id, j.processed_campaign_id, 'sms', 'payamsms', j.tracking_ids, j.retry_count, j.scheduled_at, j.executed_at, j.error, j.raw_provider_response, j.created_at, j.updated_at
FROM campaign_status_jobs j JOIN active_sms_handoff h ON h.processed_campaign_id = j.processed_campaign_id AND h.provider = 'payamsms'
WHERE j.platform = 'sms'
  AND COALESCE(NULLIF(BTRIM(j.provider), ''), 'payamsms') = 'payamsms'
ON CONFLICT (id) DO NOTHING;
INSERT INTO candoo_sms_status_jobs (id, correlation_id, processed_campaign_id, platform, provider, tracking_ids, retry_count, scheduled_at, executed_at, error, raw_provider_response, created_at, updated_at)
SELECT j.id, j.correlation_id, j.processed_campaign_id, 'sms', 'candoo', j.tracking_ids, j.retry_count, j.scheduled_at, j.executed_at, j.error, j.raw_provider_response, j.created_at, j.updated_at
FROM campaign_status_jobs j JOIN active_sms_handoff h ON h.processed_campaign_id = j.processed_campaign_id AND h.provider = 'candoo'
WHERE j.platform = 'sms'
  AND COALESCE(NULLIF(BTRIM(j.provider), ''), 'payamsms') = 'candoo'
ON CONFLICT (id) DO NOTHING;

INSERT INTO payam_sms_status_results (id, job_id, processed_campaign_id, tracking_id, server_id, provider, provider_status_code, provider_status_text, internal_status, total_parts, total_delivered_parts, total_undelivered_parts, total_unknown_parts, status, metadata, created_at)
SELECT r.id, r.job_id, r.processed_campaign_id, r.tracking_id, r.server_id, 'payamsms', r.provider_status_code, r.provider_status_text, r.internal_status, r.total_parts, r.total_delivered_parts, r.total_undelivered_parts, r.total_unknown_parts, r.status, r.metadata, r.created_at
FROM sms_status_results r
JOIN campaign_status_jobs j ON j.id = r.job_id
JOIN active_sms_handoff h ON h.processed_campaign_id = r.processed_campaign_id AND h.provider = 'payamsms'
WHERE j.platform = 'sms'
  AND COALESCE(NULLIF(BTRIM(j.provider), ''), 'payamsms') = 'payamsms'
ON CONFLICT (id) DO NOTHING;
INSERT INTO candoo_sms_status_results (id, job_id, processed_campaign_id, tracking_id, server_id, provider, provider_status_code, provider_status_text, internal_status, total_parts, total_delivered_parts, total_undelivered_parts, total_unknown_parts, status, metadata, created_at)
SELECT r.id, r.job_id, r.processed_campaign_id, r.tracking_id, r.server_id, 'candoo', r.provider_status_code, r.provider_status_text, r.internal_status, r.total_parts, r.total_delivered_parts, r.total_undelivered_parts, r.total_unknown_parts, r.status, r.metadata, r.created_at
FROM sms_status_results r
JOIN campaign_status_jobs j ON j.id = r.job_id
JOIN active_sms_handoff h ON h.processed_campaign_id = r.processed_campaign_id AND h.provider = 'candoo'
WHERE j.platform = 'sms'
  AND COALESCE(NULLIF(BTRIM(j.provider), ''), 'payamsms') = 'candoo'
ON CONFLICT (id) DO NOTHING;

INSERT INTO payam_sms_send_attempts (processed_campaign_id, tracking_ids, http_status_code, response_headers, response_body, error, attempt_count, created_at)
SELECT a.processed_campaign_id, a.tracking_ids, a.http_status_code, a.response_headers, a.response_body, a.error, a.attempt_count, a.created_at
FROM payam_sms_send_responses a JOIN active_sms_handoff h ON h.processed_campaign_id = a.processed_campaign_id AND h.provider = 'payamsms';
INSERT INTO candoo_sms_send_attempts (processed_campaign_id, tracking_ids, http_status_code, response_headers, response_body, error, attempt_count, created_at)
SELECT a.processed_campaign_id, a.tracking_ids, a.http_status_code, a.response_headers, a.response_body, a.error, a.attempt_count, a.created_at
FROM sms_provider_send_attempts a JOIN active_sms_handoff h ON h.processed_campaign_id = a.processed_campaign_id AND h.provider = 'candoo'
WHERE a.provider = 'candoo';

SELECT setval(pg_get_serial_sequence('payam_processed_campaigns', 'id'), COALESCE((SELECT MAX(id) FROM payam_processed_campaigns), 1), true);
SELECT setval(pg_get_serial_sequence('candoo_processed_campaigns', 'id'), COALESCE((SELECT MAX(id) FROM candoo_processed_campaigns), 1), true);
SELECT setval(pg_get_serial_sequence('payam_sent_sms', 'id'), COALESCE((SELECT MAX(id) FROM payam_sent_sms), 1), true);
SELECT setval(pg_get_serial_sequence('candoo_sent_sms', 'id'), COALESCE((SELECT MAX(id) FROM candoo_sent_sms), 1), true);
SELECT setval(pg_get_serial_sequence('payam_sms_status_jobs', 'id'), COALESCE((SELECT MAX(id) FROM payam_sms_status_jobs), 1), true);
SELECT setval(pg_get_serial_sequence('candoo_sms_status_jobs', 'id'), COALESCE((SELECT MAX(id) FROM candoo_sms_status_jobs), 1), true);
SELECT setval(pg_get_serial_sequence('payam_sms_status_results', 'id'), COALESCE((SELECT MAX(id) FROM payam_sms_status_results), 1), true);
SELECT setval(pg_get_serial_sequence('candoo_sms_status_results', 'id'), COALESCE((SELECT MAX(id) FROM candoo_sms_status_results), 1), true);
SELECT setval('candoo_scheduler_customer_id_seq', COALESCE((SELECT MAX(provider_customer_id) FROM candoo_sent_sms), 0) + 1, false);

COMMIT;
