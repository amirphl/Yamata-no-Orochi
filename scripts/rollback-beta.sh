#!/bin/bash
# ============================================================================
# Blue-Green Rollback Script for Yamata no Orochi – Beta
# ============================================================================
#
# Restores the previous deployment slot without running migrations.
# The old slot's containers must still exist (not yet pruned) or the old
# slot's image tags (yamata-no-orochi:<slot>) must be available.
#
# Usage
# -----
#   ./scripts/rollback-beta.sh <domain> [--drain-seconds N] [--help]
#
# What it does
# ------------
#   1. Reads .deployment-slot to find the CURRENT active slot (e.g. green).
#   2. Determines the previous slot (e.g. blue).
#   3. Starts the previous slot containers (using the slot-tagged image).
#   4. Waits for the previous slot to pass its health-check.
#   5. Regenerates the nginx config to point at the previous slot.
#   6. Gracefully reloads nginx (zero-downtime).
#   7. Waits DRAIN_SECONDS, then stops the current slot.
#   8. Writes the previous slot name to .deployment-slot.
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
ENV_FILE="$PROJECT_ROOT/.env.beta"
SLOT_FILE="$PROJECT_ROOT/.deployment-slot"
COMPOSE_FILE="$PROJECT_ROOT/docker-compose.beta.yml"
NGINX_CONF_DIR="$PROJECT_ROOT/docker/nginx/sites-available"
NGINX_TEMPLATE="$NGINX_CONF_DIR/yamata-beta.conf"

DRAIN_SECONDS="${DRAIN_SECONDS:-30}"
HEALTH_CHECK_ATTEMPTS="${HEALTH_CHECK_ATTEMPTS:-40}"
HEALTH_CHECK_INTERVAL="${HEALTH_CHECK_INTERVAL:-5}"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
print_status()  { echo -e "${BLUE}[INFO]${NC} $1"; }
print_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
print_error()   { echo -e "${RED}[ERROR]${NC} $1"; }
print_step()    { echo -e "\n${BLUE}══ $1 ══${NC}"; }

# ── Helpers shared with deploy-beta.sh ───────────────────────────────────────
get_active_slot() {
    [[ -f "$SLOT_FILE" ]] || { print_error ".deployment-slot file not found – nothing to roll back."; exit 1; }
    local s; s=$(cat "$SLOT_FILE" | tr -d '[:space:]')
    [[ "$s" == "blue" || "$s" == "green" ]] || { print_error "Invalid slot value in .deployment-slot: $s"; exit 1; }
    echo "$s"
}

get_opposite_slot()           { [[ "$1" == "blue" ]] && echo "green" || echo "blue"; }
slot_app_service()            { echo "app-$1"; }
slot_frontend_service()       { echo "frontend-$1"; }
slot_app_container()          { echo "yamata-app-$1"; }
slot_frontend_container()     { echo "frontend-$1"; }

DOCKER_CMD=""
resolve_docker_cmd() {
    if docker info >/dev/null 2>&1; then
        DOCKER_CMD="docker"
    elif command -v sudo >/dev/null 2>&1 && sudo -n docker info >/dev/null 2>&1; then
        DOCKER_CMD="sudo docker"
    else
        DOCKER_CMD="docker"
    fi
}

