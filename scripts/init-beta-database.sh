#!/bin/bash

# Database Initialization Script for Yamata no Orochi Beta
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

# Load environment variables from .env.beta file if it exists
if [ -f "$PROJECT_ROOT/.env.beta" ]; then
    print_status "Loading environment variables from .env.beta file..."
    # Use set -a to automatically export variables, then source the file
    set -a
    source "$PROJECT_ROOT/.env.beta"
    set +a
fi

# Function to get database configuration from environment variables
get_db_config() {
    # Database configuration with defaults
    DB_HOST="172.30.0.10"
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
    if ! docker ps --format "table {{.Names}}" | grep -q "yamata-postgres-beta"; then
        print_error "PostgreSQL container is not running. Please start the services first:"
        echo "  docker-compose -f docker-compose.beta.yml up -d postgres-beta"
        exit 1
    fi
    
    # Wait for PostgreSQL to be ready
    print_status "Waiting for PostgreSQL to be ready..."
    local max_attempts=30
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        if docker exec yamata-postgres-beta pg_isready -U "$DB_USER" -d "$DB_NAME" >/dev/null 2>&1; then
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
    if docker exec yamata-postgres-beta psql -U "$DB_USER" -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$DB_NAME'" | grep -q 1; then
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
    
    if docker exec yamata-postgres-beta createdb -U "$DB_USER" "$DB_NAME" 2>/dev/null; then
        print_success "Database '$DB_NAME' created successfully"
        return 0
    else
        print_warning "Database '$DB_NAME' might already exist, already in use or creation failed"
        return 1
    fi
}

