-- Description: Create smart tag evaluation storage, views, and tag metadata columns

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE tags
    ADD COLUMN IF NOT EXISTS display_title TEXT,
    ADD COLUMN IF NOT EXISTS audience_persona TEXT,
    ADD COLUMN IF NOT EXISTS audience_count BIGINT;

CREATE TABLE IF NOT EXISTS bundle_tag_evaluation_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bundle_id BIGINT NOT NULL REFERENCES bundles(id),
    customer_id BIGINT NOT NULL REFERENCES customers(id),
    target_persona_snapshot TEXT NOT NULL,
    persona_analysis_prompt_snapshot TEXT NOT NULL,
    configuration_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    tag_batch_size INTEGER NOT NULL CHECK (tag_batch_size > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_bundle_tag_eval_runs_bundle_created
    ON bundle_tag_evaluation_runs (bundle_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_bundle_tag_eval_runs_customer_created
    ON bundle_tag_evaluation_runs (customer_id, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS bundle_tag_evaluation_events (
    id BIGSERIAL PRIMARY KEY,
    evaluation_run_id UUID NOT NULL REFERENCES bundle_tag_evaluation_runs(id),
    batch_id UUID NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_bundle_tag_eval_events_run_created
    ON bundle_tag_evaluation_events (evaluation_run_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_bundle_tag_eval_events_type_created
    ON bundle_tag_evaluation_events (event_type, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS bundle_tag_persona_analysis_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    evaluation_run_id UUID NOT NULL REFERENCES bundle_tag_evaluation_runs(id),
    attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
    request_payload JSONB NOT NULL,
    raw_response TEXT,
    extracted_response_text TEXT,
    http_status_code INTEGER,
    provider_response_id TEXT,
    model_name TEXT NOT NULL,
    usage_metadata JSONB,
    error_message TEXT,
    requested_at TIMESTAMPTZ NOT NULL,
    responded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT bundle_tag_persona_attempt_unique UNIQUE (evaluation_run_id, attempt_number)
);

CREATE INDEX IF NOT EXISTS idx_bundle_tag_persona_attempts_run_attempt
    ON bundle_tag_persona_analysis_attempts (evaluation_run_id, attempt_number DESC);

CREATE TABLE IF NOT EXISTS bundle_tag_evaluation_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    evaluation_run_id UUID NOT NULL REFERENCES bundle_tag_evaluation_runs(id),
    batch_number INTEGER NOT NULL CHECK (batch_number > 0),
    tag_count INTEGER NOT NULL CHECK (tag_count >= 0),
    first_tag_id BIGINT NOT NULL,
    last_tag_id BIGINT NOT NULL,
    tags_snapshot JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT bundle_tag_evaluation_batch_unique UNIQUE (evaluation_run_id, batch_number)
);

CREATE INDEX IF NOT EXISTS idx_bundle_tag_eval_batches_run_batch
    ON bundle_tag_evaluation_batches (evaluation_run_id, batch_number);

CREATE TABLE IF NOT EXISTS bundle_tag_evaluation_batch_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id UUID NOT NULL REFERENCES bundle_tag_evaluation_batches(id),
    attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
    request_payload JSONB NOT NULL,
    raw_response TEXT,
    http_status_code INTEGER,
    provider_response_id TEXT,
    model_name TEXT NOT NULL,
    usage_metadata JSONB,
    error_message TEXT,
    requested_at TIMESTAMPTZ NOT NULL,
    responded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT bundle_tag_batch_attempt_unique UNIQUE (batch_id, attempt_number)
);

CREATE INDEX IF NOT EXISTS idx_bundle_tag_eval_batch_attempts_batch_attempt
    ON bundle_tag_evaluation_batch_attempts (batch_id, attempt_number DESC);

ALTER TABLE bundle_tag_evaluation_events
    DROP CONSTRAINT IF EXISTS bundle_tag_evaluation_events_batch_fk;

ALTER TABLE bundle_tag_evaluation_events
    ADD CONSTRAINT bundle_tag_evaluation_events_batch_fk
    FOREIGN KEY (batch_id) REFERENCES bundle_tag_evaluation_batches(id);

CREATE TABLE IF NOT EXISTS bundle_tag_scores (
    id BIGSERIAL PRIMARY KEY,
    evaluation_run_id UUID NOT NULL REFERENCES bundle_tag_evaluation_runs(id),
    batch_id UUID NOT NULL REFERENCES bundle_tag_evaluation_batches(id),
    batch_attempt_id UUID NOT NULL REFERENCES bundle_tag_evaluation_batch_attempts(id),
    bundle_id BIGINT NOT NULL REFERENCES bundles(id),
    tag_id BIGINT NOT NULL REFERENCES tags(id),
    tag_name_snapshot TEXT,
    tag_display_title_snapshot TEXT,
    tag_persona_snapshot TEXT,
    tag_audience_count_snapshot BIGINT,
    bundle_fit_score NUMERIC(5, 2) NOT NULL CHECK (bundle_fit_score >= 0 AND bundle_fit_score <= 100),
    fit_level TEXT NOT NULL,
    relation_type TEXT NOT NULL,
    reason TEXT NOT NULL,
    raw_result JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT bundle_tag_scores_run_tag_unique UNIQUE (evaluation_run_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_bundle_tag_scores_bundle_tag_created
    ON bundle_tag_scores (bundle_id, tag_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_bundle_tag_scores_run_tag
    ON bundle_tag_scores (evaluation_run_id, tag_id);

CREATE INDEX IF NOT EXISTS idx_bundle_tag_scores_batch_tag
    ON bundle_tag_scores (batch_id, tag_id);

CREATE OR REPLACE FUNCTION smart_tag_normalize_text(input TEXT)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT btrim(replace(replace(COALESCE(input, ''), E'\r\n', E'\n'), E'\r', E'\n'));
$$;

CREATE OR REPLACE VIEW bundle_tag_evaluation_run_status AS
WITH latest_event AS (
    SELECT DISTINCT ON (event.evaluation_run_id)
        event.evaluation_run_id,
        event.event_type,
        event.payload,
        event.created_at
    FROM bundle_tag_evaluation_events AS event
    ORDER BY event.evaluation_run_id, event.created_at DESC, event.id DESC
)
SELECT
    run.id AS evaluation_run_id,
    run.bundle_id,
    run.customer_id,
    run.target_persona_snapshot,
    run.created_at AS evaluation_created_at,
    latest_event.event_type AS latest_event_type,
    latest_event.payload AS latest_event_payload,
    latest_event.created_at AS latest_event_at,
    CASE
        WHEN latest_event.event_type = 'evaluation_completed' THEN 'evaluated'
        WHEN latest_event.event_type = 'evaluation_failed' THEN 'error'
        WHEN latest_event.event_type = 'evaluation_created' THEN 'created'
        WHEN latest_event.event_type IS NULL THEN 'created'
        ELSE 'evaluating'
    END AS run_status
FROM bundle_tag_evaluation_runs AS run
LEFT JOIN latest_event
    ON latest_event.evaluation_run_id = run.id;

CREATE OR REPLACE VIEW current_bundle_tag_scores AS
WITH completed_runs AS (
    SELECT
        run.id AS evaluation_run_id,
        run.bundle_id,
        completion.created_at AS completed_at
    FROM bundle_tag_evaluation_runs AS run
    JOIN LATERAL (
        SELECT event.created_at
        FROM bundle_tag_evaluation_events AS event
        WHERE event.evaluation_run_id = run.id
          AND event.event_type = 'evaluation_completed'
        ORDER BY event.created_at DESC, event.id DESC
        LIMIT 1
    ) AS completion ON TRUE
),
latest_completed_run AS (
    SELECT DISTINCT ON (bundle_id)
        bundle_id,
        evaluation_run_id,
        completed_at
    FROM completed_runs
    ORDER BY bundle_id, completed_at DESC, evaluation_run_id DESC
)
SELECT score.*
FROM latest_completed_run AS latest
JOIN bundle_tag_scores AS score
    ON score.evaluation_run_id = latest.evaluation_run_id;

CREATE OR REPLACE VIEW current_bundle_tag_evaluation_status AS
WITH latest_run AS (
    SELECT DISTINCT ON (run.bundle_id)
        run.bundle_id,
        run.evaluation_run_id,
        run.run_status,
        run.latest_event_type,
        run.latest_event_payload,
        run.latest_event_at,
        run.evaluation_created_at
    FROM bundle_tag_evaluation_run_status AS run
    ORDER BY run.bundle_id, run.evaluation_created_at DESC, run.evaluation_run_id DESC
),
latest_successful_run AS (
    SELECT DISTINCT ON (run.bundle_id)
        run.bundle_id,
        run.id AS evaluation_run_id,
        run.target_persona_snapshot,
        completion.created_at AS completed_at
    FROM bundle_tag_evaluation_runs AS run
    JOIN LATERAL (
        SELECT event.created_at
        FROM bundle_tag_evaluation_events AS event
        WHERE event.evaluation_run_id = run.id
          AND event.event_type = 'evaluation_completed'
        ORDER BY event.created_at DESC, event.id DESC
        LIMIT 1
    ) AS completion ON TRUE
    ORDER BY run.bundle_id, completion.created_at DESC, run.id DESC
)
SELECT
    bundle.id AS bundle_id,
    bundle.customer_id,
    CASE
        WHEN latest_run.evaluation_run_id IS NULL THEN 'not_evaluated'
        WHEN latest_run.run_status IN ('created', 'evaluating') THEN 'evaluating'
        WHEN latest_successful_run.evaluation_run_id IS NOT NULL
             AND smart_tag_normalize_text(bundle.target_audience_persona) <> smart_tag_normalize_text(latest_successful_run.target_persona_snapshot)
            THEN 'update_required'
        WHEN latest_successful_run.evaluation_run_id IS NOT NULL THEN 'evaluated'
        WHEN latest_run.run_status = 'error' THEN 'error'
        ELSE 'not_evaluated'
    END AS status,
    latest_run.evaluation_run_id AS latest_run_id,
    latest_successful_run.evaluation_run_id AS latest_successful_run_id,
    latest_run.evaluation_created_at AS latest_run_created_at,
    latest_successful_run.completed_at AS latest_completed_at,
    COALESCE(
        latest_run.latest_event_payload ->> 'message',
        latest_run.latest_event_payload ->> 'error_message'
    ) AS latest_error_message,
    CASE
        WHEN latest_run.run_status = 'error' THEN latest_run.latest_event_at
        ELSE NULL
    END AS latest_error_at
FROM bundles AS bundle
LEFT JOIN latest_run
    ON latest_run.bundle_id = bundle.id
LEFT JOIN latest_successful_run
    ON latest_successful_run.bundle_id = bundle.id;

CREATE OR REPLACE FUNCTION reject_immutable_record_change()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION
        'Table % is immutable; % operations are not allowed',
        TG_TABLE_NAME,
        TG_OP;
END;
$$;

DROP TRIGGER IF EXISTS bundle_tag_evaluation_runs_immutable ON bundle_tag_evaluation_runs;
CREATE TRIGGER bundle_tag_evaluation_runs_immutable
BEFORE UPDATE OR DELETE ON bundle_tag_evaluation_runs
FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_change();

DROP TRIGGER IF EXISTS bundle_tag_evaluation_events_immutable ON bundle_tag_evaluation_events;
CREATE TRIGGER bundle_tag_evaluation_events_immutable
BEFORE UPDATE OR DELETE ON bundle_tag_evaluation_events
FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_change();

DROP TRIGGER IF EXISTS bundle_tag_persona_attempts_immutable ON bundle_tag_persona_analysis_attempts;
CREATE TRIGGER bundle_tag_persona_attempts_immutable
BEFORE UPDATE OR DELETE ON bundle_tag_persona_analysis_attempts
FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_change();

DROP TRIGGER IF EXISTS bundle_tag_evaluation_batches_immutable ON bundle_tag_evaluation_batches;
CREATE TRIGGER bundle_tag_evaluation_batches_immutable
BEFORE UPDATE OR DELETE ON bundle_tag_evaluation_batches
FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_change();

DROP TRIGGER IF EXISTS bundle_tag_batch_attempts_immutable ON bundle_tag_evaluation_batch_attempts;
CREATE TRIGGER bundle_tag_batch_attempts_immutable
BEFORE UPDATE OR DELETE ON bundle_tag_evaluation_batch_attempts
FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_change();

DROP TRIGGER IF EXISTS bundle_tag_scores_immutable ON bundle_tag_scores;
CREATE TRIGGER bundle_tag_scores_immutable
BEFORE UPDATE OR DELETE ON bundle_tag_scores
FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_change();

COMMIT;
