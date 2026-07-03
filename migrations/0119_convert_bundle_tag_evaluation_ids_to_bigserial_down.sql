-- Restore smart-tag identifiers to UUID. This rollback also discards evaluation data.

BEGIN;

DROP VIEW IF EXISTS current_bundle_tag_evaluation_status;
DROP VIEW IF EXISTS current_bundle_tag_scores;
DROP VIEW IF EXISTS bundle_tag_evaluation_run_status;

TRUNCATE TABLE
    bundle_tag_scores,
    bundle_tag_evaluation_batch_attempts,
    bundle_tag_evaluation_batches,
    bundle_tag_persona_analysis_attempts,
    bundle_tag_evaluation_events,
    bundle_tag_evaluation_runs
RESTART IDENTITY;

ALTER TABLE bundle_tag_evaluation_events
    DROP CONSTRAINT IF EXISTS bundle_tag_evaluation_events_evaluation_run_id_fkey,
    DROP CONSTRAINT IF EXISTS bundle_tag_evaluation_events_batch_fk;

ALTER TABLE bundle_tag_persona_analysis_attempts
    DROP CONSTRAINT IF EXISTS bundle_tag_persona_analysis_attempts_evaluation_run_id_fkey,
    DROP CONSTRAINT IF EXISTS bundle_tag_persona_attempt_unique;

ALTER TABLE bundle_tag_evaluation_batches
    DROP CONSTRAINT IF EXISTS bundle_tag_evaluation_batches_evaluation_run_id_fkey,
    DROP CONSTRAINT IF EXISTS bundle_tag_evaluation_batch_unique;

ALTER TABLE bundle_tag_evaluation_batch_attempts
    DROP CONSTRAINT IF EXISTS bundle_tag_evaluation_batch_attempts_batch_id_fkey,
    DROP CONSTRAINT IF EXISTS bundle_tag_batch_attempt_unique;

ALTER TABLE bundle_tag_scores
    DROP CONSTRAINT IF EXISTS bundle_tag_scores_evaluation_run_id_fkey,
    DROP CONSTRAINT IF EXISTS bundle_tag_scores_batch_id_fkey,
    DROP CONSTRAINT IF EXISTS bundle_tag_scores_batch_attempt_id_fkey,
    DROP CONSTRAINT IF EXISTS bundle_tag_scores_run_tag_unique;

ALTER TABLE bundle_tag_evaluation_runs
    DROP CONSTRAINT IF EXISTS bundle_tag_evaluation_runs_pkey;

ALTER TABLE bundle_tag_persona_analysis_attempts
    DROP CONSTRAINT IF EXISTS bundle_tag_persona_analysis_attempts_pkey;

ALTER TABLE bundle_tag_evaluation_batches
    DROP CONSTRAINT IF EXISTS bundle_tag_evaluation_batches_pkey;

ALTER TABLE bundle_tag_evaluation_batch_attempts
    DROP CONSTRAINT IF EXISTS bundle_tag_evaluation_batch_attempts_pkey;

DROP INDEX IF EXISTS idx_bundle_tag_eval_runs_bundle_created;
DROP INDEX IF EXISTS idx_bundle_tag_eval_runs_customer_created;
DROP INDEX IF EXISTS idx_bundle_tag_eval_events_run_created;
DROP INDEX IF EXISTS idx_bundle_tag_persona_attempts_run_attempt;
DROP INDEX IF EXISTS idx_bundle_tag_eval_batches_run_batch;
DROP INDEX IF EXISTS idx_bundle_tag_eval_batch_attempts_batch_attempt;
DROP INDEX IF EXISTS idx_bundle_tag_scores_run_tag;
DROP INDEX IF EXISTS idx_bundle_tag_scores_batch_tag;

ALTER TABLE bundle_tag_evaluation_events
    DROP COLUMN evaluation_run_id,
    DROP COLUMN batch_id;

ALTER TABLE bundle_tag_persona_analysis_attempts
    DROP COLUMN id,
    DROP COLUMN evaluation_run_id;

ALTER TABLE bundle_tag_evaluation_batches
    DROP COLUMN id,
    DROP COLUMN evaluation_run_id;

ALTER TABLE bundle_tag_evaluation_batch_attempts
    DROP COLUMN id,
    DROP COLUMN batch_id;

ALTER TABLE bundle_tag_scores
    DROP COLUMN evaluation_run_id,
    DROP COLUMN batch_id,
    DROP COLUMN batch_attempt_id;

ALTER TABLE bundle_tag_evaluation_runs
    DROP COLUMN id;

ALTER TABLE bundle_tag_evaluation_runs
    ADD COLUMN id UUID PRIMARY KEY DEFAULT gen_random_uuid();