dc() { $DOCKER_CMD compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" "$@"; }

container_health() {
    $DOCKER_CMD inspect -f \
        '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' \
        "$1" 2>/dev/null || echo "unknown"
}

wait_for_healthy() {
    local container="$1" label="${2:-$1}" attempt=1 status
    print_status "Waiting for $label to become healthy …"
    while [[ $attempt -le $HEALTH_CHECK_ATTEMPTS ]]; do
        status=$(container_health "$container")
        [[ "$status" == "healthy" ]] && { print_success "$label is healthy."; return 0; }
        printf "  attempt %d/%d – status: %s\n" "$attempt" "$HEALTH_CHECK_ATTEMPTS" "$status"
        sleep "$HEALTH_CHECK_INTERVAL"
        (( attempt++ ))
    done
    print_error "$label did not become healthy (last status: $status)."
    return 1
}

export_beta_nginx_template_vars() {
    local domain="$1" app_backend="$2" frontend_backend="$3"
    export DOMAIN="$domain"
    export API_DOMAIN="api.$domain"
    export MONITORING_DOMAIN="monitoring.$domain"
    export SENTRY_UI_DOMAIN="sentry.$domain"
    export HSTS_MAX_AGE="31536000"
    export GLOBAL_RATE_LIMIT="1000"
    export AUTH_RATE_LIMIT="10"
    export APP_BACKEND="$app_backend"
    export FRONTEND_BACKEND="$frontend_backend"
}

generate_nginx_config() {
    local domain="$1" slot="$2"
    local out_dir="$NGINX_CONF_DIR/generated/beta"
    mkdir -p "$out_dir"
    print_status "Generating nginx config for slot: $slot"
    export_beta_nginx_template_vars "$domain" "$(slot_app_service "$slot")" "$(slot_frontend_service "$slot")"
    envsubst '$DOMAIN $API_DOMAIN $MONITORING_DOMAIN $SENTRY_UI_DOMAIN $HSTS_MAX_AGE $GLOBAL_RATE_LIMIT $AUTH_RATE_LIMIT $APP_BACKEND $FRONTEND_BACKEND' \
        < "$NGINX_TEMPLATE" > "$out_dir/yamata.conf"
    print_success "nginx config → $out_dir/yamata.conf"
}

reload_nginx() {
    print_step "Gracefully reloading nginx"
    $DOCKER_CMD exec yamata-nginx-beta nginx -t 2>&1 || {
        print_error "nginx config test FAILED – aborting reload."
        return 1
    }
    $DOCKER_CMD exec yamata-nginx-beta nginx -s reload
    print_success "nginx reloaded – traffic now routes to rollback slot."
}

show_help() {
    cat <<EOF
Usage: $0 <domain> [OPTIONS]

Options:
  --drain-seconds N    Seconds to drain current slot before stopping (default: 30)
  --help | -h          Show this help
EOF
}

# ── Main ──────────────────────────────────────────────────────────────────────
main() {
    echo "🐍 Yamata no Orochi – Blue-Green Rollback"
    echo "=========================================="
    echo ""

    local domain="" custom_drain=""
    while [[ $# -gt 0 ]]; do
        case $1 in
            --drain-seconds) custom_drain="$2"; shift 2 ;;
            --help|-h) show_help; exit 0 ;;
            *)
                if [[ -z "$domain" ]]; then domain="$1"; shift
                else print_error "Unknown option: $1"; show_help; exit 1; fi ;;
        esac
    done

    [[ -z "$domain" ]] && { print_error "Domain is required."; show_help; exit 1; }
    [[ -n "$custom_drain" ]] && DRAIN_SECONDS="$custom_drain"

    resolve_docker_cmd

    if [[ ! -f "$ENV_FILE" ]]; then
        print_error "Missing $ENV_FILE"; exit 1
    fi
    set -a; source "$ENV_FILE"; set +a

    # ── Determine rollback target ────────────────────────────────────────────
    local current_slot; current_slot=$(get_active_slot)
    local rollback_slot; rollback_slot=$(get_opposite_slot "$current_slot")

    print_status "Current slot:  $current_slot"
    print_status "Rollback slot: $rollback_slot"

    # Verify the rollback image exists
    if ! $DOCKER_CMD image inspect "yamata-no-orochi:$rollback_slot" >/dev/null 2>&1; then
        print_error "Image yamata-no-orochi:$rollback_slot not found."
        print_error "Cannot roll back – previous slot image is missing."
        exit 1
    fi

    # ── Generate nginx config for rollback slot ──────────────────────────────
    generate_nginx_config "$domain" "$rollback_slot"

    # ── Start rollback slot ──────────────────────────────────────────────────
    print_step "Starting rollback slot: $rollback_slot"

    # Remove any stale containers
    for ctr in "$(slot_app_container "$rollback_slot")" "$(slot_frontend_container "$rollback_slot")"; do
        $DOCKER_CMD stop "$ctr" 2>/dev/null || true
        $DOCKER_CMD rm -f "$ctr" 2>/dev/null || true
    done

    APP_BLUE_IMAGE="yamata-no-orochi:blue"   \
    APP_GREEN_IMAGE="yamata-no-orochi:green" \
    FRONTEND_BLUE_IMAGE="yamata-frontend-beta:blue"   \
    FRONTEND_GREEN_IMAGE="yamata-frontend-beta:green" \
    dc up -d "$(slot_app_service "$rollback_slot")" "$(slot_frontend_service "$rollback_slot")"

    # ── Health check ─────────────────────────────────────────────────────────
    wait_for_healthy "$(slot_app_container "$rollback_slot")" "app-$rollback_slot" || {
        print_error "Rollback slot health check failed."
        dc stop "$(slot_app_service "$rollback_slot")" "$(slot_frontend_service "$rollback_slot")" 2>/dev/null || true
        exit 1
    }

    # ── Switch nginx ─────────────────────────────────────────────────────────
    reload_nginx || {
        print_error "nginx reload failed during rollback."
        dc stop "$(slot_app_service "$rollback_slot")" "$(slot_frontend_service "$rollback_slot")" 2>/dev/null || true
        exit 1
    }

    # ── Persist state ────────────────────────────────────────────────────────
    echo "$rollback_slot" > "$SLOT_FILE"
    print_success "Active slot recorded: $rollback_slot"

    # ── Drain & stop current slot ────────────────────────────────────────────
    print_status "Draining current slot ($current_slot) for ${DRAIN_SECONDS}s …"
    sleep "$DRAIN_SECONDS"
    dc stop "$(slot_app_service "$current_slot")" "$(slot_frontend_service "$current_slot")" || true
    print_success "Current slot $current_slot stopped."

    print_success "🎉 Rollback complete! Active slot: $rollback_slot"
    echo ""
    echo "  To deploy again:  ./scripts/deploy-beta.sh $domain"
    echo ""
}

main "$@"
