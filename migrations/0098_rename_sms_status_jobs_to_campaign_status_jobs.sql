-- Rename sms_status_jobs to campaign_status_jobs for cross-platform status jobs
BEGIN;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'sms_status_jobs' AND relkind = 'r'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'campaign_status_jobs' AND relkind = 'r'
    ) THEN
        ALTER TABLE sms_status_jobs RENAME TO campaign_status_jobs;
    END IF;
END
$$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'sms_status_jobs_pkey' AND relkind = 'i'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'campaign_status_jobs_pkey' AND relkind = 'i'
    ) THEN
        ALTER INDEX sms_status_jobs_pkey RENAME TO campaign_status_jobs_pkey;
    END IF;
END
$$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'idx_sms_status_jobs_processed_campaign_id' AND relkind = 'i'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'idx_campaign_status_jobs_processed_campaign_id' AND relkind = 'i'
    ) THEN
        ALTER INDEX idx_sms_status_jobs_processed_campaign_id RENAME TO idx_campaign_status_jobs_processed_campaign_id;
    END IF;
END
$$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'idx_sms_status_jobs_scheduled_retry' AND relkind = 'i'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'idx_campaign_status_jobs_scheduled_retry' AND relkind = 'i'
    ) THEN
        ALTER INDEX idx_sms_status_jobs_scheduled_retry RENAME TO idx_campaign_status_jobs_scheduled_retry;
    END IF;
END
$$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'idx_sms_status_jobs_corr_id' AND relkind = 'i'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'idx_campaign_status_jobs_corr_id' AND relkind = 'i'
    ) THEN
        ALTER INDEX idx_sms_status_jobs_corr_id RENAME TO idx_campaign_status_jobs_corr_id;
    END IF;
END
$$;

COMMIT;
