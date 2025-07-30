#!/bin/bash

# Development script to run the application with Swagger UI enabled
# This script runs the app without Docker in the simplest manner

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
TEMP_DIR="$PROJECT_ROOT/tmp"

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

# Function to check if port is available
check_port() {
    local port=$1
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

# Function to generate JWT secret
generate_jwt_secret() {
    openssl rand -hex 32
}

# Function to check prerequisites
check_prerequisites() {
    print_status "Checking prerequisites..."
    
    # Check Go
    if ! command_exists go; then
        print_error "Go is not installed. Please install Go first."
        exit 1
    fi
    
    # Check PostgreSQL client tools
    if ! command_exists psql; then
        print_warning "PostgreSQL client (psql) not found. Please install PostgreSQL client tools."
        print_warning "You may need to install: sudo apt-get install postgresql-client"
        print_warning "Migration functionality will not be available."
    fi
    
    if ! command_exists createdb; then
        print_warning "PostgreSQL client (createdb) not found. Please install PostgreSQL client tools."
        print_warning "You may need to install: sudo apt-get install postgresql-client"
        print_warning "Database creation functionality will not be available."
    fi
    
    # Check if PostgreSQL is running
    if ! pg_isready -h localhost -p 5432 >/dev/null 2>&1; then
        print_warning "PostgreSQL is not running on localhost:5432"
        print_warning "Please start PostgreSQL or use Docker:"
        print_warning "  docker run --name postgres-dev -e POSTGRES_PASSWORD=yamata_password -e POSTGRES_USER=yamata_user -e POSTGRES_DB=yamata_db -p 5432:5432 -d postgres:15"
    fi
    
    # Check if Redis is running (optional for development)
    if command_exists redis-cli; then
        if ! redis-cli ping >/dev/null 2>&1; then
            print_warning "Redis is not running. Some features may not work properly."
            print_warning "To start Redis: docker run --name redis-dev -p 6379:6379 -d redis:7-alpine"
        fi
    else
        print_warning "Redis client not found. Some features may not work properly."
    fi
    
    print_success "Prerequisites check completed"
}

# Function to create temporary directory
create_temp_dir() {
    print_status "Creating temporary directory..."
    mkdir -p "$TEMP_DIR"
    print_success "Temporary directory created: $TEMP_DIR"
}

# Function to generate JWT keys
generate_jwt_keys() {
    print_status "Generating JWT keys..."
    
    local private_key_file="$TEMP_DIR/private.pem"
    local public_key_file="$TEMP_DIR/public.pem"
    
    # Generate private key
    openssl genrsa -out "$private_key_file" 2048 2>/dev/null
    
    # Generate public key
    openssl rsa -in "$private_key_file" -pubout -out "$public_key_file" 2>/dev/null
    
    # Set proper permissions
    chmod 600 "$private_key_file"
    chmod 644 "$public_key_file"
    
    # Export as environment variables
    export JWT_PRIVATE_KEY=$(cat "$private_key_file")
    export JWT_PUBLIC_KEY=$(cat "$public_key_file")
    
    print_success "JWT keys generated successfully"
}

# Function to set environment variables
set_environment() {
    print_status "Setting development environment variables..."
    
    # Generate JWT secret
    local jwt_secret=$(generate_jwt_secret)
    
    # Set environment variables
    export APP_ENV=development
    export DB_HOST=localhost
    export DB_PORT=5432
    export DB_USER=postgres
    export DB_PASSWORD=postgres
    export DB_NAME=yamata_db
    export DB_SSL_MODE=disable
    export DB_MAX_OPEN_CONNS=10
    export DB_MAX_IDLE_CONNS=5
    export DB_CONN_MAX_LIFETIME=30m
    export DB_CONN_MAX_IDLE_TIME=15m
    export SERVER_HOST=0.0.0.0
    export SERVER_PORT=8080
    export SERVER_READ_TIMEOUT=30s
    export SERVER_WRITE_TIMEOUT=30s
    export SERVER_IDLE_TIMEOUT=120s
    export SERVER_SHUTDOWN_TIMEOUT=30s
    export JWT_SECRET_KEY="$jwt_secret"
    export JWT_ACCESS_TOKEN_TTL=15m
    export JWT_REFRESH_TOKEN_TTL=7d
    export JWT_ISSUER=yamata-dev
    export JWT_AUDIENCE=yamata-users
    export JWT_USE_RSA_KEYS=false
    export JWT_ALGORITHM=HS256
    export LOG_LEVEL=debug
    export LOG_FORMAT=json
    export LOG_OUTPUT=stdout
    export METRICS_ENABLED=true
    export METRICS_PORT=9090
    export CACHE_ENABLED=false
    export SMS_PROVIDER=mock
    export EMAIL_HOST=smtp.gmail.com
    export EMAIL_PORT=587
    export EMAIL_USERNAME=mock_email@gmail.com
    export EMAIL_PASSWORD=mock_password
    export EMAIL_FROM_EMAIL=noreply@localhost
    export EMAIL_FROM_NAME="Yamata no Orochi (Dev)"
    export EMAIL_USE_TLS=true
    export EMAIL_USE_STARTTLS=true
    
    print_success "Environment variables set for development"
    print_status "APP_ENV: $APP_ENV"
    print_status "Server will start on: $SERVER_HOST:$SERVER_PORT"
    print_status "Database: $DB_HOST:$DB_PORT/$DB_NAME"
}

# Function to build application
build_application() {
    print_status "Building application..."
    
    # Download dependencies
    go mod download
    
    # Build the application
    go build -o "$TEMP_DIR/main" .
    
    print_success "Application built successfully: $TEMP_DIR/main"
}

# Function to initialize database
initialize_database() {
    print_status "Checking database connection..."
    
    # Test database connection
    if pg_isready -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" >/dev/null 2>&1; then
        print_success "Database connection successful"
        
        # Check if database exists
        if psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "SELECT 1;" >/dev/null 2>&1; then
            print_success "Database '$DB_NAME' exists and is accessible"
            
            # Ask user if they want to run migrations
            print_status "Do you want to run database migrations? (y/N)"
            read -r run_migrations
            if [[ "$run_migrations" =~ ^[Yy]$ ]]; then
                run_migrations
            fi
        else
            print_warning "Database '$DB_NAME' does not exist or is not accessible"
            print_warning "Please create the database:"
            print_warning "  createdb -h $DB_HOST -p $DB_PORT -U $DB_USER $DB_NAME"
            
            # Ask user if they want to create database and run migrations
            print_status "Do you want to create the database and run migrations? (y/N)"
            read -r create_and_migrate
            if [[ "$create_and_migrate" =~ ^[Yy]$ ]]; then
                create_database_and_migrate
            fi
        fi
    else
        print_warning "Cannot connect to database. Please ensure PostgreSQL is running."
        print_warning "You can start PostgreSQL with Docker:"
        print_warning "  docker run --name postgres-dev -e POSTGRES_PASSWORD=$DB_PASSWORD -e POSTGRES_USER=$DB_USER -e POSTGRES_DB=$DB_NAME -p $DB_PORT:5432 -d postgres:15"
    fi
}

# Function to create database and run migrations
create_database_and_migrate() {
    print_status "Creating database '$DB_NAME'..."
    
    if createdb -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" "$DB_NAME" 2>/dev/null; then
        print_success "Database '$DB_NAME' created successfully"
        run_migrations
    else
        print_error "Failed to create database '$DB_NAME'"
        print_warning "Please create the database manually:"
        print_warning "  createdb -h $DB_HOST -p $DB_PORT -U $DB_USER $DB_NAME"
    fi
}

# Function to run migrations
run_migrations() {
    print_status "Running database migrations..."
    
    # Check if migrations directory exists
    local migrations_dir="$PROJECT_ROOT/migrations"
    local run_all_up="$migrations_dir/run_all_up.sql"
    
    if [ ! -f "$run_all_up" ]; then
        print_error "Migration file not found: $run_all_up"
        return 1
    fi
    
    # Run migrations
    if psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -f "$run_all_up" >/dev/null 2>&1; then
        print_success "Database migrations applied successfully"
        # verify_schema
    else
        print_error "Failed to apply database migrations"
        print_warning "You can run migrations manually:"
        print_warning "  psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f $run_all_up"
        return 1
    fi
}

# Function to verify database schema
# verify_schema() {
#     print_status "Verifying database schema..."
    
#     # Check if key tables exist
#     local tables=("account_types" "customers" "otp_verifications" "customer_sessions" "audit_log")
#     local missing_tables=()
    
#     for table in "${tables[@]}"; do
#         if ! psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -tAc "SELECT 1 FROM information_schema.tables WHERE table_name='$table'" | grep -q 1; then
#             missing_tables+=("$table")
#         fi
#     done
    
#     if [ ${#missing_tables[@]} -eq 0 ]; then
#         print_success "All required tables exist"
#     else
#         print_warning "Missing tables: ${missing_tables[*]}"
#         print_warning "This might indicate incomplete migrations"
#     fi
# }

# Function to cleanup temporary files
cleanup() {
    print_status "Cleaning up temporary files..."
    rm -rf "$TEMP_DIR"
    print_success "Cleanup completed"
}

# Function to show help
show_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --help, -h          Show this help message"
    echo "  --no-cleanup        Don't cleanup temporary files on exit"
    echo ""
    echo "This script runs the Yamata no Orochi application in development mode:"
    echo "  - No Docker required"
    echo "  - Swagger UI enabled at http://localhost:8080/swagger"
    echo "  - Uses local PostgreSQL and Redis (if available)"
    echo "  - Generates temporary JWT keys"
    echo "  - Interactive database migration support"
    echo ""
    echo "Prerequisites:"
    echo "  - Go installed"
    echo "  - PostgreSQL running (optional, will show warnings if not)"
    echo "  - Redis running (optional, will show warnings if not)"
    echo "  - PostgreSQL client tools (psql, createdb)"
    echo ""
    echo "Database Features:"
    echo "  - Automatic database connection checking"
    echo "  - Interactive database creation"
    echo "  - Automatic migration running"
    echo "  - Schema verification"
    echo ""
    echo "Examples:"
    echo "  $0                    # Run with cleanup"
    echo "  $0 --no-cleanup      # Run without cleanup"
    echo ""
}

