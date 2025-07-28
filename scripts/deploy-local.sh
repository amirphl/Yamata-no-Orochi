#!/bin/bash

# Local Deployment Script for Yamata no Orochi
# This script automates the entire local deployment process including self-signed certificates

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
ENV_FILE="$PROJECT_ROOT/.env.local"
SSL_DIR="$PROJECT_ROOT/docker/nginx/ssl"
NGINX_CONF_DIR="$PROJECT_ROOT/docker/nginx/sites-available"
NGINX_TEMPLATE="$NGINX_CONF_DIR/yamata-local.conf"

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

# Function to generate random password
generate_password() {
    openssl rand -base64 32 | tr -d "=+/" | cut -c1-25
}

# Function to generate JWT secret
generate_jwt_secret() {
    openssl rand -hex 32
}

# Function to create self-signed certificates
create_self_signed_certificates() {
    local domain=$1
    
    print_status "Creating self-signed certificates for domain: $domain"
    
    # Create SSL directory if it doesn't exist
    mkdir -p "$SSL_DIR"
    
    # Generate private key
    openssl genrsa -out "$SSL_DIR/$domain.key" 2048
    
    # Create certificate signing request
    cat > "$SSL_DIR/$domain.conf" << EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
C = US
ST = State
L = City
O = Organization
OU = Organizational Unit
CN = $domain

[v3_req]
keyUsage = keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = $domain
DNS.2 = www.$domain
DNS.3 = api.$domain
DNS.4 = monitoring.$domain
DNS.5 = localhost
IP.1 = 127.0.0.1
IP.2 = ::1
EOF
    
    # Generate certificate
    openssl req -new -x509 -key "$SSL_DIR/$domain.key" -out "$SSL_DIR/$domain.crt" -days 365 -config "$SSL_DIR/$domain.conf"
    
    # Set proper permissions
    chmod 600 "$SSL_DIR/$domain.key"
    chmod 644 "$SSL_DIR/$domain.crt"
    
    # Clean up config file
    rm "$SSL_DIR/$domain.conf"
    
    print_success "Self-signed certificates created successfully"
}

# Function to generate nginx configuration from template
generate_nginx_config() {
    local domain=$1
    
    print_status "Generating nginx configuration for domain: $domain"
    
    # Create generated directory if it doesn't exist
    mkdir -p "$NGINX_CONF_DIR/generated/local"
    
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
        envsubst '$DOMAIN $API_DOMAIN $MONITORING_DOMAIN $HSTS_MAX_AGE $GLOBAL_RATE_LIMIT $AUTH_RATE_LIMIT' < "$NGINX_TEMPLATE" > "$NGINX_CONF_DIR/generated/local/yamata.conf"
        
        # Replace SSL certificate paths for local development
        # Replace main domain SSL paths
        sed -i "s|/etc/letsencrypt/live/$domain/fullchain.pem|/etc/nginx/ssl/$domain.crt|g" "$NGINX_CONF_DIR/generated/local/yamata.conf"
        sed -i "s|/etc/letsencrypt/live/$domain/privkey.pem|/etc/nginx/ssl/$domain.key|g" "$NGINX_CONF_DIR/generated/local/yamata.conf"
        sed -i "s|/etc/letsencrypt/live/$domain/chain.pem|/etc/nginx/ssl/$domain.crt|g" "$NGINX_CONF_DIR/generated/local/yamata.conf"
        
        # Replace API domain SSL paths
        sed -i "s|/etc/letsencrypt/live/api.$domain/fullchain.pem|/etc/nginx/ssl/$domain.crt|g" "$NGINX_CONF_DIR/generated/local/yamata.conf"
        sed -i "s|/etc/letsencrypt/live/api.$domain/privkey.pem|/etc/nginx/ssl/$domain.key|g" "$NGINX_CONF_DIR/generated/local/yamata.conf"
        sed -i "s|/etc/letsencrypt/live/api.$domain/chain.pem|/etc/nginx/ssl/$domain.crt|g" "$NGINX_CONF_DIR/generated/local/yamata.conf"
        
        # Replace monitoring domain SSL paths
        sed -i "s|/etc/letsencrypt/live/monitoring.$domain/fullchain.pem|/etc/nginx/ssl/$domain.crt|g" "$NGINX_CONF_DIR/generated/local/yamata.conf"
        sed -i "s|/etc/letsencrypt/live/monitoring.$domain/privkey.pem|/etc/nginx/ssl/$domain.key|g" "$NGINX_CONF_DIR/generated/local/yamata.conf"
        sed -i "s|/etc/letsencrypt/live/monitoring.$domain/chain.pem|/etc/nginx/ssl/$domain.crt|g" "$NGINX_CONF_DIR/generated/local/yamata.conf"
        
        print_success "Nginx configuration generated from template"
    else
        print_error "Nginx template not found: $NGINX_TEMPLATE"
        exit 1
    fi
}

