#!/bin/bash

# Beta Deployment Script for Yamata no Orochi
# This script automates the beta deployment process and validates pre-provisioned certificates

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
ENV_FILE="$PROJECT_ROOT/.env.beta"
NGINX_CONF_DIR="$PROJECT_ROOT/docker/nginx/sites-available"
NGINX_TEMPLATE="$NGINX_CONF_DIR/yamata-beta.conf"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
	echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
	echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
	echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
	echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if command exists
command_exists() {
	command -v "$1" >/dev/null 2>&1
}

# Helper to resolve docker command (uses sudo if required)
get_docker_cmd() {
	if docker info >/dev/null 2>&1; then
		echo "docker"
	elif command_exists sudo && sudo -n docker info >/dev/null 2>&1; then
		echo "sudo docker"
	else
		echo "docker"
	fi
}

# Function to generate random password
generate_password() {
	openssl rand -base64 32 | tr -d "=+/" | cut -c1-25
}

# Function to generate JWT secret
generate_jwt_secret() {
	openssl rand -hex 32
}

# Export the variables required to render the beta nginx template safely.
export_beta_nginx_template_vars() {
	local domain=$1

	export DOMAIN="$domain"
	export API_DOMAIN="api.$domain"
	export MONITORING_DOMAIN="monitoring.$domain"
	export SENTRY_UI_DOMAIN="sentry.$domain"
	export HSTS_MAX_AGE="31536000"
	export GLOBAL_RATE_LIMIT="1000"
	export AUTH_RATE_LIMIT="10"
}

# Validate that every certificate path referenced in yamata-beta.conf exists and is valid
validate_nginx_certificates() {
	local domain=$1

	print_status "Validating certificates referenced in yamata-beta.conf"

	# Render the template into a temporary file with the provided domain values
	local tmp_conf
	tmp_conf=$(mktemp)

	export_beta_nginx_template_vars "$domain"

	if ! envsubst '$DOMAIN $API_DOMAIN $MONITORING_DOMAIN $SENTRY_UI_DOMAIN $HSTS_MAX_AGE $GLOBAL_RATE_LIMIT $AUTH_RATE_LIMIT' < "$NGINX_TEMPLATE" > "$tmp_conf"; then
		rm -f "$tmp_conf"
		print_error "Failed to render nginx template for certificate validation"
		exit 1
	fi

	local failed=false

	# Extract certificate-related file paths (2nd field on ssl_* lines)
	while IFS= read -r cert_path; do
		# Trim trailing semicolon if present
		cert_path=${cert_path%;}
		if [ -z "$cert_path" ]; then
			continue
		fi

		if [ ! -f "$cert_path" ]; then
			print_error "Missing certificate file: $cert_path"
			failed=true
			continue
		fi

		# Skip expiry check for private keys
		if [[ "$cert_path" == *privkey* ]]; then
			print_success "Found key file: $cert_path"
			continue
		fi

		if ! expiry_date=$(openssl x509 -enddate -noout -in "$cert_path" 2>/dev/null | cut -d= -f2); then
			print_error "Invalid certificate file: $cert_path"
			failed=true
			continue
		fi
		if ! expiry_ts=$(date -d "$expiry_date" +%s 2>/dev/null); then
			print_error "Invalid certificate expiry date for $cert_path: $expiry_date"
			failed=true
			continue
		fi
		now_ts=$(date +%s)
		days_left=$(( (expiry_ts - now_ts) / 86400 ))

		# Ensure certificate exists and is not expired.
		if openssl x509 -checkend 0 -noout -in "$cert_path" >/dev/null 2>&1; then
			print_success "Certificate valid: $cert_path (expires in ${days_left} days on ${expiry_date})"
		else
			print_error "Certificate expired: $cert_path (expires in ${days_left} days on ${expiry_date})"
			failed=true
		fi
		
	done < <(grep -E "^\s*ssl_(certificate|certificate_key|trusted_certificate)" "$tmp_conf" | awk '{print $2}')

	rm -f "$tmp_conf"

	if [ "$failed" = true ]; then
		print_error "Certificate validation failed. Fix missing/expired certificates before deploying."
		exit 1
	fi

	print_success "All referenced certificates exist and are valid"
}

