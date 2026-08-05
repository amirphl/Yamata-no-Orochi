#!/usr/bin/env bash

# Apply the idempotent schema changes required by imports and the current app.
# Usage: apply-yamata-required-migrations.sh [PROJECT_DIR]

set -Eeuo pipefail
umask 077

readonly PROJECT_DIR="${1:-/srv/yamata}"
readonly POSTGRES_CONTAINER="yamata-postgres-beta"

log() {
	printf '[required-migrations] %s\n' "$*"
}

die() {
	printf '[required-migrations] ERROR: %s\n' "$*" >&2
	exit 1
}

[[ -d "$PROJECT_DIR/migrations" ]] || die "Missing migrations directory: $PROJECT_DIR/migrations"

if docker info >/dev/null 2>&1; then
	DOCKER=(docker)
elif command -v sudo >/dev/null 2>&1 && sudo -n docker info >/dev/null 2>&1; then
	DOCKER=(sudo docker)
else
	die "Docker is unavailable or requires an interactive sudo login"
fi
readonly DOCKER

[[ "$("${DOCKER[@]}" inspect -f '{{.State.Running}}' "$POSTGRES_CONTAINER" 2>/dev/null || true)" == true ]] ||
	die "PostgreSQL container is not running: $POSTGRES_CONTAINER"

if "${DOCKER[@]}" inspect yamata-campaign-scheduler-beta >/dev/null 2>&1 &&
	[[ "$("${DOCKER[@]}" inspect -f '{{.State.Running}}' yamata-campaign-scheduler-beta)" == true ]]; then
	die "Stop yamata-campaign-scheduler-beta before applying migrations"
fi
if "${DOCKER[@]}" inspect yamata-app-beta >/dev/null 2>&1 &&
	[[ "$("${DOCKER[@]}" inspect -f '{{.State.Running}}' yamata-app-beta)" == true ]]; then
	die "Stop yamata-app-beta before applying migrations"
fi

DB_USER="$("${DOCKER[@]}" exec "$POSTGRES_CONTAINER" printenv POSTGRES_USER)"
DB_NAME="$("${DOCKER[@]}" exec "$POSTGRES_CONTAINER" printenv POSTGRES_DB)"
readonly DB_USER DB_NAME

psql_scalar() {
	"${DOCKER[@]}" exec "$POSTGRES_CONTAINER" psql -X -At \
		-U "$DB_USER" -d "$DB_NAME" -c "$1"
}

apply_file() {
	local migration_file="$1"
	log "Applying $(basename "$migration_file")"
	"${DOCKER[@]}" exec -i "$POSTGRES_CONTAINER" \
		psql -X -v ON_ERROR_STOP=1 -U "$DB_USER" -d "$DB_NAME" <"$migration_file"
}

[[ "$(psql_scalar "SELECT to_regclass('public.audience_profiles') IS NOT NULL;")" == t ]] ||
	die "Missing public.audience_profiles"
[[ "$(psql_scalar "SELECT to_regclass('public.bundle_audience_selections') IS NOT NULL;")" == t ]] ||
	die "Migration 0111 is missing: public.bundle_audience_selections does not exist"
[[ "$(psql_scalar "
	SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema='public'
		  AND table_name='bundle_tag_evaluation_runs'
		  AND column_name='id'
		  AND data_type='bigint'
	);")" == t ]] ||
	die "Migration 0119 must already be applied; it is destructive and is not auto-applied"

apply_file "$PROJECT_DIR/migrations/0114_add_normalized_score_to_audience_profiles.sql"
apply_file "$PROJECT_DIR/migrations/0120_create_campaign_selected_tags.sql"
apply_file "$PROJECT_DIR/migrations/0121_backfill_campaign_audience_targeting_method.sql"
apply_file "$PROJECT_DIR/migrations/0122_create_campaign_targeting_capacity_calculations.sql"
apply_file "$PROJECT_DIR/migrations/0123_add_bundle_audience_selection_to_processed_campaigns.sql"
apply_file "$PROJECT_DIR/migrations/0124_index_smart_targeting_capacity_reservations.sql"
apply_file "$PROJECT_DIR/migrations/0125_create_src_reference.sql"
apply_file "$PROJECT_DIR/migrations/0126_optimize_campaign_audience_selection.sql"
apply_file "$PROJECT_DIR/migrations/0127_normalize_bundle_audience_allocations.sql"

constraint_exists="$(psql_scalar "
	SELECT EXISTS (
		SELECT 1 FROM pg_constraint
		WHERE conrelid='public.processed_campaigns'::regclass
		  AND conname='processed_campaigns_single_audience_selection'
	);")"
[[ "$constraint_exists" == t ]] ||
	die "Migration 0123 is incomplete: processed_campaigns_single_audience_selection is missing"
[[ "$(psql_scalar "SELECT to_regclass('public.campaign_targeting_capacity_calculations') IS NOT NULL;")" == t ]] ||
	die "Migration 0122 is incomplete: campaign_targeting_capacity_calculations is missing"
[[ "$(psql_scalar "SELECT to_regclass('public.campaign_selected_tags') IS NOT NULL;")" == t ]] ||
	die "Migration 0120 is incomplete: campaign_selected_tags is missing"
[[ "$(psql_scalar "SELECT to_regclass('public.src_reference') IS NOT NULL;")" == t ]] ||
	die "Migration 0125 is incomplete: src_reference is missing"
[[ "$(psql_scalar "SELECT to_regclass('public.idx_audience_profiles_campaign_id_phone') IS NOT NULL;")" == t ]] ||
	die "Migration 0126 is incomplete: optimized audience lookup index is missing"
[[ "$(psql_scalar "SELECT to_regclass('public.bundle_audience_selection_members') IS NOT NULL;")" == t ]] ||
	die "Migration 0127 is incomplete: bundle_audience_selection_members is missing"
[[ "$(psql_scalar "SELECT to_regclass('public.uk_processed_campaigns_campaign_id') IS NOT NULL;")" == t ]] ||
	die "Migration 0127 is incomplete: processed campaign uniqueness is missing"
[[ "$(psql_scalar "SELECT to_regclass('public.idx_processed_campaigns_capacity_reservation_materialized') IS NULL;")" == t ]] ||
	die "Migration 0127 is incomplete: obsolete capacity reservation index still exists"

tracker_temporary="$(mktemp "$PROJECT_DIR/.migration_tracker_beta.XXXXXX")"
printf '%s\n' '0127_normalize_bundle_audience_allocations.sql' >"$tracker_temporary"
chmod 600 "$tracker_temporary"
mv -f -- "$tracker_temporary" "$PROJECT_DIR/.migration_tracker_beta"

log "Required schema is ready"
