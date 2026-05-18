-- Down Migration: restore FK constraint on audit_log.customer_id
BEGIN;
ALTER TABLE audit_log
  ADD CONSTRAINT audit_log_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE SET NULL;
COMMIT;
