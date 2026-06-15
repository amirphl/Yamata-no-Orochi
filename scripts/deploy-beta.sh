#!/bin/bash
# ============================================================================
# Blue-Green Deployment Script for Yamata no Orochi – Beta
# ============================================================================
#
# Strategy
# --------
# Two identical application slots ("blue" and "green") coexist in the Docker
# network.  Only one is live at any time.  A deployment:
#   1. Builds / tags fresh images for the *new* slot.
#   2. Starts the new slot containers.
#   3. Waits for the new slot to pass its health-check.
#   4. Applies database migrations (backward-compatible / additive only).
#   5. Regenerates the nginx site config to point at the new slot.
#   6. Reloads nginx *gracefully* (nginx -s reload) – zero dropped connections.
#   7. Waits DRAIN_SECONDS for in-flight requests to finish on the old slot.
#   8. Stops the old slot containers.
#   9. Writes the new slot name to .deployment-slot.
#
# First deployment (no .deployment-slot file): starts everything from scratch
# and lands on the blue slot.
#
# Slot map
# --------
#   blue  → app-blue  (172.30.0.20) + frontend-blue  (172.30.0.60)
#   green → app-green (172.30.0.21) + frontend-green (172.30.0.62)
#
# Usage
# -----
#   ./scripts/deploy-beta.sh <domain> [OPTIONS]
#
# Options
#   --domain DOMAIN          Override domain (default: thewritingonthewall.com)
#   --email  EMAIL           Let's Encrypt contact email
#   --build                  Build backend Docker image before deploying
#   --build-frontend         Build frontend Docker image before deploying
#   --skip-migrations        Skip database migrations (dangerous – use with care)
#   --drain-seconds N        Seconds to wait for connection drain (default: 30)
#   --force-slot blue|green  Deploy to a specific slot regardless of current state
#   --help | -h              Show this help
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
ENV_FILE="$PROJECT_ROOT/.env.beta"
SLOT_FILE="$PROJECT_ROOT/.deployment-slot"
COMPOSE_FILE="$PROJECT_ROOT/docker-compose.beta.yml"
NGINX_CONF_DIR="$PROJECT_ROOT/docker/nginx/sites-available"
NGINX_TEMPLATE="$NGINX_CONF_DIR/yamata-beta.conf"
LETSENCRYPT_DIR="/etc/letsencrypt"
ACME_SH_DIR="$HOME/.acme.sh"

# Tunables (can be overridden via environment variables)
DRAIN_SECONDS="${DRAIN_SECONDS:-30}"
HEALTH_CHECK_ATTEMPTS="${HEALTH_CHECK_ATTEMPTS:-40}"
HEALTH_CHECK_INTERVAL="${HEALTH_CHECK_INTERVAL:-5}"

# ── Colours ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
print_status()  { echo -e "${BLUE}[INFO]${NC} $1"; }
print_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
print_error()   { echo -e "${RED}[ERROR]${NC} $1"; }
print_step()    { echo -e "\n${BLUE}══ $1 ══${NC}"; }

# ── Slot helpers ─────────────────────────────────────────────────────────────
get_active_slot() {
    if [[ -f "$SLOT_FILE" ]]; then
        local s; s=$(cat "$SLOT_FILE" | tr -d '[:space:]')
        if [[ "$s" == "blue" || "$s" == "green" ]]; then echo "$s"; return; fi
    fi
    echo ""   # empty = no active slot (first deploy)
}

get_opposite_slot() {
    [[ "$1" == "blue" ]] && echo "green" || echo "blue"
}

slot_app_container()      { echo "yamata-app-$1"; }
slot_frontend_container() { echo "frontend-$1"; }
slot_app_service()        { echo "app-$1"; }
slot_frontend_service()   { echo "frontend-$1"; }

# ── Docker helpers ───────────────────────────────────────────────────────────
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

container_exists()  { $DOCKER_CMD ps -a --format '{{.Names}}' | grep -qx "$1"; }
container_running() { $DOCKER_CMD ps   --format '{{.Names}}' | grep -qx "$1"; }

container_health() {
    $DOCKER_CMD inspect -f \
        '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' \
        "$1" 2>/dev/null || echo "unknown"
}

