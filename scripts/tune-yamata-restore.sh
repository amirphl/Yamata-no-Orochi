#!/usr/bin/env bash

# Temporarily reduce checkpoint pressure during large PostgreSQL imports.
# Usage: tune-yamata-restore.sh enable|reset

set -Eeuo pipefail

readonly ACTION="${1:-}"
readonly POSTGRES_CONTAINER="yamata-postgres-beta"

die() {
	printf '[restore-tuning] ERROR: %s\n' "$*" >&2
	exit 1
}

case "$ACTION" in
	enable|reset) ;;
	*) die "Usage: $(basename "$0") enable|reset" ;;
esac

if docker info >/dev/null 2>&1; then
	DOCKER=(docker)
elif command -v sudo >/dev/null 2>&1 && sudo -n docker info >/dev/null 2>&1; then
	DOCKER=(sudo docker)
else
	die "Docker is unavailable or requires an interactive sudo login"
fi
readonly DOCKER

[[ "$("${DOCKER[@]}" inspect -f '{{.State.Running}}' "$POSTGRES_CONTAINER" 2>/dev/null || true)" == true ]] ||
	die "PostgreSQL container is not running"

if [[ "$ACTION" == enable ]]; then
	SQL=$(cat <<'SQL'
ALTER SYSTEM SET max_wal_size = '32GB';
ALTER SYSTEM SET min_wal_size = '4GB';
ALTER SYSTEM SET checkpoint_timeout = '30min';
ALTER SYSTEM SET checkpoint_completion_target = '0.9';
ALTER SYSTEM SET wal_compression = 'on';
SELECT pg_reload_conf();
SQL
)
else
	SQL=$(cat <<'SQL'
ALTER SYSTEM RESET max_wal_size;
ALTER SYSTEM RESET min_wal_size;
ALTER SYSTEM RESET checkpoint_timeout;
ALTER SYSTEM RESET checkpoint_completion_target;
ALTER SYSTEM RESET wal_compression;
SELECT pg_reload_conf();
SQL
)
fi
readonly SQL

"${DOCKER[@]}" exec -i "$POSTGRES_CONTAINER" sh -lc \
	'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<<"$SQL"

"${DOCKER[@]}" exec "$POSTGRES_CONTAINER" sh -lc \
	'psql -X -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
	 SHOW max_wal_size;
	 SHOW min_wal_size;
	 SHOW checkpoint_timeout;
	 SHOW checkpoint_completion_target;
	 SHOW wal_compression;"'
