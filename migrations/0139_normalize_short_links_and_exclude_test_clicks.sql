-- Canonical short-link metadata and explicit campaign test-send attribution.

BEGIN;

ALTER TABLE short_links
    ADD COLUMN IF NOT EXISTS is_test BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE short_link_clicks
    ADD COLUMN IF NOT EXISTS is_test BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_short_links_test
    ON short_links (is_test)
    WHERE is_test = TRUE;

CREATE INDEX IF NOT EXISTS idx_short_link_clicks_test
    ON short_link_clicks (is_test)
    WHERE is_test = TRUE;

-- Campaign test-sends substitute a UUID-derived test-xxxxxxxx audience token
-- into their destination. Mark historical rows so their clicks no longer enter
-- production campaign or tag-performance metrics.
UPDATE short_links
SET is_test = TRUE
WHERE is_test = FALSE
  AND long_link ~ '(^|[^[:alnum:]_-])test-[0-9a-f]{8}([^[:alnum:]_-]|$)';

UPDATE short_link_clicks AS click
SET is_test = TRUE
FROM short_links AS link
WHERE click.short_link_id = link.id
  AND link.is_test = TRUE
  AND click.is_test = FALSE;

-- Store known public short-link domains as canonical HTTPS URLs. This affects
-- metadata and exports only; SMS message text remains scheme-less.
UPDATE short_links
SET short_link = 'https://' || short_link
WHERE short_link ~ '^(jzbe\.ir|jo1n\.ir|joinsahel\.ir)/';

UPDATE short_link_clicks
SET short_link = 'https://' || short_link
WHERE short_link ~ '^(jzbe\.ir|jo1n\.ir|joinsahel\.ir)/';

ALTER TABLE campaign_tag_test_reports
    ALTER COLUMN calculation_version SET DEFAULT 3;
ALTER TABLE campaign_tag_test_performances
    ALTER COLUMN calculation_version SET DEFAULT 3;
ALTER TABLE tag_test_phase_performance_summaries
    ALTER COLUMN calculation_version SET DEFAULT 3;
ALTER TABLE tag_overall_performance_summaries
    ALTER COLUMN calculation_version SET DEFAULT 3;

COMMIT;