# ── Wait for a container to become healthy ───────────────────────────────────
wait_for_healthy() {
    local container="$1"
    local label="${2:-$container}"
    local attempt=1
    local status

    print_status "Waiting for $label to become healthy …"
    while [[ $attempt -le $HEALTH_CHECK_ATTEMPTS ]]; do
        status=$(container_health "$container")
        if [[ "$status" == "healthy" ]]; then
            print_success "$label is healthy."
            return 0
        fi
        printf "  attempt %d/%d – status: %s\n" "$attempt" "$HEALTH_CHECK_ATTEMPTS" "$status"
        sleep "$HEALTH_CHECK_INTERVAL"
        (( attempt++ ))
    done

    print_error "$label did NOT become healthy within $(( HEALTH_CHECK_ATTEMPTS * HEALTH_CHECK_INTERVAL )) seconds (last status: $status)."
    return 1
}

# ── Prerequisites ─────────────────────────────────────────────────────────────
command_exists() { command -v "$1" >/dev/null 2>&1; }

check_prerequisites() {
    print_step "Checking prerequisites"
    local ok=true

    command_exists docker    || { print_error "docker not found"; ok=false; }
    docker compose version >/dev/null 2>&1 || { print_error "Docker Compose V2 not found"; ok=false; }
    command_exists openssl   || { print_error "openssl not found"; ok=false; }
    command_exists envsubst  || { print_error "envsubst not found (install gettext)"; ok=false; }
    docker info >/dev/null 2>&1 || { print_error "Docker daemon not running"; ok=false; }

    [[ "$ok" == "true" ]] || exit 1
    print_success "Prerequisites satisfied."
}

# ── Proxy helpers ─────────────────────────────────────────────────────────────
get_proxy_build_args() {
    local args=""
    [[ -n "${HTTP_PROXY:-}"  ]] && args+=" --build-arg HTTP_PROXY=$HTTP_PROXY"
    [[ -n "${http_proxy:-}"  ]] && [[ -z "${HTTP_PROXY:-}" ]] && args+=" --build-arg HTTP_PROXY=$http_proxy"
    [[ -n "${HTTPS_PROXY:-}" ]] && args+=" --build-arg HTTPS_PROXY=$HTTPS_PROXY"
    [[ -n "${https_proxy:-}" ]] && [[ -z "${HTTPS_PROXY:-}" ]] && args+=" --build-arg HTTPS_PROXY=$https_proxy"
    [[ -n "${NO_PROXY:-}"    ]] && args+=" --build-arg NO_PROXY=$NO_PROXY"
    echo "$args"
}

# ── Image build & tagging ─────────────────────────────────────────────────────
build_backend_image() {
    local slot="$1"
    print_step "Building backend image for slot: $slot"
    local proxy_args; proxy_args=$(get_proxy_build_args)
    # shellcheck disable=SC2086
    $DOCKER_CMD build $proxy_args \
        -f "$PROJECT_ROOT/docker/Dockerfile.production" \
        -t "yamata-no-orochi:$slot" \
        "$PROJECT_ROOT"
    print_success "Backend image built → yamata-no-orochi:$slot"
}

build_frontend_image() {
    local slot="$1"
    print_step "Building frontend image for slot: $slot"
    local proxy_args; proxy_args=$(get_proxy_build_args)
    if [[ ! -f "$PROJECT_ROOT/Dockerfile.frontend" ]]; then
        print_warning "Dockerfile.frontend not found – skipping frontend build."
        print_warning "Make sure yamata-frontend-beta:$slot image is already present."
        return 0
    fi
    # shellcheck disable=SC2086
    $DOCKER_CMD build $proxy_args \
        -f "$PROJECT_ROOT/Dockerfile.frontend" \
        -t "yamata-frontend-beta:$slot" \
        "$PROJECT_ROOT"
    print_success "Frontend image built → yamata-frontend-beta:$slot"
}

