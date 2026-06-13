-- Migration: 0090_add_expired_status_to_sms_campaigns_down.sql
-- Description: Down migration for adding 'expired' to sms_campaign_status enum (no-op)

-- PostgreSQL enums do not support dropping a value safely in place.
-- No-op by design.