ALTER TABLE bundle_tag_persona_analysis_attempts
    ADD COLUMN id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ADD COLUMN evaluation_run_id UUID NOT NULL;

ALTER TABLE bundle_tag_evaluation_batches
    ADD COLUMN id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ADD COLUMN evaluation_run_id UUID NOT NULL;

ALTER TABLE bundle_tag_evaluation_batch_attempts
    ADD COLUMN id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ADD COLUMN batch_id UUID NOT NULL;

ALTER TABLE bundle_tag_evaluation_events
    ADD COLUMN evaluation_run_id UUID NOT NULL,
    ADD COLUMN batch_id UUID;

ALTER TABLE bundle_tag_scores
    ADD COLUMN evaluation_run_id UUID NOT NULL,
    ADD COLUMN batch_id UUID NOT NULL,
    ADD COLUMN batch_attempt_id UUID NOT NULL;

ALTER TABLE bundle_tag_evaluation_events
    ADD CONSTRAINT bundle_tag_evaluation_events_evaluation_run_id_fkey
        FOREIGN KEY (evaluation_run_id) REFERENCES bundle_tag_evaluation_runs(id),
    ADD CONSTRAINT bundle_tag_evaluation_events_batch_fk
        FOREIGN KEY (batch_id) REFERENCES bundle_tag_evaluation_batches(id);

ALTER TABLE bundle_tag_persona_analysis_attempts
    ADD CONSTRAINT bundle_tag_persona_analysis_attempts_evaluation_run_id_fkey
        FOREIGN KEY (evaluation_run_id) REFERENCES bundle_tag_evaluation_runs(id),
    ADD CONSTRAINT bundle_tag_persona_attempt_unique
        UNIQUE (evaluation_run_id, attempt_number);

ALTER TABLE bundle_tag_evaluation_batches
    ADD CONSTRAINT bundle_tag_evaluation_batches_evaluation_run_id_fkey
        FOREIGN KEY (evaluation_run_id) REFERENCES bundle_tag_evaluation_runs(id),
    ADD CONSTRAINT bundle_tag_evaluation_batch_unique
        UNIQUE (evaluation_run_id, batch_number);

ALTER TABLE bundle_tag_evaluation_batch_attempts
    ADD CONSTRAINT bundle_tag_evaluation_batch_attempts_batch_id_fkey
        FOREIGN KEY (batch_id) REFERENCES bundle_tag_evaluation_batches(id),
    ADD CONSTRAINT bundle_tag_batch_attempt_unique
        UNIQUE (batch_id, attempt_number);

ALTER TABLE bundle_tag_scores
    ADD CONSTRAINT bundle_tag_scores_evaluation_run_id_fkey
        FOREIGN KEY (evaluation_run_id) REFERENCES bundle_tag_evaluation_runs(id),
    ADD CONSTRAINT bundle_tag_scores_batch_id_fkey
        FOREIGN KEY (batch_id) REFERENCES bundle_tag_evaluation_batches(id),
    ADD CONSTRAINT bundle_tag_scores_batch_attempt_id_fkey
        FOREIGN KEY (batch_attempt_id) REFERENCES bundle_tag_evaluation_batch_attempts(id),
    ADD CONSTRAINT bundle_tag_scores_run_tag_unique
        UNIQUE (evaluation_run_id, tag_id);

CREATE INDEX idx_bundle_tag_eval_runs_bundle_created
    ON bundle_tag_evaluation_runs (bundle_id, created_at DESC, id DESC);

CREATE INDEX idx_bundle_tag_eval_runs_customer_created
    ON bundle_tag_evaluation_runs (customer_id, created_at DESC, id DESC);

CREATE INDEX idx_bundle_tag_eval_events_run_created
    ON bundle_tag_evaluation_events (evaluation_run_id, created_at DESC, id DESC);

CREATE INDEX idx_bundle_tag_persona_attempts_run_attempt
    ON bundle_tag_persona_analysis_attempts (evaluation_run_id, attempt_number DESC);

CREATE INDEX idx_bundle_tag_eval_batches_run_batch
    ON bundle_tag_evaluation_batches (evaluation_run_id, batch_number);

CREATE INDEX idx_bundle_tag_eval_batch_attempts_batch_attempt
    ON bundle_tag_evaluation_batch_attempts (batch_id, attempt_number DESC);

CREATE INDEX idx_bundle_tag_scores_run_tag
    ON bundle_tag_scores (evaluation_run_id, tag_id);

CREATE INDEX idx_bundle_tag_scores_batch_tag
    ON bundle_tag_scores (batch_id, tag_id);

CREATE VIEW bundle_tag_evaluation_run_status AS
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

CREATE VIEW current_bundle_tag_scores AS
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

CREATE VIEW current_bundle_tag_evaluation_status AS
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

COMMIT;