# Tag an existing image (latest) for the target slot so rollback works.
tag_images_for_slot() {
    local slot="$1"
    print_step "Tagging images for slot: $slot"

    # Backend
    if $DOCKER_CMD image inspect "yamata-no-orochi:latest" >/dev/null 2>&1; then
        $DOCKER_CMD tag "yamata-no-orochi:latest" "yamata-no-orochi:$slot"
        print_success "yamata-no-orochi:latest → yamata-no-orochi:$slot"
    elif $DOCKER_CMD image inspect "yamata-no-orochi" >/dev/null 2>&1; then
        $DOCKER_CMD tag "yamata-no-orochi" "yamata-no-orochi:$slot"
        print_success "yamata-no-orochi → yamata-no-orochi:$slot"
    else
        print_error "Backend image 'yamata-no-orochi' not found. Build it first (pass --build)."
        exit 1
    fi

    # Frontend
    if $DOCKER_CMD image inspect "yamata-frontend-beta:latest" >/dev/null 2>&1; then
        $DOCKER_CMD tag "yamata-frontend-beta:latest" "yamata-frontend-beta:$slot"
        print_success "yamata-frontend-beta:latest → yamata-frontend-beta:$slot"
    elif $DOCKER_CMD image inspect "yamata-frontend-beta" >/dev/null 2>&1; then
        $DOCKER_CMD tag "yamata-frontend-beta" "yamata-frontend-beta:$slot"
        print_success "yamata-frontend-beta → yamata-frontend-beta:$slot"
    else
        print_warning "Frontend image 'yamata-frontend-beta' not found; will use whatever tag is already present."
    fi
}

# ── SSL certificates ──────────────────────────────────────────────────────────
check_acme_sh() {
    if ! command_exists "$ACME_SH_DIR/acme.sh"; then
        print_error "acme.sh not installed. Run: curl https://get.acme.sh | sh"
        exit 1
    fi
}

check_certificate_validity() {
    local domain="$1"
    local cert_file="$LETSENCRYPT_DIR/live/$domain/fullchain.pem"
    [[ -f "$cert_file" ]] || return 1
    local days; days=$(( ( $(date -d "$(openssl x509 -enddate -noout -in "$cert_file" | cut -d= -f2)" +%s) - $(date +%s) ) / 86400 ))
    [[ $days -gt 30 ]] && { print_success "Certificate for $domain valid for $days days"; return 0; }
    print_warning "Certificate for $domain expires in $days days"
    return 1
}

