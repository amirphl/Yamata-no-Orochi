-- Feature 5: materialized Smart Targeting Test-phase tag CTR reports.

BEGIN;

CREATE TABLE IF NOT EXISTS campaign_tag_test_reports (
    campaign_id BIGINT PRIMARY KEY REFERENCES campaigns(id) ON DELETE CASCADE,
    bundle_id BIGINT NOT NULL REFERENCES bundles(id),
    status VARCHAR(32) NOT NULL DEFAULT 'not_prepared',
    calculation_version INTEGER NOT NULL DEFAULT 1,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    next_retry_at TIMESTAMPTZ,
    last_calculated_click_id BIGINT NOT NULL DEFAULT 0,
    error_code VARCHAR(64),
    error_message VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT campaign_tag_test_report_status_valid CHECK (
        status IN ('not_prepared', 'preparing', 'prepared', 'failed')
    ),
    CONSTRAINT campaign_tag_test_report_version_positive CHECK (calculation_version > 0),
    CONSTRAINT campaign_tag_test_report_attempt_count_nonnegative CHECK (attempt_count >= 0),
    CONSTRAINT campaign_tag_test_report_click_cursor_nonnegative CHECK (last_calculated_click_id >= 0),
    CONSTRAINT campaign_tag_test_report_preparing_started CHECK (
        status <> 'preparing' OR started_at IS NOT NULL
    ),
    CONSTRAINT campaign_tag_test_report_terminal_finished CHECK (
        status NOT IN ('prepared', 'failed') OR finished_at IS NOT NULL
    )
);

CREATE INDEX IF NOT EXISTS idx_campaign_tag_test_reports_bundle_status
    ON campaign_tag_test_reports (bundle_id, status);
CREATE INDEX IF NOT EXISTS idx_campaign_tag_test_reports_pending
    ON campaign_tag_test_reports (calculation_version, status, next_retry_at, requested_at, campaign_id)
    WHERE status IN ('not_prepared', 'preparing', 'failed');

CREATE TABLE IF NOT EXISTS campaign_tag_test_performances (
    id BIGSERIAL PRIMARY KEY,
    campaign_id BIGINT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    bundle_id BIGINT NOT NULL REFERENCES bundles(id),
    tag_id BIGINT NOT NULL REFERENCES tags(id),
    tag_display_title_snapshot TEXT NOT NULL,
    bundle_persona_fit_score_snapshot NUMERIC(5, 2),
    selected_count BIGINT NOT NULL,
    sent_count BIGINT NOT NULL,
    delivered_count BIGINT NOT NULL,
    click_count BIGINT NOT NULL,
    test_campaign_ctr NUMERIC GENERATED ALWAYS AS (
        CASE
            WHEN delivered_count = 0 THEN NULL
            ELSE click_count::NUMERIC / delivered_count::NUMERIC
        END
    ) STORED,
    calculation_version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_campaign_tag_test_performance UNIQUE (campaign_id, tag_id),
    CONSTRAINT campaign_tag_test_performance_fit_score_range CHECK (
        bundle_persona_fit_score_snapshot IS NULL
        OR bundle_persona_fit_score_snapshot BETWEEN 0 AND 100
    ),
    CONSTRAINT campaign_tag_test_performance_version_positive CHECK (calculation_version > 0),
    CONSTRAINT campaign_tag_test_performance_counts_nonnegative CHECK (
        selected_count >= 0 AND sent_count >= 0 AND delivered_count >= 0 AND click_count >= 0
    ),
    CONSTRAINT campaign_tag_test_performance_count_order_valid CHECK (
        delivered_count <= sent_count
        AND sent_count <= selected_count
        AND click_count <= selected_count
    )
);

CREATE INDEX IF NOT EXISTS idx_campaign_tag_test_performances_bundle_tag
    ON campaign_tag_test_performances (bundle_id, tag_id);
CREATE INDEX IF NOT EXISTS idx_campaign_tag_test_performances_tag
    ON campaign_tag_test_performances (tag_id);

