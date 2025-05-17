#!/bin/bash

# Redis startup script for Yamata no Orochi
# This script configures redis with environment variables and starts redis

set -e

echo "Starting Redis configuration..."

# Create redis configuration with password if provided
if [ -n "$REDIS_PASSWORD" ]; then
    echo "Configuring Redis with password..."
    # Create a temporary config file with password
    cp /usr/local/etc/redis/redis.conf /tmp/redis.conf
    echo "requirepass $REDIS_PASSWORD" >> /tmp/redis.conf
    echo "rename-command FLUSHDB \"\"" >> /tmp/redis.conf
    echo "rename-command FLUSHALL \"\"" >> /tmp/redis.conf
    echo "rename-command DEBUG \"\"" >> /tmp/redis.conf
    echo "rename-command CONFIG \"\"" >> /tmp/redis.conf
    REDIS_CONFIG="/tmp/redis.conf"
else
    echo "Warning: No Redis password provided, using default configuration"
    REDIS_CONFIG="/usr/local/etc/redis/redis.conf"
fi

# Start redis with the configuration
echo "Starting Redis..."
exec redis-server $REDIS_CONFIG 