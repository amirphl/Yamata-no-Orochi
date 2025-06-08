#!/bin/bash

# Script to process init.sql for BETA environment
# This creates a processed version that PostgreSQL can understand

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"

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

print_status "Processing init.sql for BETA environment..."

# Source environment variables from .env.beta
if [ -f "$PROJECT_ROOT/.env.beta" ]; then
    print_status "Loading environment from .env.beta"
    source "$PROJECT_ROOT/.env.beta"
else
    print_warning "No .env.beta file found, using defaults"
fi

# Set defaults for required variables
DB_NAME=${DB_NAME:-yamata_no_orochi}
DB_USER=${DB_USER:-yamata_user}

# Create processed init file for beta environment
PROCESSED_INIT="$SCRIPT_DIR/init-beta-processed.sql"
PROCESSED_DB_INIT="$SCRIPT_DIR/init-database-beta-processed.sql"

print_status "Environment variables:"
echo "  DB_NAME: $DB_NAME"
echo "  DB_USER: $DB_USER"

# Substitute variables in init.sql and create processed version
sed -e "s/\${DB_NAME:-yamata_no_orochi}/$DB_NAME/g" \
    -e "s/\${DB_USER:-yamata_user}/$DB_USER/g" \
    "$SCRIPT_DIR/init.sql" > "$PROCESSED_INIT"

# Substitute variables in init-database.sql and create processed version
sed -e "s/\${DB_NAME:-yamata_no_orochi}/$DB_NAME/g" \
    -e "s/\${DB_USER:-yamata_user}/$DB_USER/g" \
    "$SCRIPT_DIR/init-database.sql" > "$PROCESSED_DB_INIT"

print_success "Created processed init file for BETA: $PROCESSED_INIT"
print_success "Created processed database init file for BETA: $PROCESSED_DB_INIT"
print_status "Ready for Docker Compose beta environment" 