# Function to create local environment file
create_local_env() {
    local domain=$1
    
    # Check if .env.local file already exists
    if [ -f "$ENV_FILE" ]; then
        print_status "Using existing .env.local file: $ENV_FILE"
        print_warning "If you want to regenerate the .env.local file, please remove it first: rm $ENV_FILE"
        return 0
    fi
    
    local db_password=$(generate_password)
    local jwt_secret=$(generate_jwt_secret)
    local redis_password=$(generate_password)
    local grafana_password=$(generate_password)
    
    print_status "Creating local environment configuration"
    
    # Create environment file with all configuration embedded (no comments, properly quoted)
    cat > "$ENV_FILE" << EOF
APP_ENV="development"
VERSION="0.0.1"
COMMIT_HASH="local"
BUILD_TIME="local"
DB_HOST="postgres-local"
DB_PORT="5432"
DB_NAME="yamata_no_orochi"
DB_USER="yamata_user"
DB_PASSWORD="$db_password"
DB_SSL_MODE="disable"
DB_MAX_OPEN_CONNS="100"
DB_MAX_IDLE_CONNS="10"
DB_CONN_MAX_LIFETIME="30m"
DB_CONN_MAX_IDLE_TIME="15m"
DB_SLOW_QUERY_LOG="true"
DB_SLOW_QUERY_TIME="1s"
SERVER_HOST="0.0.0.0"
SERVER_PORT="8080"
SERVER_READ_TIMEOUT="30s"
SERVER_WRITE_TIMEOUT="30s"
SERVER_IDLE_TIMEOUT="120s"
SERVER_SHUTDOWN_TIMEOUT="30s"
SERVER_BODY_LIMIT="4194304"
SERVER_ENABLE_PPROF="false"
SERVER_ENABLE_METRICS="true"
SERVER_TRUSTED_PROXIES="127.0.0.1,::1"
SERVER_PROXY_HEADER="X-Real-IP"
SERVER_ENABLE_COMPRESSION="true"
SERVER_COMPRESSION_LEVEL="6"
JWT_SECRET_KEY="$jwt_secret"
JWT_PRIVATE_KEY=""
JWT_PUBLIC_KEY=""
JWT_USE_RSA_KEYS="false"
JWT_ACCESS_TOKEN_TTL="24h"
JWT_REFRESH_TOKEN_TTL="168h"
JWT_ISSUER="yamata-no-orochi"
JWT_AUDIENCE="yamata-no-orochi-api"
JWT_ALGORITHM="HS256"
TLS_ENABLED="false"
TLS_CERT_FILE="/etc/ssl/certs/yamata.crt"
TLS_KEY_FILE="/etc/ssl/private/yamata.key"
TLS_MIN_VERSION="1.3"
HSTS_MAX_AGE="31536000"
HSTS_INCLUDE_SUBDOMAINS="true"
HSTS_PRELOAD="true"
CORS_ALLOWED_ORIGINS="https://$domain,https://www.$domain,https://api.$domain,https://monitoring.$domain,http://localhost:3000"
ALLOWED_ORIGINS="https://$domain,https://www.$domain,https://api.$domain,https://monitoring.$domain,http://localhost:3000"
CORS_ALLOWED_METHODS="GET,POST,PUT,DELETE,OPTIONS"
CORS_ALLOWED_HEADERS="Origin,Content-Type,Accept,Authorization,X-Requested-With,X-API-Key"
CORS_ALLOW_CREDENTIALS="true"
CORS_MAX_AGE="86400"
AUTH_RATE_LIMIT="20"
GLOBAL_RATE_LIMIT="2000"
RATE_LIMIT_WINDOW="1m"
RATE_LIMIT_MEMORY="64"
CSP_POLICY="default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'"
X_FRAME_OPTIONS="DENY"
X_CONTENT_TYPE_OPTIONS="nosniff"
XSS_PROTECTION="1; mode=block"
REFERRER_POLICY="strict-origin-when-cross-origin"
REQUIRE_API_KEY="false"
API_KEY_HEADER="X-API-Key"
ALLOWED_API_KEYS=""
IP_WHITELIST=""
IP_BLACKLIST=""
PASSWORD_MIN_LENGTH="8"
PASSWORD_REQUIRE_UPPER="true"
PASSWORD_REQUIRE_LOWER="true"
PASSWORD_REQUIRE_NUMBER="true"
PASSWORD_REQUIRE_SYMBOL="true"
BCRYPT_COST="12"
SESSION_COOKIE_SECURE="true"
SESSION_COOKIE_HTTPONLY="true"
SESSION_COOKIE_SAMESITE="Strict"
SESSION_TIMEOUT="24h"
SESSION_CLEANUP_INTERVAL="1h"
SMS_PROVIDER_DOMAIN="mock"
SMS_API_KEY="mock_api_key"
SMS_SOURCE_NUMBER="98**********"
SMS_RETRY_COUNT="3"
SMS_VALIDITY_PERIOD="300"
SMS_TIMEOUT="30s"
EMAIL_HOST="smtp.gmail.com"
EMAIL_PORT="587"
EMAIL_USERNAME="mock_email@gmail.com"
EMAIL_PASSWORD="mock_password"
EMAIL_FROM_EMAIL="noreply@$domain"
EMAIL_FROM_NAME="Yamata no Orochi"
EMAIL_USE_TLS="true"
EMAIL_USE_STARTTLS="true"
EMAIL_RATE_LIMIT="100"
EMAIL_RETRY_ATTEMPTS="3"
EMAIL_TIMEOUT="30s"
LOG_LEVEL="debug"
LOG_FORMAT="json"
LOG_OUTPUT="stdout"
LOG_FILE_PATH="/var/log/yamata/app.log"
LOG_MAX_SIZE="100"
LOG_MAX_BACKUPS="10"
LOG_MAX_AGE="30"
LOG_COMPRESS="true"
LOG_ENABLE_CALLER="true"
LOG_ENABLE_STACKTRACE="true"
LOG_ENABLE_ACCESS="true"
LOG_ACCESS_PATH="/var/log/yamata/access.log"
LOG_ACCESS_FORMAT="combined"
LOG_ENABLE_AUDIT="true"
LOG_AUDIT_PATH="/var/log/yamata/audit.log"
LOG_ENABLE_SECURITY="true"
LOG_SECURITY_PATH="/var/log/yamata/security.log"
METRICS_ENABLED="true"
METRICS_PORT="9090"
METRICS_PATH="/metrics"
METRICS_ENABLE_PPROF="false"
METRICS_PPROF_PORT="6060"
METRICS_ENABLE_PROMETHEUS="true"
METRICS_PROMETHEUS_PATH="/prometheus"
METRICS_COLLECT_DB="true"
METRICS_COLLECT_CACHE="true"
METRICS_COLLECT_APP="true"
CACHE_ENABLED="true"
CACHE_PROVIDER="redis"
CACHE_REDIS_URL="redis://redis-local:6379"
CACHE_REDIS_DB="0"
CACHE_REDIS_PREFIX="yamata:"
CACHE_DEFAULT_TTL="1h"
CACHE_MAX_MEMORY="256"
CACHE_CLEANUP_INTERVAL="10m"
DOMAIN="$domain"
API_DOMAIN="api.$domain"
MONITORING_DOMAIN="monitoring.$domain"
CERTBOT_EMAIL="$email"
GRAFANA_ADMIN_PASSWORD="$grafana_password"
REDIS_PASSWORD="$redis_password"
BACKUP_S3_BUCKET="yamata-backups"
BACKUP_S3_ACCESS_KEY=""
BACKUP_S3_SECRET_KEY=""
EOF
    
    # Set proper permissions
    chmod 600 "$ENV_FILE"
    
    print_success "Local environment configuration created"
    print_status "Generated passwords:"
    echo "  Database: $db_password"
    echo "  Redis: $redis_password"
    echo "  Grafana: $grafana_password"
    echo "  JWT Secret: $jwt_secret"
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
    if ! command_exists docker-compose; then
        print_error "Docker Compose is not installed. Please install Docker Compose first."
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

# Function to add domain to hosts file
add_to_hosts() {
    local domain=$1
    
    print_status "Adding domain to /etc/hosts file..."
    
    # Check if already exists
    if grep -q "$domain" /etc/hosts; then
        print_warning "Domain $domain already exists in /etc/hosts"
        return 0
    fi
    
    # Add to hosts file (requires sudo)
    if command_exists sudo; then
        echo "127.0.0.1 $domain www.$domain api.$domain monitoring.$domain" | sudo tee -a /etc/hosts > /dev/null
        print_success "Domain added to /etc/hosts"
    else
        print_warning "Please manually add the following line to /etc/hosts:"
        echo "127.0.0.1 $domain www.$domain api.$domain monitoring.$domain"
    fi
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
    
    # Check for HTTP proxy configuration
    if check_http_proxy; then
        print_status "Using HTTP proxy for Docker build"
        # Get proxy arguments
        local proxy_args=$(get_proxy_env)
        
        # Build with proxy arguments
        if [ -n "$proxy_args" ]; then
            print_status "Building with proxy: $proxy_args"
            docker build $proxy_args --network host -f docker/Dockerfile.production -t yamata-no-orochi .
        else
            print_status "Building without proxy"
            docker build --network host -f docker/Dockerfile.production -t yamata-no-orochi .
        fi
    else
        print_status "Building without proxy"
        docker build --network host -f docker/Dockerfile.production -t yamata-no-orochi .
    fi
    
    # Start services
    docker compose -f docker-compose.local.yml up -d
    
    print_success "Services started successfully"
}

# Function to wait for services to be ready
wait_for_services() {
    print_status "Waiting for services to be ready..."
    
    local max_attempts=30
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        if docker-compose -f docker-compose.local.yml ps | grep -q "Up"; then
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
    
    print_success "🎉 Local deployment completed successfully!"
    echo ""
    echo "📋 Deployment Information:"
    echo "  Domain: https://$domain"
    echo "  API: https://api.$domain"
    echo "  Monitoring: https://monitoring.$domain"
    echo ""
    echo "⚠️  Important Notes:"
    echo "  - Self-signed certificates are used (browser will show security warning)"
    echo "  - Accept the certificate warning in your browser"
    echo "  - All services are running in development mode"
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
    echo "  - If .env.local file exists, it will be used (preserves your custom settings)"
    echo "  - If .env.local doesn't exist, a new one will be created with generated passwords"
    echo "  - To regenerate .env.local: rm .env.local && $0 <domain>"
    echo ""
    echo "Examples:"
    echo "  $0 yourdomain.com                    # Use existing .env.local or create new one"
    echo "  $0 yourdomain.com --domain=yourdomain.com --email=admin@yourdomain.com"
    echo ""
}

# Main function
main() {
    echo "🐍 Yamata no Orochi - Local Deployment"
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
    
    # Validate domain format
    if [[ ! "$domain" =~ ^[a-zA-Z0-9][a-zA-Z0-9-]{1,61}[a-zA-Z0-9]\.[a-zA-Z]{2,}$ ]]; then
        print_error "Invalid domain format: $domain"
        echo "Please provide a valid domain name (e.g., thewritingonthewall.com)"
        exit 1
    fi
    
    print_status "Starting local deployment for domain: $domain (Email: $email)"
    
    # Check and display proxy information
    echo ""
    print_status "Checking HTTP proxy configuration..."
    check_http_proxy
    echo ""
    
    # Check prerequisites
    check_prerequisites
    
    # Create self-signed certificates
    create_self_signed_certificates "$domain"
    
    # Generate nginx configuration from template
    generate_nginx_config "$domain"
    
    # Create local environment file
    create_local_env "$domain"
    
    # Add domain to hosts file
    add_to_hosts "$domain"
    
    # Start services
    start_services
    
    # Initialize database and apply migrations
    print_status "Initializing database and applying migrations..."
    
    # Source environment variables for database initialization
    if [ -f "$ENV_FILE" ]; then
        # Use set -a to automatically export variables, then source the file
        set -a
        source "$ENV_FILE"
        set +a
    fi
    
    if ./scripts/init-local-database.sh; then
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