# Function to generate nginx configuration from template
generate_nginx_config() {
	local domain=$1
	
	print_status "Generating nginx configuration for domain: $domain"
	
	# Create generated directory if it doesn't exist
	mkdir -p "$NGINX_CONF_DIR/generated/beta"

	# Set environment variables for template processing
	export_beta_nginx_template_vars "$domain"
	
	# Read the template and replace environment variables
	if [ -f "$NGINX_TEMPLATE" ]; then
		# Process the template with only specific environment variable substitution
		# Use envsubst with specific variables to avoid interfering with Nginx variables
		envsubst '$DOMAIN $API_DOMAIN $MONITORING_DOMAIN $SENTRY_UI_DOMAIN $HSTS_MAX_AGE $GLOBAL_RATE_LIMIT $AUTH_RATE_LIMIT' < "$NGINX_TEMPLATE" > "$NGINX_CONF_DIR/generated/beta/yamata.conf"
		
		# SSL certificate paths are expected to be present in the rendered template.
		
		# Replace upstream server addresses for beta development
		sed -i "s|server app:8080 max_fails=3 fail_timeout=30s;|server app-beta:8080 max_fails=3 fail_timeout=30s;|g" "$NGINX_CONF_DIR/generated/beta/yamata.conf"
		sed -i "s|server app:9090 max_fails=3 fail_timeout=30s;|server app-beta:9090 max_fails=3 fail_timeout=30s;|g" "$NGINX_CONF_DIR/generated/beta/yamata.conf"

		print_success "Nginx configuration generated from template"
	else
		print_error "Nginx template not found: $NGINX_TEMPLATE"
		exit 1
	fi
}

# Function to create beta environment file
create_beta_env() {
	local domain=$1
	
	# Check if .env.beta file already exists
	if [ -f "$ENV_FILE" ]; then
		print_status "Using existing .env.beta file: $ENV_FILE"
		print_warning "If you want to regenerate the .env.beta file, please remove it first: rm $ENV_FILE"
		return 0
	fi

	print_error "No $ENV_FILE file found"
	
	exit 1
}

# Function to check prerequisites
check_prerequisites() {
	print_status "Checking prerequisites..."
	
	# Check Docker
	if ! command_exists docker; then
		print_error "Docker is not installed. Please install Docker first."
		exit 1
	fi
	
	# Check if docker compose is available (Docker Compose V2)
	if ! docker compose version >/dev/null 2>&1; then
		print_error "Docker Compose is not available. Please ensure Docker Compose V2 is installed."
		exit 1
	fi
	
	# Check OpenSSL
	if ! command_exists openssl; then
		print_error "OpenSSL is not installed. Please install OpenSSL first."
		exit 1
	fi
	
	# Check if Docker daemon is running
	if ! docker info >/dev/null 2>&1; then
		print_error "Docker daemon is not running. Please start Docker first."
		exit 1
	fi
	
	print_success "All prerequisites are satisfied"
}

# Function to check for HTTP proxy environment variables
check_http_proxy() {
	local proxy_found=false
	
	# Check for HTTP proxy in various formats
	if [ -n "$HTTP_PROXY" ]; then
		print_status "Found HTTP_PROXY: $HTTP_PROXY"
		proxy_found=true
	fi
	
	if [ -n "$http_proxy" ]; then
		print_status "Found http_proxy: $http_proxy"
		proxy_found=true
	fi
	
	if [ -n "$HTTPS_PROXY" ]; then
		print_status "Found HTTPS_PROXY: $HTTPS_PROXY"
		proxy_found=true
	fi
	
	if [ -n "$https_proxy" ]; then
		print_status "Found https_proxy: $https_proxy"
		proxy_found=true
	fi
	
	if [ "$proxy_found" = true ]; then
		print_success "HTTP proxy configuration detected"
		return 0
	else
		print_warning "No HTTP proxy configuration found"
		return 0
	fi
}

