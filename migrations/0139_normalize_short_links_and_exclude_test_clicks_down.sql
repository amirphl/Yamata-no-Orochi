BEGIN;

ALTER TABLE campaign_tag_test_reports
    ALTER COLUMN calculation_version SET DEFAULT 2;
ALTER TABLE campaign_tag_test_performances
    ALTER COLUMN calculation_version SET DEFAULT 2;
ALTER TABLE tag_test_phase_performance_summaries
    ALTER COLUMN calculation_version SET DEFAULT 2;
ALTER TABLE tag_overall_performance_summaries
    ALTER COLUMN calculation_version SET DEFAULT 2;

DROP INDEX IF EXISTS idx_short_link_clicks_test;
DROP INDEX IF EXISTS idx_short_links_test;

ALTER TABLE short_link_clicks
    DROP COLUMN IF EXISTS is_test;
ALTER TABLE short_links
    DROP COLUMN IF EXISTS is_test;

COMMIT;
