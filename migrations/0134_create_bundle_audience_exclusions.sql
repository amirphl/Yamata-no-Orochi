-- Bundle-scoped opt-out list for Smart Targeting Test audience eligibility.
-- The natural pair key both prevents duplicates and supports the
-- capacity/preview/scheduler bundle_id + audience_id anti-join.

BEGIN;

CREATE TABLE IF NOT EXISTS bundle_audience_exclusions (
    bundle_id INTEGER NOT NULL REFERENCES bundles(id) ON DELETE CASCADE,
    audience_id BIGINT NOT NULL REFERENCES audience_profiles(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT bundle_audience_exclusions_pkey PRIMARY KEY (bundle_id, audience_id)
);

-- PostgreSQL does not create an index for the referencing side of a foreign
-- key. This keeps audience deletion/cascade checks index-backed; the primary
-- key already covers the scheduler's bundle-first lookup.
CREATE INDEX IF NOT EXISTS idx_bundle_audience_exclusions_audience
    ON bundle_audience_exclusions (audience_id);

COMMENT ON TABLE bundle_audience_exclusions IS
    'Audience IDs excluded from Smart Targeting Test eligibility for a bundle';

COMMIT;