# Function to get proxy environment variables
get_proxy_env() {
	local proxy_args=""
	
	# Add HTTP proxy if set
	if [ -n "$HTTP_PROXY" ]; then
		proxy_args="$proxy_args --build-arg HTTP_PROXY=$HTTP_PROXY"
	elif [ -n "$http_proxy" ]; then
		proxy_args="$proxy_args --build-arg HTTP_PROXY=$http_proxy"
	fi
	
	# Add HTTPS proxy if set
	if [ -n "$HTTPS_PROXY" ]; then
		proxy_args="$proxy_args --build-arg HTTPS_PROXY=$HTTPS_PROXY"
	elif [ -n "$https_proxy" ]; then
		proxy_args="$proxy_args --build-arg HTTPS_PROXY=$https_proxy"
	fi
	
	# Add NO_PROXY if set
	if [ -n "$NO_PROXY" ]; then
		proxy_args="$proxy_args --build-arg NO_PROXY=$NO_PROXY"
	elif [ -n "$no_proxy" ]; then
		proxy_args="$proxy_args --build-arg NO_PROXY=$no_proxy"
	fi
	
	echo "$proxy_args"
}

# Function to start services (all except app-beta)
start_services() {
	print_status "Starting Docker Compose services (excluding app-beta)..."
	
	# Resolve docker command (fallback to sudo if needed)
	local docker_cmd
	docker_cmd=$(get_docker_cmd)
	
	# Process init.sql with environment variables for beta environment
	print_status "Processing PostgreSQL init.sql for beta environment..."
	if [ -f "docker/postgres/process-init-beta.sh" ]; then
		./docker/postgres/process-init-beta.sh
		if [ $? -ne 0 ]; then
			print_error "Failed to process init.sql for beta environment"
			return 1
		fi
	else
		print_error "process-init-beta.sh not found"
		return 1
	fi
	
	# Start all supporting services explicitly, excluding app-beta to allow safe DB migration
	# Note: nginx-beta depends on app-beta, so we start it after app-beta to avoid pulling app-beta up implicitly
	$docker_cmd compose -f docker-compose.beta.yml up -d \
		postgres-beta \
		redis-beta \
		sentry-postgres-beta \
		sentry-redis-beta \
		sentry-beta \
		prometheus-beta \
		grafana-beta \
		frontend-beta \
		postgres-backup-beta \
		postgres-exporter-beta \
		node-exporter-beta \
		cadvisor-beta

	print_success "Core services started successfully (app-beta not started)"
}

# Function to wait for services to be ready
wait_for_services() {
	print_status "Waiting for services to be ready..."
	
	# Resolve docker command (fallback to sudo if needed)
	local docker_cmd
	docker_cmd=$(get_docker_cmd)
	
	local max_attempts=30
	local attempt=1
	
	while [ $attempt -le $max_attempts ]; do
		if $docker_cmd compose -f docker-compose.beta.yml ps | grep -q "Up"; then
			print_success "Services are ready!"
			return 0
		fi
		
		echo "Attempt $attempt/$max_attempts - Waiting for services..."
		sleep 10
		attempt=$((attempt + 1))
	done
	
	print_error "Services failed to start within expected time"
	return 1
}

