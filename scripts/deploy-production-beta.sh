#!/usr/bin/env bash

# Canonical production deployment: deploy the API, then recreate the isolated scheduler.
# Usage: ./scripts/deploy-production-beta.sh --domain jazebeh.ir

set -Eeuo pipefail
set +x # Deployment environment values may contain credentials.
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
readonly SCRIPT_DIR PROJECT_ROOT

for argument in "$@"; do
	case "$argument" in
		--help|-h) exec "$SCRIPT_DIR/deploy-beta.sh" --help ;;
	esac
done

[[ -f "$PROJECT_ROOT/.env.beta" ]] || {
	printf '[deploy-production] ERROR: Missing %s/.env.beta\n' "$PROJECT_ROOT" >&2
	exit 1
}
[[ ! -L "$PROJECT_ROOT/.env.beta" ]] || {
	printf '[deploy-production] ERROR: Refusing symlinked .env.beta\n' >&2
	exit 1
}
chmod 600 "$PROJECT_ROOT/.env.beta"

# shellcheck disable=SC1091
source "$SCRIPT_DIR/load-yamata-env.sh"
load_yamata_env_file "$PROJECT_ROOT/.env.beta"

[[ "${CAMPAIGN_EXECUTION_ENABLED:-}" == false ]] || {
	printf '[deploy-production] ERROR: CAMPAIGN_EXECUTION_ENABLED must be false in .env.beta\n' >&2
	exit 1
}

cd "$PROJECT_ROOT"
"$SCRIPT_DIR/deploy-beta.sh" "$@"
"$SCRIPT_DIR/deploy-campaign-scheduler-beta.sh" yamata-no-orochi
"$SCRIPT_DIR/check-yamata-production.sh" "$PROJECT_ROOT"

printf '[deploy-production] API and isolated campaign scheduler deployed successfully\n'
