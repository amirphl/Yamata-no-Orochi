#!/usr/bin/env bash

# Run campaign execution in an isolated backend container.
# Usage: ./scripts/deploy-campaign-scheduler-beta.sh [IMAGE]

set -Eeuo pipefail
umask 077

readonly SOURCE_CONTAINER="${YAMATA_SOURCE_CONTAINER:-yamata-app-beta}"
readonly CONTAINER_NAME="${YAMATA_SCHEDULER_CONTAINER:-yamata-campaign-scheduler-beta}"
readonly LOG_VOLUME="${YAMATA_SCHEDULER_LOG_VOLUME:-yamata-campaign-scheduler-logs-beta}"

log() {
	printf '[campaign-scheduler] %s\n' "$*"
}

die() {
	printf '[campaign-scheduler] ERROR: %s\n' "$*" >&2
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

"${DOCKER[@]}" container inspect "$SOURCE_CONTAINER" >/dev/null 2>&1 ||
	die "Source backend container does not exist: $SOURCE_CONTAINER"

[[ "$("${DOCKER[@]}" inspect -f '{{.State.Running}}' "$SOURCE_CONTAINER")" == "true" ]] ||
	die "Source backend is not running: $SOURCE_CONTAINER"

MAIN_CAMPAIGN_SETTING="$(
	"${DOCKER[@]}" inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$SOURCE_CONTAINER" |
		awk -F= '$1 == "CAMPAIGN_EXECUTION_ENABLED" { value=substr($0, index($0, "=")+1) } END { print value }'
)"
[[ "${MAIN_CAMPAIGN_SETTING,,}" == "false" ]] ||
	die "$SOURCE_CONTAINER must have CAMPAIGN_EXECUTION_ENABLED=false (found: ${MAIN_CAMPAIGN_SETTING:-unset})"

NETWORK="$(
	"${DOCKER[@]}" inspect -f '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' \
		"$SOURCE_CONTAINER" | awk 'NF { print; exit }'
)"
[[ -n "$NETWORK" ]] || die "Could not determine the Docker network used by $SOURCE_CONTAINER"

NETWORK_MEMBERS="$("${DOCKER[@]}" network inspect -f '{{range .Containers}}{{println .Name}}{{end}}' "$NETWORK")"
grep -Fxq 'yamata-postgres-beta' <<<"$NETWORK_MEMBERS" ||
	die "yamata-postgres-beta is not attached to $NETWORK"
grep -Fxq 'yamata-redis-beta' <<<"$NETWORK_MEMBERS" ||
	die "yamata-redis-beta is not attached to $NETWORK"

for dependency in yamata-postgres-beta yamata-redis-beta; do
	[[ "$("${DOCKER[@]}" inspect -f '{{.State.Running}}' "$dependency")" == "true" ]] ||
		die "Dependency is not running: $dependency"
done

IMAGE="${1:-$("${DOCKER[@]}" inspect -f '{{.Config.Image}}' "$SOURCE_CONTAINER")}"
readonly IMAGE
"${DOCKER[@]}" image inspect "$IMAGE" >/dev/null 2>&1 || die "Image does not exist locally: $IMAGE"

RUNTIME_ENV="$(mktemp)"
cleanup() {
	rm -f -- "$RUNTIME_ENV"
}
trap cleanup EXIT

# Copy the backend's exact effective environment without exposing its secrets.
"${DOCKER[@]}" inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$SOURCE_CONTAINER" |
	awk -F= '
		BEGIN {
			override["CAMPAIGN_EXECUTION_ENABLED"] = 1
			override["SMART_TAG_EVALUATION_ENABLED"] = 1
			override["SMART_TAG_EVALUATION_SCHEDULER_ENABLED"] = 1
			override["SERVER_HOST"] = 1
			override["SERVER_PORT"] = 1
			override["SERVER_ENABLE_PPROF"] = 1
			override["SERVER_ENABLE_METRICS"] = 1
			override["METRICS_ENABLED"] = 1
			override["METRICS_ENABLE_PROMETHEUS"] = 1
			override["LOG_OUTPUT"] = 1
			override["LOG_FILE_PATH"] = 1
			override["LOG_ENABLE_ACCESS"] = 1
			override["LOG_ACCESS_PATH"] = 1
			override["LOG_AUDIT_PATH"] = 1
			override["LOG_SECURITY_PATH"] = 1
			override["SENTRY_SERVER_NAME"] = 1
		}
		!($1 in override) { print }
	' >"$RUNTIME_ENV"

