BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM sent_sms WHERE provider = 'candoo')
       OR EXISTS (SELECT 1 FROM campaign_status_jobs WHERE platform = 'sms' AND provider = 'candoo')
       OR EXISTS (SELECT 1 FROM sms_status_results WHERE provider = 'candoo')
       OR EXISTS (SELECT 1 FROM sms_provider_send_attempts) THEN
        RAISE EXCEPTION 'cannot roll back 0137 while SMS provider send or status history exists';
    END IF;
END
$$;

DROP TABLE IF EXISTS sms_provider_send_attempts;
DROP SEQUENCE IF EXISTS candoo_customer_id_seq;

ALTER TABLE sms_status_results
    DROP CONSTRAINT IF EXISTS chk_sms_status_results_provider,
    DROP COLUMN IF EXISTS metadata,
    DROP COLUMN IF EXISTS internal_status,
    DROP COLUMN IF EXISTS provider_status_text,
    DROP COLUMN IF EXISTS provider_status_code,
    DROP COLUMN IF EXISTS provider;

ALTER TABLE campaign_status_jobs
    DROP CONSTRAINT IF EXISTS chk_campaign_status_jobs_sms_provider,
    DROP COLUMN IF EXISTS provider;

DROP INDEX IF EXISTS idx_sent_sms_provider_customer_id;
ALTER TABLE sent_sms
    DROP CONSTRAINT IF EXISTS chk_sent_sms_provider,
    DROP COLUMN IF EXISTS provider_customer_id,
    DROP COLUMN IF EXISTS provider;

ALTER TABLE line_numbers
    DROP CONSTRAINT IF EXISTS chk_line_numbers_provider,
    DROP COLUMN IF EXISTS provider;

COMMIT;