obtain_letsencrypt_certificates() {
    local domain="$1"
    check_acme_sh
    print_step "Checking/obtaining Let's Encrypt certificates for $domain"
    sudo mkdir -p "$LETSENCRYPT_DIR/live/$domain" "$LETSENCRYPT_DIR/archive/$domain"
    check_certificate_validity "$domain" && return 0

    systemctl is-active --quiet nginx && sudo systemctl stop nginx

    "$ACME_SH_DIR/acme.sh" --issue \
        -d "$domain" -d "www.$domain" -d "api.$domain" \
        -d "monitoring.$domain" -d "sentry.$domain" \
        --webroot /var/www/html --server letsencrypt || {
        print_error "Failed to obtain certificate for $domain"
        return 1
    }

    "$ACME_SH_DIR/acme.sh" --install-cert -d "$domain" \
        --cert-file      "$LETSENCRYPT_DIR/live/$domain/cert.pem" \
        --key-file       "$LETSENCRYPT_DIR/live/$domain/privkey.pem" \
        --fullchain-file "$LETSENCRYPT_DIR/live/$domain/fullchain.pem" \
        --chain-file     "$LETSENCRYPT_DIR/live/$domain/chain.pem"

    sudo chmod 644 "$LETSENCRYPT_DIR/live/$domain/cert.pem"
    sudo chmod 600 "$LETSENCRYPT_DIR/live/$domain/privkey.pem"
    sudo chmod 644 "$LETSENCRYPT_DIR/live/$domain/fullchain.pem"
    sudo chmod 644 "$LETSENCRYPT_DIR/live/$domain/chain.pem"
    sudo chown -R root:root "$LETSENCRYPT_DIR/live/$domain"
    print_success "Certificates obtained for $domain"
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

validate_nginx_certificates() {
    local domain="$1"
    print_step "Validating SSL certificates referenced in yamata-beta.conf"
    local tmp; tmp=$(mktemp)
    export_beta_nginx_template_vars "$domain" "app-blue" "frontend-blue"
    envsubst '$DOMAIN $API_DOMAIN $MONITORING_DOMAIN $SENTRY_UI_DOMAIN $HSTS_MAX_AGE $GLOBAL_RATE_LIMIT $AUTH_RATE_LIMIT $APP_BACKEND $FRONTEND_BACKEND' \
        < "$NGINX_TEMPLATE" > "$tmp"
    local failed=false
    while IFS= read -r cert_path; do
        cert_path="${cert_path%;}"
        [[ -z "$cert_path" ]] && continue
        if [[ ! -f "$cert_path" ]]; then
            print_error "Missing certificate file: $cert_path"; failed=true; continue
        fi
        if [[ "$cert_path" == *privkey* ]]; then
            print_success "Key file found: $cert_path"; continue
        fi
        if openssl x509 -checkend $(( 7*24*3600 )) -noout -in "$cert_path" >/dev/null 2>&1; then
            print_success "Certificate OK: $cert_path"
        else
            print_error "Certificate expired or expiring within 7 days: $cert_path"
            failed=true
        fi
    done < <(grep -E '^\s*ssl_(certificate|certificate_key|trusted_certificate)' "$tmp" | awk '{print $2}')
    rm -f "$tmp"
    [[ "$failed" == "false" ]] || { print_error "Certificate validation failed."; exit 1; }
    print_success "All certificates valid."
}

# ── nginx config generation & reload ─────────────────────────────────────────
generate_nginx_config() {
    local domain="$1" slot="$2"
    local app_backend; app_backend=$(slot_app_service "$slot")
    local fe_backend;  fe_backend=$(slot_frontend_service "$slot")
    local out_dir="$NGINX_CONF_DIR/generated/beta"
    mkdir -p "$out_dir"

    print_status "Generating nginx config for slot: $slot (backend: $app_backend, frontend: $fe_backend)"
    export_beta_nginx_template_vars "$domain" "$app_backend" "$fe_backend"
    envsubst '$DOMAIN $API_DOMAIN $MONITORING_DOMAIN $SENTRY_UI_DOMAIN $HSTS_MAX_AGE $GLOBAL_RATE_LIMIT $AUTH_RATE_LIMIT $APP_BACKEND $FRONTEND_BACKEND' \
        < "$NGINX_TEMPLATE" > "$out_dir/yamata.conf"
    print_success "nginx config written to $out_dir/yamata.conf"
}

reload_nginx() {
    print_step "Gracefully reloading nginx (zero-downtime config switch)"
    # Test config first – abort if invalid
    if ! $DOCKER_CMD exec yamata-nginx-beta nginx -t 2>&1; then
        print_error "nginx config test FAILED – aborting reload to protect live traffic."
        return 1
    fi
    $DOCKER_CMD exec yamata-nginx-beta nginx -s reload
    print_success "nginx reloaded – traffic now routes to new slot."
}

# ── Supporting services ───────────────────────────────────────────────────────
start_supporting_services() {
    print_step "Starting supporting services"
    print_status "Processing PostgreSQL init files …"
    "$PROJECT_ROOT/docker/postgres/process-init-beta.sh"

    dc up -d \
        postgres-beta \
        redis-beta \
        sentry-postgres-beta \
        sentry-redis-beta \
        sentry-beta \
        prometheus-beta \
        grafana-beta \
        postgres-backup-beta \
        postgres-exporter-beta \
        node-exporter-beta \
        cadvisor-beta
    print_success "Supporting services started."
}

wait_for_postgres() {
    print_status "Waiting for postgres-beta …"
    local attempt=1
    while [[ $attempt -le 30 ]]; do
        $DOCKER_CMD exec yamata-postgres-beta pg_isready \
            -U "${DB_USER:-yamata_user}" >/dev/null 2>&1 && {
            print_success "postgres-beta is ready."; return 0
        }
        echo "  attempt $attempt/30 …"
        sleep 5; (( attempt++ ))
    done
    print_error "postgres-beta did not become ready in time."
    return 1
}

# ── New slot lifecycle ────────────────────────────────────────────────────────
start_new_slot() {
    local slot="$1"
    print_step "Starting new slot: $slot"

    # Stop any leftover containers from a previous failed deploy to this slot
    local app_ctr; app_ctr=$(slot_app_container "$slot")
    local fe_ctr;  fe_ctr=$(slot_frontend_container "$slot")
    for ctr in "$app_ctr" "$fe_ctr"; do
        if container_running "$ctr"; then
            print_warning "Container $ctr is already running – stopping it first."
            $DOCKER_CMD stop "$ctr" 2>/dev/null || true
        fi
        if container_exists "$ctr"; then
            $DOCKER_CMD rm -f "$ctr" 2>/dev/null || true
        fi
    done

    # Override image tags so compose uses the slot-specific images.
    # docker compose reads YAML 'image:' field; we pass per-slot env vars.
    APP_BLUE_IMAGE="yamata-no-orochi:blue"   \
    APP_GREEN_IMAGE="yamata-no-orochi:green" \
    FRONTEND_BLUE_IMAGE="yamata-frontend-beta:blue"   \
    FRONTEND_GREEN_IMAGE="yamata-frontend-beta:green" \
    dc up -d "$(slot_app_service "$slot")" "$(slot_frontend_service "$slot")"

    print_success "Slot $slot containers started."
}

stop_old_slot() {
    local slot="$1"
    print_step "Stopping old slot: $slot (after ${DRAIN_SECONDS}s drain)"
    print_status "Waiting ${DRAIN_SECONDS}s for in-flight requests to finish …"
    sleep "$DRAIN_SECONDS"
    dc stop "$(slot_app_service "$slot")" "$(slot_frontend_service "$slot")" || true
    print_success "Old slot $slot stopped."
}

# ── Database migrations ───────────────────────────────────────────────────────
run_migrations() {
    print_step "Applying database migrations"
    "$PROJECT_ROOT/scripts/init-beta-database.sh" --yes
    print_success "Migrations complete."
}

# ── Nginx + auxiliary services startup (first deploy only) ───────────────────
start_nginx_and_auxiliaries() {
    print_step "Starting nginx and auxiliary services"
    dc up -d nginx-beta
    wait_for_healthy yamata-nginx-beta "nginx-beta" || true
    dc up -d cert-monitor-beta domain-monitor-beta nginx-sentry-forwarder-beta frontend-blue
    print_success "nginx and auxiliaries started."
}

# ── Deployment info ───────────────────────────────────────────────────────────
show_deployment_info() {
    local domain="$1" new_slot="$2"
    print_success "🎉 Blue-green deployment completed! Active slot: $new_slot"
    echo ""
    echo "📋 Deployment Information:"
    echo "  Domain:     https://$domain"
    echo "  API:        https://api.$domain"
    echo "  Monitoring: https://monitoring.$domain"
    echo "  Sentry:     https://sentry.$domain"
    echo "  Active slot: $new_slot"
    echo ""
    echo "🔄 To roll back run:  ./scripts/rollback-beta.sh $domain"
    echo ""
}

# ── Help ──────────────────────────────────────────────────────────────────────
show_help() {
    cat <<EOF
Usage: $0 <domain> [OPTIONS]

Options:
  --domain DOMAIN          Domain name (e.g. thewritingonthewall.com)
  --email  EMAIL           Let's Encrypt contact email
  --build                  Build backend image before deploying
  --build-frontend         Build frontend image before deploying
  --skip-migrations        Skip database migrations
  --drain-seconds N        Seconds to drain old slot (default: 30)
  --force-slot blue|green  Force deployment to a specific slot
  --help | -h              Show this help

Environment:
  DRAIN_SECONDS            Override drain wait time
  HEALTH_CHECK_ATTEMPTS    Override health-check retry count (default: 40)
  HEALTH_CHECK_INTERVAL    Override health-check interval seconds (default: 5)

Examples:
  $0 example.com
  $0 example.com --build --drain-seconds 60
  $0 example.com --force-slot green --skip-migrations
EOF
}

# ── Main ──────────────────────────────────────────────────────────────────────
main() {
    echo "🐍 Yamata no Orochi – Blue-Green Deployment"
    echo "============================================"
    echo ""

    # ── Parse arguments ──────────────────────────────────────────────────────
    local domain="" email="" do_build=false do_build_frontend=false
    local skip_migrations=false forced_slot="" custom_drain=""

    while [[ $# -gt 0 ]]; do
        case $1 in
            --domain)        domain="$2"; shift 2 ;;
            --email)         email="$2";  shift 2 ;;
            --build)         do_build=true; shift ;;
            --build-frontend) do_build_frontend=true; shift ;;
            --skip-migrations) skip_migrations=true; shift ;;
            --drain-seconds) custom_drain="$2"; shift 2 ;;
            --force-slot)
                if [[ "$2" != "blue" && "$2" != "green" ]]; then
                    print_error "--force-slot must be 'blue' or 'green'"; exit 1
                fi
                forced_slot="$2"; shift 2 ;;
            --help|-h) show_help; exit 0 ;;
            *)
                if [[ -z "$domain" ]]; then domain="$1"; shift
                else print_error "Unknown option: $1"; show_help; exit 1; fi ;;
        esac
    done

    [[ -z "$domain" ]] && domain="thewritingonthewall.com"
    [[ -z "$email"  ]] && email="admin@$domain"
    [[ -n "$custom_drain" ]] && DRAIN_SECONDS="$custom_drain"

    if [[ ! "$domain" =~ ^([a-zA-Z0-9]([-a-zA-Z0-9]{0,61}[a-zA-Z0-9])\.)+[a-zA-Z]{2,}$ ]]; then
        print_error "Invalid domain format: $domain"; exit 1
    fi

    print_status "Domain: $domain | Email: $email"

    # ── Prerequisites ────────────────────────────────────────────────────────
    resolve_docker_cmd
    check_prerequisites

    # ── Environment ──────────────────────────────────────────────────────────
    if [[ ! -f "$ENV_FILE" ]]; then
        print_error "Missing $ENV_FILE – create it before deploying."
        exit 1
    fi
    set -a; source "$ENV_FILE"; set +a

    # ── Certificates ─────────────────────────────────────────────────────────
    obtain_letsencrypt_certificates "$domain"
    validate_nginx_certificates "$domain"

    # ── Determine slots ──────────────────────────────────────────────────────
    local active_slot; active_slot=$(get_active_slot)
    local new_slot

    if [[ -n "$forced_slot" ]]; then
        new_slot="$forced_slot"
        print_warning "Forced deployment to slot: $new_slot"
    elif [[ -z "$active_slot" ]]; then
        new_slot="blue"
        print_status "First deployment → deploying to slot: $new_slot"
    else
        new_slot=$(get_opposite_slot "$active_slot")
        print_status "Active slot: $active_slot → deploying to new slot: $new_slot"
    fi

    # ── Build images ─────────────────────────────────────────────────────────
    if [[ "$do_build" == true ]]; then
        build_backend_image "$new_slot"
    else
        tag_images_for_slot "$new_slot"
    fi
    if [[ "$do_build_frontend" == true ]]; then
        build_frontend_image "$new_slot"
    fi

    # ── Generate nginx config for new slot BEFORE starting anything ──────────
    # (config is on disk; nginx will read it after reload, not before)
    generate_nginx_config "$domain" "$new_slot"

    # ── First deployment: bring up all supporting services ───────────────────
    if [[ -z "$active_slot" ]]; then
        start_supporting_services
        wait_for_postgres
    fi

    # ── Start new slot ───────────────────────────────────────────────────────
    start_new_slot "$new_slot"

    # ── Wait for new slot to become healthy ──────────────────────────────────
    wait_for_healthy "$(slot_app_container "$new_slot")" "app-$new_slot" || {
        print_error "New slot $new_slot failed health check – aborting deployment."
        print_status "Old slot '$active_slot' continues to serve traffic."
        dc stop "$(slot_app_service "$new_slot")" "$(slot_frontend_service "$new_slot")" 2>/dev/null || true
        exit 1
    }
    wait_for_healthy "$(slot_frontend_container "$new_slot")" "frontend-$new_slot" || {
        print_warning "Frontend slot $new_slot health check failed – proceeding with caution."
    }

    # ── Database migrations ──────────────────────────────────────────────────
    if [[ "$skip_migrations" == false ]]; then
        run_migrations || {
            print_error "Migrations failed – aborting deployment."
            print_status "Old slot '$active_slot' continues to serve traffic."
            dc stop "$(slot_app_service "$new_slot")" "$(slot_frontend_service "$new_slot")" 2>/dev/null || true
            exit 1
        }
    else
        print_warning "Skipping database migrations (--skip-migrations)."
    fi

    # ── Switch nginx traffic ─────────────────────────────────────────────────
    if [[ -z "$active_slot" ]]; then
        # First deploy: nginx doesn't exist yet – bring it up
        start_nginx_and_auxiliaries
    else
        # Subsequent deploy: graceful reload (zero-downtime)
        reload_nginx || {
            print_error "nginx reload failed – aborting to protect live traffic."
            dc stop "$(slot_app_service "$new_slot")" "$(slot_frontend_service "$new_slot")" 2>/dev/null || true
            exit 1
        }
    fi

    # ── Persist new slot state ───────────────────────────────────────────────
    echo "$new_slot" > "$SLOT_FILE"
    print_success "Active slot recorded: $new_slot"

    # ── Drain & stop old slot ────────────────────────────────────────────────
    if [[ -n "$active_slot" ]]; then
        stop_old_slot "$active_slot"
    fi

    # ── Done ─────────────────────────────────────────────────────────────────
    show_deployment_info "$domain" "$new_slot"
}

main "$@"
