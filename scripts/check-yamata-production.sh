#!/usr/bin/env bash

# Read-only validation of the restored/deployed production topology.

set -Eeuo pipefail
set +x # Container environment checks must never emit credential values.

readonly PROJECT_DIR="${1:-/srv/yamata}"

log() {
	printf '[production-check] %s\n' "$*"
}

die() {
	printf '[production-check] ERROR: %s\n' "$*" >&2
	exit 1
}

if docker info >/dev/null 2>&1; then
	DOCKER=(docker)
elif command -v sudo >/dev/null 2>&1 && sudo -n docker info >/dev/null 2>&1; then
	DOCKER=(sudo docker)
else
	die "Docker is unavailable or requires an interactive sudo login"
fi
readonly DOCKER

env_value() {
	local container="$1"
	local key="$2"
	"${DOCKER[@]}" inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$container" |
		awk -F= -v key="$key" '$1 == key { value=substr($0, index($0, "=")+1) } END { print value }'
}

for container in \
	yamata-postgres-beta yamata-redis-beta yamata-app-beta yamata-nginx-beta \
	yamata-campaign-scheduler-beta; do
	"${DOCKER[@]}" container inspect "$container" >/dev/null 2>&1 || die "Missing container: $container"
	state="$("${DOCKER[@]}" inspect -f '{{.State.Status}}' "$container")"
	[[ "$state" == running ]] || die "$container is $state"
	log "$container: running"
done

[[ "$(env_value yamata-app-beta CAMPAIGN_EXECUTION_ENABLED)" == false ]] ||
	die "yamata-app-beta must have CAMPAIGN_EXECUTION_ENABLED=false"
[[ "$(env_value yamata-campaign-scheduler-beta CAMPAIGN_EXECUTION_ENABLED)" == true ]] ||
	die "yamata-campaign-scheduler-beta must have CAMPAIGN_EXECUTION_ENABLED=true"
[[ "$(env_value yamata-campaign-scheduler-beta BOT_API_DOMAIN)" == http://app-beta:8080 ]] ||
	die "Scheduler BOT_API_DOMAIN must be http://app-beta:8080"
[[ "$(env_value yamata-campaign-scheduler-beta SERVER_HOST)" == 127.0.0.1 ]] ||
	die "Scheduler HTTP listener must bind to 127.0.0.1"
[[ "$(env_value yamata-campaign-scheduler-beta SMART_TAG_EVALUATION_ENABLED)" == false ]] ||
	die "Smart-tag evaluation must be disabled in the campaign scheduler"
[[ "$(env_value yamata-campaign-scheduler-beta SMART_TAG_EVALUATION_SCHEDULER_ENABLED)" == false ]] ||
	die "Smart-tag scheduling must be disabled in the campaign scheduler"
[[ -n "$(env_value yamata-app-beta BOT_USERNAME)" ]] || die "BOT_USERNAME is empty"
[[ -n "$(env_value yamata-app-beta BOT_PASSWORD)" ]] || die "BOT_PASSWORD is empty"

app_image_id="$("${DOCKER[@]}" inspect -f '{{.Image}}' yamata-app-beta)"
scheduler_image_id="$("${DOCKER[@]}" inspect -f '{{.Image}}' yamata-campaign-scheduler-beta)"
[[ "$app_image_id" == "$scheduler_image_id" ]] ||
	die "API and scheduler use different image IDs; run deploy-production-beta.sh"
[[ "$("${DOCKER[@]}" inspect -f '{{.HostConfig.RestartPolicy.Name}}' yamata-campaign-scheduler-beta)" == unless-stopped ]] ||
	die "Scheduler restart policy must be unless-stopped"
[[ "$("${DOCKER[@]}" inspect -f '{{len .HostConfig.PortBindings}}' yamata-campaign-scheduler-beta)" == 0 ]] ||
	die "Scheduler must not publish ports"
scheduler_log_volume="$(
	"${DOCKER[@]}" inspect -f \
		'{{range .Mounts}}{{if and (eq .Type "volume") (eq .Destination "/var/log/yamata")}}{{.Name}}{{end}}{{end}}' \
		yamata-campaign-scheduler-beta
)"
[[ -n "$scheduler_log_volume" ]] || die "Scheduler has no persistent log volume"
log "scheduler log volume: $scheduler_log_volume"

app_network="$(
	"${DOCKER[@]}" inspect -f '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' \
		yamata-app-beta | awk 'NF { print; exit }'
)"
scheduler_network="$(
	"${DOCKER[@]}" inspect -f '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' \
		yamata-campaign-scheduler-beta | awk 'NF { print; exit }'
)"
[[ -n "$app_network" && "$app_network" == "$scheduler_network" ]] ||
	die "API and scheduler are not attached to the same Docker network"
