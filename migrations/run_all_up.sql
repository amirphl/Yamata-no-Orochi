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

\echo 'Running 0012_update_timestamp_defaults_to_utc.sql...'
\i migrations/0012_update_timestamp_defaults_to_utc.sql

\echo 'Running 0013_relax_name_validation.sql...'
\i migrations/0013_relax_name_validation.sql

\echo 'Running 0014_create_sms_campaigns.sql...'
\i migrations/0014_create_sms_campaigns.sql

\echo 'Running 0015_add_sms_campaign_audit_actions.sql...'
\i migrations/0015_add_sms_campaign_audit_actions.sql

\echo 'Running 0016_add_comment_to_sms_campaigns.sql...'
\i migrations/0016_add_comment_to_sms_campaigns.sql

\echo 'Running 0017_create_wallet_models.sql...'
\i migrations/0017_create_wallet_models.sql

\echo 'Running 0018_create_agency_commission_models.sql...'
\i migrations/0018_create_agency_commission_models.sql

\echo 'Running 0019_add_payment_audit_actions.sql...'
\i migrations/0019_add_payment_audit_actions.sql

\echo 'Running 0020_create_tax_wallet.sql...'
\i migrations/0020_create_tax_wallet.sql

\echo 'Running 0021_change_agency_referer_code_to_varchar.sql...'
\i migrations/0021_change_agency_referer_code_to_varchar.sql

\echo 'Running 0022_create_agency_discounts.sql...'
\i migrations/0022_create_agency_discounts.sql

\echo 'Running 0023_add_credit_balance_to_balance_snapshots.sql...'
\i migrations/0023_add_credit_balance_to_balance_snapshots.sql

\echo 'Running 0024_create_sheba_number_on_customers.sql...'
\i migrations/0024_create_sheba_number_on_customers.sql

\echo 'Running 0024_create_system_company_and_wallet.sql...'
\i migrations/0024_create_system_company_and_wallet.sql

\echo 'Running 0025_add_indexes_on_transactions_metadata.sql...'
\i migrations/0025_add_indexes_on_transactions_metadata.sql

\echo 'Running 0026_add_index_on_transactions_metadata_agency_discount_id.sql...'
\i migrations/0026_add_index_on_transactions_metadata_agency_discount_id.sql

\echo 'Running 0027_add_discount_audit_actions.sql...'
\i migrations/0027_add_discount_audit_actions.sql

\echo 'Running 0028_add_launch_campaign_transaction_type.sql...'
\i migrations/0028_add_launch_campaign_transaction_type.sql

\echo 'Running 0029_create_admins.sql...'
\i migrations/0029_create_admins.sql

\echo 'Running 0030_create_line_numbers.sql...'
\i migrations/0030_create_line_numbers.sql

\echo 'Running 0031_remove_hardcoded_system_tax_data.sql...'
\i migrations/0031_remove_hardcoded_system_tax_data.sql

\echo 'Running 0032_add_updated_at_to_customer_sessions.sql...'
\i migrations/0032_add_updated_at_to_customer_sessions.sql

\echo 'Running 0033_create_bots.sql...'
\i migrations/0033_create_bots.sql

\echo 'Running 0034_create_audience_profiles.sql...'
\i migrations/0034_create_audience_profiles.sql

\echo 'Running 0035_add_running_executed_to_sms_campaign_status.sql...'
\i migrations/0035_add_running_executed_to_sms_campaign_status.sql

\echo 'Running 0036_create_processed_campaigns.sql...'
\i migrations/0036_create_processed_campaigns.sql

\echo 'Running 0037_create_sent_sms.sql...'
\i migrations/0037_create_sent_sms.sql

\echo 'Running 0038_create_tags.sql...'
\i migrations/0038_create_tags.sql

\echo 'Running 0039_add_num_target_after_finalize_to_sms_campaigns.sql...'
\i migrations/0039_add_num_target_after_finalize_to_sms_campaigns.sql

\echo 'Running 0040_create_tickets.sql...'
\i migrations/0040_create_tickets.sql

\echo 'Running 0041_add_replied_by_admin_to_tickets.sql...'
\i migrations/0041_add_replied_by_admin_to_tickets.sql

\echo 'Running 0042_add_provider_fields_to_sent_sms.sql...'
\i migrations/0042_add_provider_fields_to_sent_sms.sql

\echo 'Running 0043_create_short_links.sql...'
\i migrations/0043_create_short_links.sql

\echo 'All migrations completed successfully!'
\echo 'Database schema is now ready for the Yamata no Orochi wallet, payment, and agency commission system with comprehensive audit logging and tax collection.' 