-- Description: Drop smart tag evaluation storage and tag metadata columns

BEGIN;

DROP VIEW IF EXISTS current_bundle_tag_evaluation_status;
DROP VIEW IF EXISTS current_bundle_tag_scores;
DROP VIEW IF EXISTS bundle_tag_evaluation_run_status;

DROP TRIGGER IF EXISTS bundle_tag_scores_immutable ON bundle_tag_scores;
DROP TRIGGER IF EXISTS bundle_tag_batch_attempts_immutable ON bundle_tag_evaluation_batch_attempts;
DROP TRIGGER IF EXISTS bundle_tag_evaluation_batches_immutable ON bundle_tag_evaluation_batches;
DROP TRIGGER IF EXISTS bundle_tag_persona_attempts_immutable ON bundle_tag_persona_analysis_attempts;
DROP TRIGGER IF EXISTS bundle_tag_evaluation_events_immutable ON bundle_tag_evaluation_events;
DROP TRIGGER IF EXISTS bundle_tag_evaluation_runs_immutable ON bundle_tag_evaluation_runs;

DROP FUNCTION IF EXISTS reject_immutable_record_change();
DROP FUNCTION IF EXISTS smart_tag_normalize_text(TEXT);

DROP TABLE IF EXISTS bundle_tag_scores;
DROP TABLE IF EXISTS bundle_tag_evaluation_batch_attempts;
DROP TABLE IF EXISTS bundle_tag_evaluation_batches;
DROP TABLE IF EXISTS bundle_tag_persona_analysis_attempts;
DROP TABLE IF EXISTS bundle_tag_evaluation_events;
DROP TABLE IF EXISTS bundle_tag_evaluation_runs;

ALTER TABLE tags
    DROP COLUMN IF EXISTS audience_count,
    DROP COLUMN IF EXISTS audience_persona,
    DROP COLUMN IF EXISTS display_title;

COMMIT;