# Wait for app-beta container to become healthy
wait_for_app_health() {
	print_status "Waiting for app-beta health..."
	local docker_cmd
	docker_cmd=$(get_docker_cmd)
	local max_attempts=40
	local attempt=1
	local status="unknown"
	while [ $attempt -le $max_attempts ]; do
		status=$($docker_cmd inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' yamata-app-beta 2>/dev/null || echo "unknown")
		if [ "$status" = "healthy" ]; then
			print_success "app-beta is healthy"
			return 0
		fi
		print_status "Attempt $attempt/$max_attempts - app-beta health status: $status"
		sleep 5
		attempt=$((attempt + 1))
	done
	print_warning "app-beta did not become healthy within expected time (last status: $status)"
	return 0
}

# Function to start app-beta (and nginx-beta after)
start_app_service() {
	print_status "Starting app-beta and nginx-beta services..."
	local docker_cmd
	docker_cmd=$(get_docker_cmd)
	$docker_cmd compose -f docker-compose.beta.yml up -d app-beta
	print_success "app-beta started"
	# Start nginx after app-beta to avoid implicit dependency startup
	$docker_cmd compose -f docker-compose.beta.yml up -d nginx-beta
	print_success "nginx-beta started"
	# Start services that depend on nginx after the proxy is up.
	$docker_cmd compose -f docker-compose.beta.yml up -d cert-monitor-beta
	print_success "cert-monitor-beta started"
	$docker_cmd compose -f docker-compose.beta.yml up -d nginx-sentry-forwarder-beta
	print_success "nginx-sentry-forwarder-beta started"
}

# Function to display deployment information
show_deployment_info() {
	local domain=$1
	
	print_success "🎉 Beta deployment completed successfully!"
	echo ""
	echo "📋 Deployment Information:"
	echo "  Domain: https://$domain"
	echo "  API: https://api.$domain"
	echo "  Monitoring: https://monitoring.$domain"
	echo "  Sentry: https://sentry.$domain"
	echo ""
	echo "⚠️  Important Notes:"
	echo "  - SSL certificates must already exist and be valid"
	echo "  - All services are running in beta mode"
	echo ""
	echo "🚀 Your application is ready at: https://$domain"
}

# Function to show help message
show_help() {
	echo "Usage: $0 <domain> [OPTIONS]"
	echo ""
	echo "Arguments:"
	echo "  domain              Domain name (e.g., thewritingonthewall.com)"
	echo ""
	echo "Options:"
	echo "  --domain            Override the default domain (e.g., yourdomain.com)"
	echo "  --help, -h          Show this help message"
	echo ""
	echo "Environment Configuration:"
	echo "  - If .env.beta file exists, it will be used (preserves your custom settings)"
	echo "  - If .env.beta doesn't exist, a new one will be created with generated passwords"
	echo "  - To regenerate .env.beta: rm .env.beta && $0 <domain>"
	echo ""
	echo "SSL Certificate Configuration:"
	echo "  - Certificates must be obtained before running this script"
	echo "  - The script only checks that certificate files referenced by nginx exist and are not expired"
	echo ""
	echo "Examples:"
	echo "  $0 yourdomain.com                    # Use existing .env.beta or create new one"
	echo "  $0 yourdomain.com --domain=yourdomain.com"
	echo ""
}

# Main function
main() {
	echo "🐍 Yamata no Orochi - Beta Deployment"
	echo "======================================"
	echo ""
	
	# Parse command line arguments
	local domain="" # Default domain, can be overridden by argument
	
	# Parse command line arguments
	while [[ $# -gt 0 ]]; do
		case $1 in
			--domain)
				domain="$2"
				shift 2
				;;
			--help|-h)
				show_help
				exit 0
				;;
			*)
				if [ -z "$domain" ]; then
					domain=$1 # First argument is domain if not an option
				else
					print_error "Unknown option or multiple domains specified: $1"
					show_help
					exit 1
				fi
				shift
				;;
		esac
	done
	
	# Set default domain if not provided
	if [ -z "$domain" ]; then
		domain="thewritingonthewall.com"
	fi
	# Validate domain
	if [ -z "$domain" ]; then
		print_error "Domain name is required."
		show_help
		exit 1
	fi
	
	# Validate domain format (supports subdomains)
	if [[ ! "$domain" =~ ^([a-zA-Z0-9]([-a-zA-Z0-9]{0,61}[a-zA-Z0-9])\.)+[a-zA-Z]{2,}$ ]]; then
		print_error "Invalid domain format: $domain"
		echo "Please provide a valid domain name (e.g., thewritingonthewall.com)"
		exit 1
	fi
	
	print_status "Starting beta deployment for domain: $domain"
	
	# Check and display proxy information
	echo ""
	print_status "Checking HTTP proxy configuration..."
	check_http_proxy
	echo ""
	
	# Check prerequisites
	check_prerequisites

	# Validate certificate files referenced by nginx config
	validate_nginx_certificates "$domain"
	
	# Generate nginx configuration from template
	generate_nginx_config "$domain"
	
	# Create beta environment file
	create_beta_env "$domain"
	
	# Source environment variables for database initialization
	if [ -f "$ENV_FILE" ]; then
		# Use set -a to automatically export variables, then source the file
		set -a
		source "$ENV_FILE"
		set +a
	fi

	# Start core services (excluding app-beta)
	start_services
	
	# Initialize database and apply migrations
	print_status "Initializing database and applying migrations..."
	
	if ./scripts/init-beta-database.sh; then
		print_success "Database initialization completed"
	else
		print_warning "Database initialization failed or was skipped"
	fi
	
	# Start app service after successful migrations
	start_app_service
	
	# Wait for app-beta to report healthy status
	wait_for_app_health
	
	# Show deployment information
	show_deployment_info "$domain"
}

# Run main function with all arguments
main "$@"
