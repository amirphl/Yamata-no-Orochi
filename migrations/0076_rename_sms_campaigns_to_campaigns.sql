-- Migration: 0076_rename_sms_campaigns_to_campaigns.sql
-- Description: Rename sms_campaigns table to campaigns and fix related index/constraint names.

BEGIN;

DO $$
BEGIN
    IF to_regclass('public.sms_campaigns') IS NOT NULL AND to_regclass('public.campaigns') IS NULL THEN
        ALTER TABLE public.sms_campaigns RENAME TO campaigns;
    END IF;
END
$$;

DO $$
BEGIN
    IF to_regclass('public.sms_campaigns_id_seq') IS NOT NULL AND to_regclass('public.campaigns_id_seq') IS NULL THEN
        ALTER SEQUENCE public.sms_campaigns_id_seq RENAME TO campaigns_id_seq;
    END IF;
END
$$;

DO $$
BEGIN
    IF to_regclass('public.campaigns') IS NOT NULL THEN
        IF EXISTS (
            SELECT 1
            FROM pg_constraint
            WHERE conname = 'sms_campaigns_pkey'
              AND conrelid = 'public.campaigns'::regclass
        ) THEN
            ALTER TABLE public.campaigns RENAME CONSTRAINT sms_campaigns_pkey TO campaigns_pkey;
        END IF;

        IF EXISTS (
            SELECT 1
            FROM pg_constraint
            WHERE conname = 'idx_sms_campaigns_uuid'
              AND conrelid = 'public.campaigns'::regclass
        ) THEN
            ALTER TABLE public.campaigns RENAME CONSTRAINT idx_sms_campaigns_uuid TO uk_campaigns_uuid;
        END IF;

        IF EXISTS (
            SELECT 1
            FROM pg_constraint
            WHERE conname = 'fk_sms_campaigns_customer'
              AND conrelid = 'public.campaigns'::regclass
        ) THEN
            ALTER TABLE public.campaigns RENAME CONSTRAINT fk_sms_campaigns_customer TO fk_campaigns_customer;
        END IF;
    END IF;
END
$$;

DO $$
BEGIN
    IF to_regclass('public.idx_sms_campaigns_customer_id') IS NOT NULL
       AND to_regclass('public.idx_campaigns_customer_id') IS NULL THEN
        ALTER INDEX public.idx_sms_campaigns_customer_id RENAME TO idx_campaigns_customer_id;
    END IF;

    IF to_regclass('public.idx_sms_campaigns_status') IS NOT NULL
       AND to_regclass('public.idx_campaigns_status') IS NULL THEN
        ALTER INDEX public.idx_sms_campaigns_status RENAME TO idx_campaigns_status;
    END IF;

    IF to_regclass('public.idx_sms_campaigns_created_at') IS NOT NULL
       AND to_regclass('public.idx_campaigns_created_at') IS NULL THEN
        ALTER INDEX public.idx_sms_campaigns_created_at RENAME TO idx_campaigns_created_at;
    END IF;

    IF to_regclass('public.idx_sms_campaigns_updated_at') IS NOT NULL
       AND to_regclass('public.idx_campaigns_updated_at') IS NULL THEN
        ALTER INDEX public.idx_sms_campaigns_updated_at RENAME TO idx_campaigns_updated_at;
    END IF;

    IF to_regclass('public.idx_sms_campaigns_uuid') IS NOT NULL
       AND to_regclass('public.idx_campaigns_uuid') IS NULL THEN
        ALTER INDEX public.idx_sms_campaigns_uuid RENAME TO idx_campaigns_uuid;
    END IF;
END
$$;

COMMIT;
