-- Keep redirect writes available during rollback too.
DROP INDEX CONCURRENTLY IF EXISTS idx_short_link_clicks_campaign_phone;

BEGIN;

DROP INDEX IF EXISTS idx_campaigns_smart_test_bundle;
DROP INDEX IF EXISTS idx_sent_rubika_processed_phone_tracking;
DROP INDEX IF EXISTS idx_sent_splus_processed_phone_tracking;
DROP INDEX IF EXISTS idx_sent_bale_processed_phone_tracking;
DROP INDEX IF EXISTS idx_sent_sms_processed_phone_tracking;
DROP INDEX IF EXISTS idx_processed_campaigns_campaign_selection;
DROP TABLE IF EXISTS tag_test_performance_scheduler_state;
DROP TABLE IF EXISTS tag_test_phase_performance_summaries;
DROP TABLE IF EXISTS campaign_tag_test_performances;
DROP TABLE IF EXISTS campaign_tag_test_reports;

COMMIT;
