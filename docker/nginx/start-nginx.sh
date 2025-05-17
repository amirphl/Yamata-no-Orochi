#!/bin/bash

# Nginx startup script for Yamata no Orochi
# This script generates nginx configuration from environment variables and starts nginx

set -e

echo "Starting Nginx configuration generation..."

# Generate nginx configuration from environment variables
if [ -f /etc/nginx/generate-config.sh ]; then
    /etc/nginx/generate-config.sh
else
    echo "Warning: generate-config.sh not found, using static configuration"
fi

# Test nginx configuration
echo "Testing nginx configuration..."
nginx -t

# Start nginx
echo "Starting nginx..."
exec nginx -g "daemon off;" 