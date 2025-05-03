-- run_all_up.sql
-- Convenience script to apply all migrations in correct order

\echo 'Starting database migration...'

\echo 'Running 0001_create_account_types.sql...'
\i migrations/0001_create_account_types.sql

\echo 'Running 0002_create_customers.sql...'
\i migrations/0002_create_customers.sql

\echo 'Running 0003_create_otp_verifications.sql...'
\i migrations/0003_create_otp_verifications.sql

\echo 'Running 0004_create_customer_sessions.sql...'
\i migrations/0004_create_customer_sessions.sql

\echo 'Running 0005_create_audit_log.sql...'
\i migrations/0005_create_audit_log.sql

\echo 'Running 0006_update_customer_fields.sql...'
\i migrations/0006_update_customer_fields.sql

\echo 'All migrations completed successfully!'
\echo 'Database schema is now ready for the Yamata no Orochi signup system.' 