# Function to apply migrations
apply_migrations() {
    print_status "Applying database migrations..."
    
    # Migration tracking file
    local tracking_file="$PROJECT_ROOT/.migration_tracker_beta"
    local last_migration=""
    
    # Read last applied migration if tracking file exists
    if [ -f "$tracking_file" ]; then
        last_migration=$(cat "$tracking_file" 2>/dev/null || echo "")
        if [ -n "$last_migration" ]; then
            print_status "Last applied migration: $last_migration"
            print_status "Will resume from the next migration after this one"
        fi
    fi
    
    # Get all migration files
    local migration_dir="$PROJECT_ROOT/migrations"
    local migration_files=()
    
    # Find all .sql files that are not down migrations and not run_all_up.sql
    while IFS= read -r -d '' file; do
        local filename=$(basename "$file")
        # Skip down migrations and run_all_up.sql
        if [[ "$filename" != *"_down.sql" && "$filename" != "run_all_up.sql" ]]; then
            migration_files+=("$file")
        fi
    done < <(find "$migration_dir" -name "*.sql" -type f -print0 | sort -z)
    
    if [ ${#migration_files[@]} -eq 0 ]; then
        print_warning "No migration files found in $migration_dir"
        return 0
    fi
    
    print_status "Found ${#migration_files[@]} migration files (excluding run_all_up.sql)"
    
    # Filter migrations to apply only those after the last applied one
    local migrations_to_apply=()
    local found_last=false
    
    if [ -z "$last_migration" ]; then
        # First run, apply all migrations
        migrations_to_apply=("${migration_files[@]}")
        print_status "First run - will apply all migrations"
    else
        # Find migrations after the last applied one
        for file in "${migration_files[@]}"; do
            local filename=$(basename "$file")
            if [ "$found_last" = true ]; then
                migrations_to_apply+=("$file")
            elif [ "$filename" = "$last_migration" ]; then
                found_last=true
                print_status "Found last applied migration: $filename"
            fi
        done
        
        if [ ${#migrations_to_apply[@]} -eq 0 ]; then
            print_success "All migrations are already applied (last: $last_migration)"
            return 0
        fi
        
        print_status "Found ${#migrations_to_apply[@]} new migrations to apply"
    fi
    
    # Apply each migration
    for file in "${migrations_to_apply[@]}"; do
        local filename=$(basename "$file")
        print_status "Applying migration: $filename"
        
        if docker exec -i -e PGPASSWORD="$DB_PASSWORD" yamata-postgres-beta psql -v ON_ERROR_STOP=1 -U "$DB_USER" -d "$DB_NAME" < "$file"; then
            print_success "Migration '$filename' applied successfully"
            
            # Update tracking file with the last applied migration
            echo "$filename" > "$tracking_file"
            print_status "Updated migration tracker: $filename"
        else
            print_error "Failed to apply migration '$filename'"
            return 1
        fi
    done
    
    print_success "All pending migrations applied successfully"
    return 0
}

# Function to verify database setup
verify_database_setup() {
    print_status "Verifying database setup..."
    
    # Check if tables exist
    local tables=("account_types" "customers")
    local missing_tables=()
    
    for table in "${tables[@]}"; do
        if docker exec yamata-postgres-beta psql -U "$DB_USER" -d "$DB_NAME" -tAc "SELECT 1 FROM information_schema.tables WHERE table_name='$table'" | grep -q 1; then
            print_success "Table '$table' exists"
        else
            print_warning "Table '$table' is missing"
            missing_tables+=("$table")
        fi
    done
    
    if [ ${#missing_tables[@]} -gt 0 ]; then
        print_warning "Missing tables: ${missing_tables[*]}"
        print_warning "This might indicate incomplete migration application"
    else
        print_success "All expected tables exist"
    fi
    
    # Check if audit_action_enum exists
    if docker exec yamata-postgres-beta psql -U "$DB_USER" -d "$DB_NAME" -tAc "SELECT 1 FROM pg_type WHERE typname='audit_action_enum'" | grep -q 1; then
        print_success "audit_action_enum type exists"
    else
        print_warning "audit_action_enum type is missing"
    fi
}

# Function to show database information
show_database_info() {
    print_status "Database Information:"
    echo "  Host: $DB_HOST"
    echo "  Port: $DB_PORT"
    echo "  Database: $DB_NAME"
    echo "  User: $DB_USER"
    echo ""
    
    # Show table count
    local table_count=$(docker exec yamata-postgres-beta psql -U "$DB_USER" -d "$DB_NAME" -tAc "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'")
    echo "  Tables: $table_count"
    
    # Show record counts for main tables
    local tables=("account_types" "customers" "otp_verifications" "customer_sessions" "audit_log")
    for table in "${tables[@]}"; do
        if docker exec yamata-postgres-beta psql -U "$DB_USER" -d "$DB_NAME" -tAc "SELECT COUNT(*) FROM $table" >/dev/null 2>&1; then
            local count=$(docker exec yamata-postgres-beta psql -U "$DB_USER" -d "$DB_NAME" -tAc "SELECT COUNT(*) FROM $table")
            echo "  $table: $count records"
        fi
    done
}

# Function to show migration status
show_migration_status() {
    local tracking_file="$PROJECT_ROOT/.migration_tracker_beta"
    local migration_dir="$PROJECT_ROOT/migrations"
    
    print_status "Migration Status:"
    
    if [ -f "$tracking_file" ]; then
        local last_migration=$(cat "$tracking_file" 2>/dev/null || echo "")
        if [ -n "$last_migration" ]; then
            echo "  Last applied migration: $last_migration"
        else
            echo "  No migrations have been applied yet"
        fi
    else
        echo "  No migration tracking file found (first run)"
    fi
    
    # Count total available migrations
    local total_migrations=0
    local applied_migrations=0
    
    while IFS= read -r -d '' file; do
        local filename=$(basename "$file")
        if [[ "$filename" != *"_down.sql" && "$filename" != "run_all_up.sql" ]]; then
            total_migrations=$((total_migrations + 1))
            
            # Check if this migration is already applied
            if [ -f "$tracking_file" ]; then
                local last_applied=$(cat "$tracking_file" 2>/dev/null || echo "")
                if [ "$filename" = "$last_applied" ] || [ "$filename" \< "$last_applied" ]; then
                    applied_migrations=$((applied_migrations + 1))
                fi
            fi
        fi
    done < <(find "$migration_dir" -name "*.sql" -type f -print0 | sort -z)
    
    echo "  Total migrations available: $total_migrations"
    echo "  Migrations already applied: $applied_migrations"
    echo "  Pending migrations: $((total_migrations - applied_migrations))"
    echo ""
}

# Function to reset migration tracker
reset_migration_tracker() {
    local tracking_file="$PROJECT_ROOT/.migration_tracker_beta"
    if [ -f "$tracking_file" ]; then
        rm "$tracking_file"
        print_status "Migration tracker reset"
    fi
}

# Main function
main() {
    echo "🗄️  Yamata no Orochi - Beta Database Initialization"
    echo "=================================================="
    echo ""
    
    # Get database configuration
    get_db_config
    
    # Check if PostgreSQL container is running
    check_postgres_container
    
    # Check if database exists
    if check_database_exists; then
        print_warning "Database '$DB_NAME' already exists"
        read -p "Do you want to reinitialize the database? This will drop and recreate it. (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            print_status "Dropping existing database..."
            docker exec yamata-postgres-beta dropdb -U "$DB_USER" "$DB_NAME" 2>/dev/null || true
            create_database || true
            # Reset migration tracker when reinitializing database
            reset_migration_tracker
        else
            print_status "Skipping database creation"
        fi
    else
        create_database || true
    fi
    
    # Show migration status before asking for confirmation
    show_migration_status
    
    # Ask if user wants to reset migration tracker
    if [ -f "$PROJECT_ROOT/.migration_tracker_beta" ]; then
        read -p "Do you want to reset the migration tracker and start fresh? [y/N]: " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            reset_migration_tracker
            print_status "Migration tracker reset. Will apply all migrations from the beginning."
        fi
    fi
    
    # Apply migrations
    print_status "Ready to apply database migrations."
    read -p "Do you want to proceed with applying migrations? [y/N]: " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        apply_migrations
    else
        print_status "Migration step skipped."
    fi
    
    # Verify database setup
    verify_database_setup
    
    # Show database information
    show_database_info
    
    print_success "🎉 Beta database initialization completed successfully!"
    echo ""
    echo "📋 Next Steps:"
    echo "  1. Your application should now be able to connect to the database"
    echo "  2. Check the application logs for any connection issues"
    echo "  3. Test the API endpoints to ensure everything is working"
    echo ""
}

# Run main function
main "$@" 