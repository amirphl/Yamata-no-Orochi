-- Migration: 0077_drop_audit_log_customer_fk.sql
-- Description: Remove FK constraint on audit_log.customer_id to allow anonymous audit logs

BEGIN;
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_customer_id_fkey;
COMMIT;
