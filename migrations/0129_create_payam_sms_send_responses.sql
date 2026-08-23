-- Store the immediate PayamSMS batch response for campaign failure analysis.
BEGIN;

CREATE TABLE IF NOT EXISTS payam_sms_send_responses (
    id BIGSERIAL PRIMARY KEY,
    processed_campaign_id BIGINT NOT NULL REFERENCES processed_campaigns(id) ON DELETE CASCADE,
    tracking_ids TEXT[] NOT NULL,
    http_status_code INTEGER,
    response_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    response_body TEXT,
    error TEXT,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    CONSTRAINT payam_sms_send_responses_attempt_count_nonnegative CHECK (attempt_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_payam_sms_send_responses_campaign
    ON payam_sms_send_responses (processed_campaign_id, id);
CREATE INDEX IF NOT EXISTS idx_payam_sms_send_responses_created_at
    ON payam_sms_send_responses (created_at);

COMMIT;
