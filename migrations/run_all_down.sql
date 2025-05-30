-- run_all_down.sql
-- Convenience script to rollback all migrations in reverse order

\echo 'Starting database rollback...'

\echo 'Running 0016_add_comment_to_sms_campaigns_down.sql...'
\i migrations/0016_add_comment_to_sms_campaigns_down.sql

\echo 'Running 0015_add_sms_campaign_audit_actions_down.sql...'
\i migrations/0015_add_sms_campaign_audit_actions_down.sql

\echo 'Running 0014_create_sms_campaigns_down.sql...'
\i migrations/0014_create_sms_campaigns_down.sql

\echo 'Running 0013_relax_name_validation_down.sql...'
\i migrations/0013_relax_name_validation_down.sql

\echo 'Running 0012_update_timestamp_defaults_to_utc_down.sql...'
\i migrations/0012_update_timestamp_defaults_to_utc_down.sql

\echo 'Running 0011_add_new_audit_actions_down.sql...'
\i migrations/0011_add_new_audit_actions_down.sql

\echo 'Running 0010_add_customer_uuid_and_agency_id_down.sql...'
\i migrations/0010_add_customer_uuid_and_agency_id_down.sql

\echo 'Running 0009_add_correlation_ids_down.sql...'
\i migrations/0009_add_correlation_ids_down.sql

\echo 'Running 0008_update_audit_log_success_field_down.sql...'
\i migrations/0008_update_audit_log_success_field_down.sql

\echo 'Running 0007_add_missing_audit_actions_down.sql...'
\i migrations/0007_add_missing_audit_actions_down.sql

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