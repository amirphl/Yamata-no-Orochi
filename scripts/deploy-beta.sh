#!/bin/bash

# Beta Deployment Script for Yamata no Orochi
# This script automates the entire beta deployment process including Let's Encrypt certificates via acme.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
ENV_FILE="$PROJECT_ROOT/.env.beta"
SSL_DIR="$PROJECT_ROOT/docker/nginx/ssl"
NGINX_CONF_DIR="$PROJECT_ROOT/docker/nginx/sites-available"
NGINX_TEMPLATE="$NGINX_CONF_DIR/yamata-beta.conf"
LETSENCRYPT_DIR="/etc/letsencrypt"
ACME_SH_DIR="$HOME/.acme.sh"

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

# Function to check if acme.sh is installed
check_acme_sh() {
	if ! command_exists "$ACME_SH_DIR/acme.sh"; then
		print_error "acme.sh is not installed. Please install it first:"
		print_status "curl https://get.acme.sh | sh"
		exit 1
	fi
}

# Function to check certificate validity
check_certificate_validity() {
	local domain=$1
	local cert_file="$LETSENCRYPT_DIR/live/$domain/fullchain.pem"
	
	if [ ! -f "$cert_file" ]; then
		return 1
	fi
	
	# Check if certificate is valid and not expired
	local expiry_date=$(openssl x509 -enddate -noout -in "$cert_file" | cut -d= -f2)
	local expiry_timestamp=$(date -d "$expiry_date" +%s)
	local current_timestamp=$(date +%s)
	local days_until_expiry=$(( (expiry_timestamp - current_timestamp) / 86400 ))
	
	if [ $days_until_expiry -gt 30 ]; then
		print_success "Certificate for $domain is valid and expires in $days_until_expiry days"
		return 0
	else
		print_warning "Certificate for $domain expires in $days_until_expiry days"
		return 1
	fi
}

# Function to obtain Let's Encrypt certificates
obtain_letsencrypt_certificates() {
	local domain=$1
	
	print_status "Checking/obtaining Let's Encrypt certificates for domain: $domain"
	
	# Check if acme.sh is installed
	check_acme_sh
	
	# Create necessary directories
	sudo mkdir -p "$LETSENCRYPT_DIR/live/$domain"
	sudo mkdir -p "$LETSENCRYPT_DIR/archive/$domain"
	
	# Check if certificate exists and is valid
	if check_certificate_validity "$domain"; then
		print_success "Valid certificate already exists for $domain"
		return 0
	fi
	
	# Certificate doesn't exist or is expired, obtain new one
	print_status "Obtaining new Let's Encrypt certificate for $domain"
	
	# Stop nginx temporarily if running
	if systemctl is-active --quiet nginx; then
		print_status "Stopping nginx temporarily for certificate issuance"
		sudo systemctl stop nginx
	fi
	
	# Obtain certificate using acme.sh with HTTP challenge
	if "$ACME_SH_DIR/acme.sh" --issue -d "$domain" -d "www.$domain" -d "api.$domain" -d "monitoring.$domain" \
		--webroot /var/www/html --server letsencrypt; then
		
		print_success "Certificate obtained successfully for $domain"
		
		# Install certificate to Let's Encrypt directory structure
		"$ACME_SH_DIR/acme.sh" --install-cert -d "$domain" \
			--cert-file "$LETSENCRYPT_DIR/live/$domain/cert.pem" \
			--key-file "$LETSENCRYPT_DIR/live/$domain/privkey.pem" \
			--fullchain-file "$LETSENCRYPT_DIR/live/$domain/fullchain.pem" \
			--chain-file "$LETSENCRYPT_DIR/live/$domain/chain.pem"
		
		# Set proper permissions
		sudo chmod 644 "$LETSENCRYPT_DIR/live/$domain/cert.pem"
		sudo chmod 600 "$LETSENCRYPT_DIR/live/$domain/privkey.pem"
		sudo chmod 644 "$LETSENCRYPT_DIR/live/$domain/fullchain.pem"
		sudo chmod 644 "$LETSENCRYPT_DIR/live/$domain/chain.pem"
		
		# Set ownership
		sudo chown -R root:root "$LETSENCRYPT_DIR/live/$domain"
		
	else
		print_error "Failed to obtain certificate for $domain"
		print_warning "Make sure:"
		print_warning "1. Domain $domain points to this server"
		print_warning "2. Port 80 is open and accessible"
		print_warning "3. /var/www/html is writable by acme.sh"
		return 1
	fi
	
	# Restart nginx if it was running
	if ! systemctl is-active --quiet nginx; then
		print_status "Starting nginx"
		sudo systemctl start nginx
	fi
	
	print_success "Let's Encrypt certificates obtained successfully"
}