# Function to start the application
start_application() {
	print_status "Starting Yamata no Orochi application..."
	
	# Generate Swagger documentation first
	print_status "Generating Swagger documentation..."
	if command -v swag >/dev/null 2>&1; then
		if swag init -g main.go -o docs >/dev/null 2>&1; then
			print_success "Swagger documentation generated successfully"
		else
			print_warning "Failed to generate Swagger documentation, continuing anyway..."
		fi
	else
		print_warning "swag command not found, skipping Swagger generation"
		print_warning "Install with: go install github.com/swaggo/swag/cmd/swag@latest"
	fi
	
	# Start the application with air for hot reloading
	print_status "Starting application with hot reloading..."
	if command -v air >/dev/null 2>&1; then
		print_success "Using air for hot reloading"
		air
	else
		print_warning "air not found, using go run"
		print_warning "Install air with: go install github.com/cosmtrek/air@latest"
		go run main.go
	fi
}

# Main function
main() {
    local cleanup_on_exit=true
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --help|-h)
                show_help
                exit 0
                ;;
            --no-cleanup)
                cleanup_on_exit=false
                shift
                ;;
            *)
                print_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
    
    echo "🐍 Yamata no Orochi - Development Mode"
    echo "======================================"
    echo ""
    
    # Set up cleanup trap
    if [ "$cleanup_on_exit" = true ]; then
        trap cleanup EXIT
    fi
    
    # Check prerequisites
    check_prerequisites
    
    # Create temporary directory
    create_temp_dir
    
    # Generate JWT keys
    generate_jwt_keys
    
    # Set environment variables
    set_environment
    
    # Initialize database
    initialize_database
    
    # Build application
    build_application
    
    # Start the application
    start_application
    
    echo ""
    print_success "🎉 Development environment ready!"
    echo ""
    echo "📋 Development Information:"
    echo "  Application: http://localhost:$SERVER_PORT"
    echo "  Swagger UI: http://localhost:$SERVER_PORT/swagger"
    echo "  Health Check: http://localhost:$SERVER_PORT/api/v1/health"
    echo "  Metrics: http://localhost:$METRICS_PORT/metrics"
    echo ""
    echo "🔧 Environment:"
    echo "  APP_ENV: $APP_ENV"
    echo "  Database: $DB_HOST:$DB_PORT/$DB_NAME"
    echo "  JWT Issuer: $JWT_ISSUER"
    echo ""
    echo "🚀 Starting application..."
    echo ""
    
    # Run the application
    # "$TEMP_DIR/main" # This line is now handled by start_application
}

# Run main function with all arguments
main "$@" 