log "shared network: $app_network"

"${DOCKER[@]}" exec yamata-campaign-scheduler-beta \
	curl -fsS --noproxy app-beta http://app-beta:8080/api/v1/health >/dev/null ||
	die "Scheduler cannot reach the API over the private network"
"${DOCKER[@]}" exec yamata-nginx-beta nginx -t >/dev/null

app_password_hash="$(
	"${DOCKER[@]}" exec yamata-app-beta sh -c 'printf %s "$BOT_PASSWORD" | sha256sum' | awk '{print $1}'
)"
scheduler_password_hash="$(
	"${DOCKER[@]}" exec yamata-campaign-scheduler-beta sh -c 'printf %s "$BOT_PASSWORD" | sha256sum' | awk '{print $1}'
)"
[[ "$app_password_hash" == "$scheduler_password_hash" ]] ||
	die "BOT_PASSWORD differs between API and scheduler; run deploy-production-beta.sh"
[[ "$(env_value yamata-app-beta BOT_USERNAME)" == \
	"$(env_value yamata-campaign-scheduler-beta BOT_USERNAME)" ]] ||
	die "BOT_USERNAME differs between API and scheduler; run deploy-production-beta.sh"
log "BOT_PASSWORD hashes match (secret not displayed)"

schema_checks="$("${DOCKER[@]}" exec yamata-postgres-beta sh -lc \
	'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "
	 SELECT EXISTS (
	   SELECT 1 FROM information_schema.columns
	   WHERE table_schema='\''public'\''
	     AND table_name='\''processed_campaigns'\''
	     AND column_name='\''bundle_audience_selection_id'\''
	 );
	 SELECT EXISTS (
	   SELECT 1 FROM pg_constraint
	   WHERE conrelid='\''public.processed_campaigns'\''::regclass
	     AND conname='\''processed_campaigns_single_audience_selection'\''
	 );
	 SELECT to_regclass('\''public.campaign_selected_tags'\'') IS NOT NULL;
	 SELECT to_regclass('\''public.campaign_targeting_capacity_calculations'\'') IS NOT NULL;
		 SELECT to_regclass('\''public.src_reference'\'') IS NOT NULL;
		 SELECT to_regclass('\''public.idx_audience_profiles_campaign_id_phone'\'') IS NOT NULL;
		 SELECT to_regclass('\''public.bundle_audience_selection_members'\'') IS NOT NULL;
		 SELECT to_regclass('\''public.uk_processed_campaigns_campaign_id'\'') IS NOT NULL;
		 SELECT to_regclass('\''public.idx_processed_campaigns_capacity_reservation_materialized'\'') IS NULL;"')"
[[ "$schema_checks" == $'t\nt\nt\nt\nt\nt\nt\nt\nt' ]] || die "Required migrations through 0127 are incomplete"

[[ -x "$PROJECT_DIR/scripts/check-yamata-certificates.sh" ]] ||
	die "Missing certificate checker in $PROJECT_DIR/scripts"
"$PROJECT_DIR/scripts/check-yamata-certificates.sh" \
	"$PROJECT_DIR/docker/nginx/sites-available/generated/beta/yamata.conf"

log "Production topology is valid"
