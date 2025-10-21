-- run_all_down.sql
-- Convenience script to rollback all migrations in reverse order

\echo 'Starting database rollback...'

\echo 'Running 0041_add_replied_by_admin_to_tickets_down.sql...'
\i migrations/0041_add_replied_by_admin_to_tickets_down.sql

\echo 'Running 0040_create_tickets_down.sql...'
\i migrations/0040_create_tickets_down.sql

\echo 'Running 0039_add_num_target_after_finalize_to_sms_campaigns_down.sql...'
\i migrations/0039_add_num_target_after_finalize_to_sms_campaigns_down.sql

\echo 'Running 0038_create_tags_down.sql...'
\i migrations/0038_create_tags_down.sql

\echo 'Running 0037_create_sent_sms_down.sql...'
\i migrations/0037_create_sent_sms_down.sql

\echo 'Running 0036_create_processed_campaigns_down.sql...'
\i migrations/0036_create_processed_campaigns_down.sql

\echo 'Running 0034_create_audience_profiles_down.sql...'
\i migrations/0034_create_audience_profiles_down.sql

\echo 'Running 0035_add_running_executed_to_sms_campaign_status_down.sql...'
\i migrations/0035_add_running_executed_to_sms_campaign_status_down.sql

\echo 'Running 0033_create_bots_down.sql...'
\i migrations/0033_create_bots_down.sql

\echo 'Running 0032_add_updated_at_to_customer_sessions_down.sql...'
\i migrations/0032_add_updated_at_to_customer_sessions_down.sql

\echo 'Running 0031_remove_hardcoded_system_tax_data_down.sql...'
\i migrations/0031_remove_hardcoded_system_tax_data_down.sql

\echo 'Running 0030_create_line_numbers_down.sql...'
\i migrations/0030_create_line_numbers_down.sql

\echo 'Running 0029_create_admins_down.sql...'
\i migrations/0029_create_admins_down.sql

\echo 'Running 0028_add_launch_campaign_transaction_type_down.sql...'
\i migrations/0028_add_launch_campaign_transaction_type_down.sql

\echo 'Running 0027_add_discount_audit_actions_down.sql...'
\i migrations/0027_add_discount_audit_actions_down.sql

\echo 'Running 0026_add_index_on_transactions_metadata_agency_discount_id_down.sql...'
\i migrations/0026_add_index_on_transactions_metadata_agency_discount_id_down.sql

\echo 'Running 0025_add_indexes_on_transactions_metadata_down.sql...'
\i migrations/0025_add_indexes_on_transactions_metadata_down.sql

\echo 'Running 0024_create_system_company_and_wallet_down.sql...'
\i migrations/0024_create_system_company_and_wallet_down.sql

\echo 'Running 0024_create_sheba_number_on_customers_down.sql...'
\i migrations/0024_create_sheba_number_on_customers_down.sql

\echo 'Running 0023_add_credit_balance_to_balance_snapshots_down.sql...'
\i migrations/0023_add_credit_balance_to_balance_snapshots_down.sql

\echo 'Running 0022_create_agency_discounts_down.sql...'
\i migrations/0022_create_agency_discounts_down.sql

\echo 'Running 0021_change_agency_referer_code_to_varchar_down.sql...'
\i migrations/0021_change_agency_referer_code_to_varchar_down.sql

\echo 'Running 0020_create_tax_wallet_down.sql...'
\i migrations/0020_create_tax_wallet_down.sql

\echo 'Running 0019_add_payment_audit_actions_down.sql...'
\i migrations/0019_add_payment_audit_actions_down.sql

\echo 'Running 0018_create_agency_commission_models_down.sql...'
\i migrations/0018_create_agency_commission_models_down.sql

\echo 'Running 0017_create_wallet_models_down.sql...'
\i migrations/0017_create_wallet_models_down.sql

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