#!/bin/bash

# System Nginx Installation Script
# This script installs and configures nginx for production use

set -e

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

# Check if running as root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        print_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

# Install nginx
install_nginx() {
    print_status "Installing nginx..."
    
    # Update package list
    apt update
    
    # Install nginx
    apt install -y nginx
    
    print_success "Nginx installed successfully"
}

# Backup existing configuration
backup_config() {
    if [ -f /etc/nginx/nginx.conf ]; then
        print_status "Backing up existing nginx configuration..."
        cp /etc/nginx/nginx.conf /etc/nginx/nginx.conf.backup.$(date +%Y%m%d_%H%M%S)
        print_success "Backup created"
    fi
}

# Install new configuration
install_config() {
    print_status "Installing new nginx configuration..."
    
    # Copy the configuration file
    cp nginx.conf /etc/nginx/nginx.conf
    
    # Set proper permissions
    chmod 644 /etc/nginx/nginx.conf
    chown root:root /etc/nginx/nginx.conf
    
    print_success "Configuration installed"
}

# Test nginx configuration
test_config() {
    print_status "Testing nginx configuration..."
    
    if nginx -t; then
        print_success "Configuration test passed"
        return 0
    else
        print_error "Configuration test failed"
        return 1
    fi
}

# Start/restart nginx
restart_nginx() {
    print_status "Restarting nginx..."
    
    # Stop nginx if running
    systemctl stop nginx 2>/dev/null || true
    
    # Start nginx
    systemctl start nginx
    
    # Enable nginx to start on boot
    systemctl enable nginx
    
    print_success "Nginx started successfully"
}

# Check SSL certificates
check_ssl() {
    print_status "Checking SSL certificates..."
    
    local cert_dir="/etc/letsencrypt/live/beta.jaazebeh.ir"
    
    if [ ! -d "$cert_dir" ]; then
        print_error "SSL certificates not found at $cert_dir"
        print_warning "Please ensure Let's Encrypt certificates are installed"
        return 1
    fi
    
    if [ ! -f "$cert_dir/fullchain.pem" ] || [ ! -f "$cert_dir/privkey.pem" ]; then
        print_error "SSL certificate files not found"
        print_warning "Please ensure Let's Encrypt certificates are properly installed"
        return 1
    fi
    
    print_success "SSL certificates found"
    return 0
}

# Show status
show_status() {
    print_status "Nginx Status:"
    systemctl status nginx --no-pager -l
    
    echo ""
    print_status "Configuration Test:"
    nginx -t
    
    echo ""
    print_status "Listening Ports:"
    ss -tlnp | grep nginx || echo "No nginx processes found"
}

# Main function
main() {
    echo "🌐 System Nginx Installation"
    echo "============================"
    echo ""
    
    # Check if running as root
    check_root
    
    # Install nginx
    install_nginx
    
    # Backup existing configuration
    backup_config
    
    # Install new configuration
    install_config
    
    # Test configuration
    if test_config; then
        # Check SSL certificates
        if check_ssl; then
            # Restart nginx
            restart_nginx
            
            echo ""
            print_success "🎉 Nginx installation completed successfully!"
            echo ""
            echo "📋 Configuration Summary:"
            echo "  - Domain: beta.jaazebeh.ir"
            echo "  - SSL: Let's Encrypt certificates"
            echo "  - API Proxy: 172.30.0.30:443"
            echo "  - Frontend Proxy: 172.90.0.21:80"
            echo "  - Rate Limiting: Enabled"
            echo "  - Security Headers: Enabled"
            echo ""
            
            show_status
        else
            print_warning "SSL certificates not found. Please install them before starting nginx."
            print_status "You can start nginx manually after installing certificates:"
            echo "  sudo systemctl start nginx"
        fi
    else
        print_error "Configuration test failed. Please check the configuration file."
        exit 1
    fi
}

# Run main function
main "$@" 