cat >>"$RUNTIME_ENV" <<'EOF'
CAMPAIGN_EXECUTION_ENABLED=true
SMART_TAG_EVALUATION_ENABLED=false
SMART_TAG_EVALUATION_SCHEDULER_ENABLED=false
SERVER_HOST=127.0.0.1
SERVER_PORT=8080
SERVER_ENABLE_PPROF=false
SERVER_ENABLE_METRICS=false
METRICS_ENABLED=false
METRICS_ENABLE_PROMETHEUS=false
LOG_OUTPUT=both
LOG_FILE_PATH=/var/log/yamata/campaign-scheduler.log
LOG_ENABLE_ACCESS=false
LOG_ACCESS_PATH=/var/log/yamata/campaign-scheduler-access.log
LOG_AUDIT_PATH=/var/log/yamata/campaign-scheduler-audit.log
LOG_SECURITY_PATH=/var/log/yamata/campaign-scheduler-security.log
SENTRY_SERVER_NAME=yamata-campaign-scheduler-beta
EOF

"${DOCKER[@]}" volume create "$LOG_VOLUME" >/dev/null

if "${DOCKER[@]}" container inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
	log "Stopping the previous scheduler container gracefully"
	"${DOCKER[@]}" stop --time 60 "$CONTAINER_NAME" >/dev/null || true
	"${DOCKER[@]}" rm "$CONTAINER_NAME" >/dev/null
fi

log "Starting $CONTAINER_NAME from $IMAGE on $NETWORK"
"${DOCKER[@]}" run -d \
	--name "$CONTAINER_NAME" \
	--hostname "$CONTAINER_NAME" \
	--restart unless-stopped \
	--stop-timeout 60 \
	--network "$NETWORK" \
	--env-file "$RUNTIME_ENV" \
	--user appuser \
	--read-only \
	--tmpfs /tmp:rw,nosuid,noexec,size=256m \
	--tmpfs /var/cache:rw,nosuid,noexec,size=64m \
	--mount "type=volume,source=$LOG_VOLUME,target=/var/log/yamata" \
	--mount "type=volume,source=$LOG_VOLUME,target=/data" \
	--security-opt no-new-privileges=true \
	--cap-drop ALL \
	--log-driver local \
	--log-opt max-size=20m \
	--log-opt max-file=5 \
	--label com.yamata.role=campaign-scheduler \
	"$IMAGE" >/dev/null

log "Waiting for the isolated instance to become healthy"
deadline=$((SECONDS + 90))
while ((SECONDS < deadline)); do
	state="$("${DOCKER[@]}" inspect -f '{{.State.Status}}' "$CONTAINER_NAME")"
	health="$("${DOCKER[@]}" inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$CONTAINER_NAME")"
	if [[ "$state" == "running" && "$health" == "healthy" ]]; then
		log "Ready. Campaign execution is enabled only in $CONTAINER_NAME"
		log "Logs: ${DOCKER[*]} logs -f $CONTAINER_NAME"
		log "Persistent log volume: $LOG_VOLUME"
		exit 0
	fi
	if [[ "$state" == "dead" || "$state" == "exited" ]]; then
		break
	fi
	sleep 2
done

"${DOCKER[@]}" logs --tail 100 "$CONTAINER_NAME" >&2 || true
die "$CONTAINER_NAME did not become healthy; restart policy remains active"
