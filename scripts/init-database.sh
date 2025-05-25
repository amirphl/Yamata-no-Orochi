#!/bin/bash

# Database Initialization Script for Yamata no Orochi
# This script creates the database and applies all migrations

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

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

# Load environment variables from .env file if it exists
if [ -f "$PROJECT_ROOT/.env" ]; then
    print_status "Loading environment variables from .env file..."
    # Use set -a to automatically export variables, then source the file
    set -a
    source "$PROJECT_ROOT/.env"
    set +a
fi

# Function to get database configuration from environment variables
get_db_config() {
    # Database configuration with defaults
    DB_HOST=${DB_HOST:-localhost}
    DB_PORT=${DB_PORT:-5432}
    DB_NAME=${DB_NAME:-yamata_no_orochi}
    DB_USER=${DB_USER:-yamata_user}
    DB_PASSWORD=${DB_PASSWORD:-}
    
    # Export for use in functions
    export DB_HOST DB_PORT DB_NAME DB_USER DB_PASSWORD
    
    print_status "Database Configuration:"
    echo "  Host: $DB_HOST"
    echo "  Port: $DB_PORT"
    echo "  Database: $DB_NAME"
    echo "  User: $DB_USER"
    echo ""
}

# Function to check if PostgreSQL container is running
check_postgres_container() {
    if ! docker ps --format "table {{.Names}}" | grep -q "yamata-postgres"; then
        print_error "PostgreSQL container is not running. Please start the services first:"
        echo "  docker-compose -f docker-compose.production.yml up -d postgres"
        exit 1
    fi
    
    # Wait for PostgreSQL to be ready
    print_status "Waiting for PostgreSQL to be ready..."
    local max_attempts=30
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        if docker exec yamata-postgres pg_isready -U "$DB_USER" -d "$DB_NAME" >/dev/null 2>&1; then
            print_success "PostgreSQL is ready!"
            return 0
        fi
        
        echo "Attempt $attempt/$max_attempts - Waiting for PostgreSQL..."
        sleep 2
        attempt=$((attempt + 1))
    done
    
    print_error "PostgreSQL failed to start within expected time"
    return 1
}

# Function to check if database exists
check_database_exists() {
    if docker exec yamata-postgres psql -U "$DB_USER" -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$DB_NAME'" | grep -q 1; then
        print_status "Database '$DB_NAME' already exists"
        return 0
    else
        print_status "Database '$DB_NAME' does not exist"
        return 1
    fi
}

# Function to create database
create_database() {
    print_status "Creating database '$DB_NAME'..."
    
    if docker exec yamata-postgres createdb -U "$DB_USER" "$DB_NAME" 2>/dev/null; then
        print_success "Database '$DB_NAME' created successfully"
    else
        print_warning "Database '$DB_NAME' might already exist or creation failed"
    fi
}

# Function to apply migrations
apply_migrations() {
    print_status "Applying database migrations..."
    
    # Copy migrations to container
    docker exec yamata-postgres mkdir -p /tmp/migrations
    docker cp "$PROJECT_ROOT/migrations" yamata-postgres:/tmp/
    
    # Apply migrations
    if docker exec yamata-postgres psql -U "$DB_USER" -d "$DB_NAME" -f /tmp/migrations/run_all_up.sql; then
        print_success "All migrations applied successfully"
    else
        print_error "Failed to apply migrations"
        exit 1
    fi
    
    # Clean up
    docker exec yamata-postgres rm -rf /tmp/migrations
}

# Function to verify database schema
verify_schema() {
    print_status "Verifying database schema..."
    
    # Check if key tables exist
    local tables=("account_types" "customers" "otp_verifications" "customer_sessions" "audit_log")
    local missing_tables=()
    
    for table in "${tables[@]}"; do
        if ! docker exec yamata-postgres psql -U "$DB_USER" -d "$DB_NAME" -tAc "SELECT 1 FROM information_schema.tables WHERE table_name='$table'" | grep -q 1; then
            missing_tables+=("$table")
        fi
    done
    
    if [ ${#missing_tables[@]} -eq 0 ]; then
        print_success "All required tables exist"
    else
        print_error "Missing tables: ${missing_tables[*]}"
        exit 1
    fi
}

# Main function
main() {
    echo "🗄️  Database Initialization for Yamata no Orochi"
    echo "================================================"
    echo ""
    
    # Load database configuration
    get_db_config
    
    # Check if PostgreSQL container is running
    check_postgres_container
    
    # Check if database exists
    if check_database_exists; then
        print_warning "Database already exists. Do you want to reapply migrations? (y/N)"
        read -r response
        if [[ ! "$response" =~ ^[Yy]$ ]]; then
            print_status "Skipping database initialization"
            exit 0
        fi
    else
        # Create database
        create_database
    fi
    
    # Apply migrations
    apply_migrations
    
    # Verify schema
    verify_schema
    
    print_success "🎉 Database initialization completed successfully!"
    echo ""
    echo "📋 Database Information:"
    echo "  Database: $DB_NAME"
    echo "  User: $DB_USER"
    echo "  Host: $DB_HOST"
    echo "  Port: $DB_PORT"
    echo ""
    echo "🔍 You can connect to the database using:"
    echo "  docker exec -it yamata-postgres psql -U $DB_USER -d $DB_NAME"
    echo ""
}

# Run main function
main "$@" 