CREATE TABLE IF NOT EXISTS tag_test_phase_performance_summaries (
    id BIGSERIAL PRIMARY KEY,
    bundle_id BIGINT NOT NULL REFERENCES bundles(id) ON DELETE CASCADE,
    tag_id BIGINT NOT NULL REFERENCES tags(id),
    total_test_selected_count BIGINT NOT NULL,
    total_test_sent_count BIGINT NOT NULL,
    total_test_delivered_count BIGINT NOT NULL,
    total_test_click_count BIGINT NOT NULL,
    test_phase_avg_ctr NUMERIC GENERATED ALWAYS AS (
        CASE
            WHEN total_test_delivered_count = 0 THEN NULL
            ELSE total_test_click_count::NUMERIC / total_test_delivered_count::NUMERIC
        END
    ) STORED,
    calculation_version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_tag_test_phase_performance_bundle_tag UNIQUE (bundle_id, tag_id),
    CONSTRAINT tag_test_phase_performance_version_positive CHECK (calculation_version > 0),
    CONSTRAINT tag_test_phase_performance_counts_nonnegative CHECK (
        total_test_selected_count >= 0
        AND total_test_sent_count >= 0
        AND total_test_delivered_count >= 0
        AND total_test_click_count >= 0
    ),
    CONSTRAINT tag_test_phase_performance_count_order_valid CHECK (
        total_test_delivered_count <= total_test_sent_count
        AND total_test_sent_count <= total_test_selected_count
        AND total_test_click_count <= total_test_selected_count
    )
);

CREATE INDEX IF NOT EXISTS idx_tag_test_phase_performance_tag
    ON tag_test_phase_performance_summaries (tag_id);

-- A singleton cursor makes click/status discovery durable. It is locked only
-- while scheduler work is enqueued; click tables are never explicitly locked.
CREATE TABLE IF NOT EXISTS tag_test_performance_scheduler_state (
    id SMALLINT PRIMARY KEY,
    last_click_id BIGINT NOT NULL DEFAULT 0,
    last_source_scan_at TIMESTAMPTZ NOT NULL DEFAULT TIMESTAMPTZ '1970-01-01 00:00:00+00',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT tag_test_performance_scheduler_singleton CHECK (id = 1),
    CONSTRAINT tag_test_performance_scheduler_click_cursor_nonnegative CHECK (last_click_id >= 0)
);

INSERT INTO tag_test_performance_scheduler_state (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;

-- Attribution and recipient-status joins are the high-volume report paths.
-- Feature 4 already provides (campaign_id, assigned_tag_id), which is the
-- report's access order; do not add a redundant reversed index here.
CREATE INDEX IF NOT EXISTS idx_processed_campaigns_campaign_selection
    ON processed_campaigns (campaign_id, bundle_audience_selection_id)
    WHERE bundle_audience_selection_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sent_sms_processed_phone_tracking
    ON sent_sms (processed_campaign_id, phone_number, tracking_id);
CREATE INDEX IF NOT EXISTS idx_sent_bale_processed_phone_tracking
    ON sent_bale_messages (processed_campaign_id, phone_number, tracking_id);
CREATE INDEX IF NOT EXISTS idx_sent_splus_processed_phone_tracking
    ON sent_splus_messages (processed_campaign_id, phone_number, tracking_id);
CREATE INDEX IF NOT EXISTS idx_sent_rubika_processed_phone_tracking
    ON sent_rubika_messages (processed_campaign_id, phone_number, tracking_id);
CREATE INDEX IF NOT EXISTS idx_campaigns_smart_test_bundle
    ON campaigns (bundle_id, id)
    WHERE phase = 'test'
      AND LOWER(BTRIM(COALESCE(spec->>'audience_targeting_method', ''))) = 'smart_targeting';

COMMIT;

-- Redirect handling writes this table on the product's latency-critical path.
-- Build its report index concurrently so the migration never blocks inserts.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_short_link_clicks_campaign_phone
    ON short_link_clicks (campaign_id, phone_number)
    WHERE campaign_id IS NOT NULL AND phone_number IS NOT NULL;
