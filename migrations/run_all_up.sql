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

\echo 'Running 0007_add_missing_audit_actions.sql...'
\i migrations/0007_add_missing_audit_actions.sql

\echo 'Running 0008_update_audit_log_success_field.sql...'
\i migrations/0008_update_audit_log_success_field.sql

\echo 'Running 0009_add_correlation_ids.sql...'
\i migrations/0009_add_correlation_ids.sql

\echo 'Running 0010_add_customer_uuid_and_agency_id.sql...'
\i migrations/0010_add_customer_uuid_and_agency_id.sql

\echo 'Running 0011_add_new_audit_actions.sql...'
\i migrations/0011_add_new_audit_actions.sql

\echo 'All migrations completed successfully!'
\echo 'Database schema is now ready for the Yamata no Orochi signup system.' 