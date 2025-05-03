-- run_all_down.sql
-- Convenience script to rollback all migrations in reverse order

\echo 'Starting database rollback...'

\echo 'Running 0006_update_customer_fields_down.sql...'
\i migrations/0006_update_customer_fields_down.sql

\echo 'Running 0005_create_audit_log_down.sql...'
\i migrations/0005_create_audit_log_down.sql

\echo 'Running 0004_create_customer_sessions_down.sql...'
\i migrations/0004_create_customer_sessions_down.sql

\echo 'Running 0003_create_otp_verifications_down.sql...'
\i migrations/0003_create_otp_verifications_down.sql

\echo 'Running 0002_create_customers_down.sql...'
\i migrations/0002_create_customers_down.sql

\echo 'Running 0001_create_account_types_down.sql...'
\i migrations/0001_create_account_types_down.sql

\echo 'All migrations rolled back successfully!'
\echo 'Database schema has been completely removed.' 