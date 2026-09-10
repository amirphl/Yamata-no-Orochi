-- Select the SMS gateway per sender line and retain that choice for durable
-- send/status processing. Existing data was sent through PayamSMS.
BEGIN;

ALTER TABLE line_numbers
    ADD COLUMN IF NOT EXISTS provider VARCHAR(32) NOT NULL DEFAULT 'payamsms';

UPDATE line_numbers
SET provider = 'payamsms'
WHERE provider IS NULL OR BTRIM(provider) = '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_line_numbers_provider'
          AND conrelid = 'line_numbers'::regclass
    ) THEN
        ALTER TABLE line_numbers
            ADD CONSTRAINT chk_line_numbers_provider
            CHECK (provider IN ('payamsms', 'candoo'));
    END IF;
END
$$;

ALTER TABLE sent_sms
    ADD COLUMN IF NOT EXISTS provider VARCHAR(32) NOT NULL DEFAULT 'payamsms',
    ADD COLUMN IF NOT EXISTS provider_customer_id BIGINT NULL;

UPDATE sent_sms
SET provider = 'payamsms'
WHERE provider IS NULL OR BTRIM(provider) = '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_sent_sms_provider'
          AND conrelid = 'sent_sms'::regclass
    ) THEN
        ALTER TABLE sent_sms
            ADD CONSTRAINT chk_sent_sms_provider
            CHECK (provider IN ('payamsms', 'candoo'));
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_sent_sms_provider_customer_id
    ON sent_sms (provider, provider_customer_id)
    WHERE provider_customer_id IS NOT NULL;

ALTER TABLE campaign_status_jobs
    ADD COLUMN IF NOT EXISTS provider VARCHAR(32) NULL;

UPDATE campaign_status_jobs
SET provider = 'payamsms'
WHERE platform = 'sms'
  AND (provider IS NULL OR BTRIM(provider) = '');

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_campaign_status_jobs_sms_provider'
          AND conrelid = 'campaign_status_jobs'::regclass
    ) THEN
        ALTER TABLE campaign_status_jobs
            ADD CONSTRAINT chk_campaign_status_jobs_sms_provider
            CHECK (provider IS NULL OR provider IN ('payamsms', 'candoo'));
    END IF;
END
$$;

ALTER TABLE sms_status_results
    ADD COLUMN IF NOT EXISTS provider VARCHAR(32) NOT NULL DEFAULT 'payamsms',
    ADD COLUMN IF NOT EXISTS provider_status_code TEXT NULL,
    ADD COLUMN IF NOT EXISTS provider_status_text TEXT NULL,
    ADD COLUMN IF NOT EXISTS internal_status sent_sms_status NULL,
    ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE sms_status_results
SET provider = 'payamsms'
WHERE provider IS NULL OR BTRIM(provider) = '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_sms_status_results_provider'
          AND conrelid = 'sms_status_results'::regclass
    ) THEN
        ALTER TABLE sms_status_results
            ADD CONSTRAINT chk_sms_status_results_provider
            CHECK (provider IN ('payamsms', 'candoo'));
    END IF;
END
$$;

CREATE SEQUENCE IF NOT EXISTS candoo_customer_id_seq
    AS BIGINT
    MINVALUE 1
    START WITH 1;

CREATE TABLE IF NOT EXISTS sms_provider_send_attempts (
    id BIGSERIAL PRIMARY KEY,
    processed_campaign_id BIGINT NOT NULL REFERENCES processed_campaigns(id) ON DELETE CASCADE,
    provider VARCHAR(32) NOT NULL,
    tracking_ids TEXT[] NOT NULL,
    http_status_code INTEGER NULL,
    response_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    response_body TEXT NULL,
    error TEXT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    CONSTRAINT chk_sms_provider_send_attempts_provider CHECK (provider IN ('payamsms', 'candoo')),
    CONSTRAINT sms_provider_send_attempts_attempt_count_nonnegative CHECK (attempt_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_sms_provider_send_attempts_campaign
    ON sms_provider_send_attempts (processed_campaign_id, id);
CREATE INDEX IF NOT EXISTS idx_sms_provider_send_attempts_provider_created
    ON sms_provider_send_attempts (provider, created_at);

COMMIT;
