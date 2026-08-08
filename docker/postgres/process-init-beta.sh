#!/bin/bash

# Script to process init.sql for BETA environment
# This creates a processed version that PostgreSQL can understand

set -Eeuo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_status "Processing init.sql for BETA environment..."

# The caller must load .env.beta through scripts/load-yamata-env.sh. Never
# execute dotenv files as shell code here.
DB_NAME=${DB_NAME:?DB_NAME is required}
DB_USER=${DB_USER:?DB_USER is required}
[[ "$DB_NAME" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || {
    print_error "DB_NAME must be a simple PostgreSQL identifier"
    exit 1
}
[[ "$DB_USER" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || {
    print_error "DB_USER must be a simple PostgreSQL identifier"
    exit 1
}

# Create processed init file for beta environment
PROCESSED_INIT="$SCRIPT_DIR/init-beta-processed.sql"
PROCESSED_DB_INIT="$SCRIPT_DIR/init-database-beta-processed.sql"
TEMP_INIT=$(mktemp "$SCRIPT_DIR/.init-beta-processed.sql.XXXXXX")
TEMP_DB_INIT=$(mktemp "$SCRIPT_DIR/.init-database-beta-processed.sql.XXXXXX")
cleanup() {
    rm -f -- "$TEMP_INIT" "$TEMP_DB_INIT"
}
trap cleanup EXIT

print_status "Environment variables:"
echo "  DB_NAME: $DB_NAME"
echo "  DB_USER: $DB_USER"

# Substitute variables in init.sql and create processed version
sed -e "s/\${DB_NAME:-yamata_no_orochi}/$DB_NAME/g" \
    -e "s/\${DB_USER:-yamata_user}/$DB_USER/g" \
    "$SCRIPT_DIR/init.sql" > "$TEMP_INIT"

# Substitute variables in init-database.sql and create processed version
sed -e "s/\${DB_NAME:-yamata_no_orochi}/$DB_NAME/g" \
    -e "s/\${DB_USER:-yamata_user}/$DB_USER/g" \
    "$SCRIPT_DIR/init-database.sql" > "$TEMP_DB_INIT"

chmod 644 "$TEMP_INIT" "$TEMP_DB_INIT"
mv -f -- "$TEMP_INIT" "$PROCESSED_INIT"
mv -f -- "$TEMP_DB_INIT" "$PROCESSED_DB_INIT"

print_success "Created processed init file for BETA: $PROCESSED_INIT"
print_success "Created processed database init file for BETA: $PROCESSED_DB_INIT"
print_status "Ready for Docker Compose beta environment"
