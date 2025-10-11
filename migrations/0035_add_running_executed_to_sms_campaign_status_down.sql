-- Migration: 0035_add_running_executed_to_sms_campaign_status_down.sql
-- Description: Down migration for adding 'running' and 'executed' to sms_campaign_status enum

-- DOWN MIGRATION
-- Note: PostgreSQL does not support removing enum labels prior to v13 without hacks,
-- and removing used enum values is unsafe. This down migration is a no-op.
-- If necessary, you can recreate the type and table in a controlled environment.

-- NO-OP 