# Function to generate nginx configuration from template
generate_nginx_config() {
	local domain=$1
	
	print_status "Generating nginx configuration for domain: $domain"
	
	# Create generated directory if it doesn't exist
	mkdir -p "$NGINX_CONF_DIR/generated/beta"
	
	# Set environment variables for template processing
	export DOMAIN="$domain"
	export API_DOMAIN="api.$domain"
	export MONITORING_DOMAIN="monitoring.$domain"
	export HSTS_MAX_AGE="31536000"
	export GLOBAL_RATE_LIMIT="1000"
	export AUTH_RATE_LIMIT="10"
	
	# Read the template and replace environment variables
	if [ -f "$NGINX_TEMPLATE" ]; then
		# Process the template with only specific environment variable substitution
		# Use envsubst with specific variables to avoid interfering with Nginx variables
		envsubst '$DOMAIN $API_DOMAIN $MONITORING_DOMAIN $HSTS_MAX_AGE $GLOBAL_RATE_LIMIT $AUTH_RATE_LIMIT' < "$NGINX_TEMPLATE" > "$NGINX_CONF_DIR/generated/beta/yamata.conf"
		
		# SSL certificate paths are already correct for Let's Encrypt
		# No need to replace paths as they already point to /etc/letsencrypt/live/
		
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
	
	# Check Docker Compose
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
	
	# Check acme.sh
	if ! command_exists "$ACME_SH_DIR/acme.sh"; then
		print_warning "acme.sh is not installed. Installing now..."
		print_status "Installing acme.sh..."
		curl https://get.acme.sh | sh
		source "$HOME/.bashrc" 2>/dev/null || source "$HOME/.zshrc" 2>/dev/null || true
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

# Function to start services
start_services() {
	print_status "Starting Docker Compose services..."
	
	# Resolve docker command (fallback to sudo if needed)
	local docker_cmd
	docker_cmd=$(get_docker_cmd)
	
	# Check for HTTP proxy configuration
	if check_http_proxy; then
		print_status "Using HTTP proxy for Docker build"
		# Get proxy arguments
		local proxy_args=$(get_proxy_env)
		
		# Build with proxy arguments
		if [ -n "$proxy_args" ]; then
			print_status "Building with proxy: $proxy_args"
			$docker_cmd build $proxy_args --network host -f docker/Dockerfile.production -t yamata-no-orochi .
		else
			print_status "Building without proxy"
			$docker_cmd build --network host -f docker/Dockerfile.production -t yamata-no-orochi .
		fi
	else
		print_status "Building without proxy"
		$docker_cmd build --network host -f docker/Dockerfile.production -t yamata-no-orochi .
	fi
	
	# Start services
	$docker_cmd compose -f docker-compose.beta.yml up -d
	
	print_success "Services started successfully"
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

# Function to display deployment information
show_deployment_info() {
	local domain=$1
	
	print_success "🎉 Beta deployment completed successfully!"
	echo ""
	echo "📋 Deployment Information:"
	echo "  Domain: https://$domain"
	echo "  API: https://api.$domain"
	echo "  Monitoring: https://monitoring.$domain"
	echo ""
	echo "⚠️  Important Notes:"
	echo "  - Let's Encrypt certificates are used (browser will show valid SSL)"
	echo "  - SSL certificates are automatically managed by Let's Encrypt"
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
	echo "  --email             Override the default email for Let's Encrypt (e.g., admin@yourdomain.com)"
	echo "  --help, -h          Show this help message"
	echo ""
	echo "Environment Configuration:"
	echo "  - If .env.beta file exists, it will be used (preserves your custom settings)"
	echo "  - If .env.beta doesn't exist, a new one will be created with generated passwords"
	echo "  - To regenerate .env.beta: rm .env.beta && $0 <domain>"
	echo ""
	echo "SSL Certificate Configuration:"
	echo "  - Uses Let's Encrypt certificates via acme.sh"
	echo "  - Automatically checks certificate validity and renews if needed"
	echo "  - Requires domain to point to this server and port 80 to be accessible"
	echo "  - Certificates are stored in /etc/letsencrypt/live/<domain>/"
	echo ""
	echo "Examples:"
	echo "  $0 yourdomain.com                    # Use existing .env.beta or create new one"
	echo "  $0 yourdomain.com --domain=yourdomain.com --email=admin@yourdomain.com"
	echo ""
}

# Main function
main() {
	echo "🐍 Yamata no Orochi - Beta Deployment"
	echo "======================================"
	echo ""
	
	# Parse command line arguments
	local domain="" # Default domain, can be overridden by argument
	local email=""  # Default email, can be overridden by argument
	
	# Parse command line arguments
	while [[ $# -gt 0 ]]; do
		case $1 in
			--domain)
				domain="$2"
				shift 2
				;;
			--email)
				email="$2"
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
	# Set default email if not provided
	if [ -z "$email" ]; then
		email="admin@thewritingonthewall.com"
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
	
	print_status "Starting beta deployment for domain: $domain (Email: $email)"
	
	# Check and display proxy information
	echo ""
	print_status "Checking HTTP proxy configuration..."
	check_http_proxy
	echo ""
	
	# Check prerequisites
	check_prerequisites
	
	# Obtain Let's Encrypt certificates
	obtain_letsencrypt_certificates "$domain"
	
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

	# Start services
	start_services
	
	# Initialize database and apply migrations
	print_status "Initializing database and applying migrations..."	
	
	if ./scripts/init-beta-database.sh; then
		print_success "Database initialization completed"
	else
		print_warning "Database initialization failed or was skipped"
	fi
	
	# Wait for services to be ready
	wait_for_services
	
	# Show deployment information
	show_deployment_info "$domain"
}

# Run main function with all arguments
main "$@"