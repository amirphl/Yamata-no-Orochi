-- Rename campaign_status_jobs back to sms_status_jobs
BEGIN;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'campaign_status_jobs' AND relkind = 'r'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'sms_status_jobs' AND relkind = 'r'
    ) THEN
        ALTER TABLE campaign_status_jobs RENAME TO sms_status_jobs;
    END IF;
END
$$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'campaign_status_jobs_pkey' AND relkind = 'i'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'sms_status_jobs_pkey' AND relkind = 'i'
    ) THEN
        ALTER INDEX campaign_status_jobs_pkey RENAME TO sms_status_jobs_pkey;
    END IF;
END
$$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'idx_campaign_status_jobs_processed_campaign_id' AND relkind = 'i'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'idx_sms_status_jobs_processed_campaign_id' AND relkind = 'i'
    ) THEN
        ALTER INDEX idx_campaign_status_jobs_processed_campaign_id RENAME TO idx_sms_status_jobs_processed_campaign_id;
    END IF;
END
$$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'idx_campaign_status_jobs_scheduled_retry' AND relkind = 'i'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'idx_sms_status_jobs_scheduled_retry' AND relkind = 'i'
    ) THEN
        ALTER INDEX idx_campaign_status_jobs_scheduled_retry RENAME TO idx_sms_status_jobs_scheduled_retry;
    END IF;
END
$$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'idx_campaign_status_jobs_corr_id' AND relkind = 'i'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'idx_sms_status_jobs_corr_id' AND relkind = 'i'
    ) THEN
        ALTER INDEX idx_campaign_status_jobs_corr_id RENAME TO idx_sms_status_jobs_corr_id;
    END IF;
END
$$;

COMMIT;
