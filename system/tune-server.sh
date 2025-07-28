#!/bin/bash

# Production Server Tuning Script for Debian
# This script optimizes various system parameters for high-load production environments

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

# Check if running on Debian/Ubuntu
check_distro() {
    if ! command -v apt-get >/dev/null 2>&1; then
        print_error "This script is designed for Debian/Ubuntu systems"
        exit 1
    fi
}

# Backup current configuration
backup_config() {
    print_status "Creating backup of current system configuration..."
    
    local backup_dir="/root/server-tuning-backup-$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$backup_dir"
    
    # Backup important system files
    cp /etc/sysctl.conf "$backup_dir/" 2>/dev/null || true
    cp /etc/security/limits.conf "$backup_dir/" 2>/dev/null || true
    cp /etc/systemd/system.conf "$backup_dir/" 2>/dev/null || true
    cp /etc/systemd/user.conf "$backup_dir/" 2>/dev/null || true
    
    print_success "Backup created at: $backup_dir"
}

# Tune file descriptors and limits
tune_file_descriptors() {
    print_status "Tuning file descriptors and limits..."
    
    # Update limits.conf
    cat > /etc/security/limits.conf << EOF
# Production server limits
* soft nofile 65536
* hard nofile 65536
* soft nproc 32768
* hard nproc 32768
root soft nofile 65536
root hard nofile 65536
root soft nproc 32768
root hard nproc 32768

# Nginx specific limits
nginx soft nofile 65536
nginx hard nofile 65536
nginx soft nproc 32768
nginx hard nproc 32768

# PostgreSQL specific limits
postgres soft nofile 65536
postgres hard nofile 65536
postgres soft nproc 32768
postgres hard nproc 32768
EOF
    
    print_success "File descriptor limits updated"
}

# Tune kernel parameters
tune_kernel_parameters() {
    print_status "Tuning kernel parameters..."
    
    # Create sysctl configuration
    cat > /etc/sysctl.conf << EOF
# Network tuning for high load
# TCP/IP stack optimization
net.core.rmem_default = 262144
net.core.rmem_max = 16777216
net.core.wmem_default = 262144
net.core.wmem_max = 16777216
net.core.netdev_max_backlog = 5000
net.core.somaxconn = 65535
net.core.optmem_max = 25165824

# TCP optimization
net.ipv4.tcp_rmem = 4096 87380 16777216
net.ipv4.tcp_wmem = 4096 65536 16777216
net.ipv4.tcp_congestion_control = bbr
net.ipv4.tcp_slow_start_after_idle = 0
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 15
net.ipv4.tcp_keepalive_time = 300
net.ipv4.tcp_keepalive_probes = 3
net.ipv4.tcp_keepalive_intvl = 15
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.tcp_max_tw_buckets = 2000000
net.ipv4.tcp_tw_recycle = 0
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fastopen = 3
net.ipv4.tcp_fastopen_key = 00000000-00000000-00000000-00000000

# UDP optimization
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.udp_rmem_min = 4096
net.ipv4.udp_wmem_min = 4096

# IPv6 optimization
net.ipv6.conf.all.forwarding = 0
net.ipv6.conf.all.accept_ra = 1
net.ipv6.conf.all.accept_redirects = 0
net.ipv6.conf.all.autoconf = 1
net.ipv6.conf.all.dad_transmits = 1
net.ipv6.conf.all.max_addresses = 16

# Memory management
vm.swappiness = 10
vm.dirty_ratio = 15
vm.dirty_background_ratio = 5
vm.overcommit_memory = 1
vm.overcommit_ratio = 50
vm.max_map_count = 262144

# File system optimization
fs.file-max = 2097152
fs.inotify.max_user_watches = 524288
fs.inotify.max_user_instances = 512

# Security settings
net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.default.rp_filter = 1
net.ipv4.conf.all.accept_source_route = 0
net.ipv4.conf.default.accept_source_route = 0
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.default.accept_redirects = 0
net.ipv4.conf.all.secure_redirects = 0
net.ipv4.conf.default.secure_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.default.send_redirects = 0
net.ipv4.conf.all.log_martians = 1
net.ipv4.conf.default.log_martians = 1

# Ignore ICMP redirects
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.default.accept_redirects = 0
net.ipv4.conf.all.secure_redirects = 0
net.ipv4.conf.default.secure_redirects = 0

# Ignore bogus ICMP responses
net.ipv4.icmp_echo_ignore_broadcasts = 1
net.ipv4.icmp_ignore_bogus_error_responses = 1

# Enable bad error message protection
net.ipv4.icmp_errors_use_inbound_ifaddr = 1

# Enable reverse path filtering
net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.default.rp_filter = 1

# Disable IPv6 if not needed (uncomment if IPv6 is not required)
# net.ipv6.conf.all.disable_ipv6 = 1
# net.ipv6.conf.default.disable_ipv6 = 1
# net.ipv6.conf.lo.disable_ipv6 = 1
EOF
    
    # Apply sysctl changes
    sysctl -p
    
    print_success "Kernel parameters tuned"
}

# Tune systemd settings
tune_systemd() {
    print_status "Tuning systemd settings..."
    
    # Update system.conf
    cat > /etc/systemd/system.conf << EOF
# Production systemd configuration
[Manager]
DefaultLimitNOFILE=65536
DefaultLimitNPROC=32768
DefaultTimeoutStartSec=30s
DefaultTimeoutStopSec=30s
DefaultRestartSec=100ms
DefaultStartLimitInterval=10s
DefaultStartLimitBurst=5
EOF
    
    # Update user.conf
    cat > /etc/systemd/user.conf << EOF
# Production systemd user configuration
[Manager]
DefaultLimitNOFILE=65536
DefaultLimitNPROC=32768
DefaultTimeoutStartSec=30s
DefaultTimeoutStopSec=30s
DefaultRestartSec=100ms
DefaultStartLimitInterval=10s
DefaultStartLimitBurst=5
EOF
    
    # Reload systemd
    systemctl daemon-reload
    
    print_success "Systemd settings tuned"
}

# Tune disk I/O scheduler
tune_disk_io() {
    print_status "Tuning disk I/O scheduler..."
    
    # Get list of block devices
    local devices=$(lsblk -d -n -o NAME | grep -v loop)
    
    for device in $devices; do
        if [ -e "/sys/block/$device/queue/scheduler" ]; then
            # Set scheduler to deadline for better performance
            echo deadline > "/sys/block/$device/queue/scheduler" 2>/dev/null || true
            
            # Optimize read-ahead
            echo 4096 > "/sys/block/$device/queue/read_ahead_kb" 2>/dev/null || true
            
            # Optimize I/O queue depth
            echo 1024 > "/sys/block/$device/queue/nr_requests" 2>/dev/null || true
        fi
    done
    
    print_success "Disk I/O scheduler tuned"
}

# Tune network interface
tune_network() {
    print_status "Tuning network interfaces..."
    
    # Get list of network interfaces
    local interfaces=$(ip link show | grep -E "^[0-9]+:" | cut -d: -f2 | tr -d ' ')
    
    for interface in $interfaces; do
        if [ "$interface" != "lo" ]; then
            # Optimize ring buffer sizes
            ethtool -G "$interface" rx 4096 tx 4096 2>/dev/null || true
            
            # Enable TCP segmentation offload
            ethtool -K "$interface" tso on 2>/dev/null || true
            ethtool -K "$interface" gso on 2>/dev/null || true
            ethtool -K "$interface" gro on 2>/dev/null || true
            
            # Enable large receive offload
            ethtool -K "$interface" lro on 2>/dev/null || true
        fi
    done
    
    print_success "Network interfaces tuned"
}

# Install performance monitoring tools
install_monitoring_tools() {
    print_status "Installing performance monitoring tools..."
    
    # Update package list
    apt update
    
    # Install monitoring tools
    apt install -y htop iotop nethogs iftop nload sysstat dstat \
                   net-tools ethtool iperf3 stress-ng \
                   python3-pip python3-psutil
    
    # Install additional Python tools
    pip3 install --break-system-packages psutil netifaces
    
    print_success "Monitoring tools installed"
}

# Create performance monitoring script
create_monitoring_script() {
    print_status "Creating performance monitoring script..."
    
    cat > /usr/local/bin/server-monitor.sh << 'EOF'
#!/bin/bash

# Server Performance Monitoring Script

echo "=== System Performance Monitor ==="
echo "Date: $(date)"
echo ""

# CPU Usage
echo "=== CPU Usage ==="
top -bn1 | grep "Cpu(s)" | awk '{print $2}' | cut -d'%' -f1
echo ""

# Memory Usage
echo "=== Memory Usage ==="
free -h
echo ""

# Disk Usage
echo "=== Disk Usage ==="
df -h
echo ""

# Network Usage
echo "=== Network Usage ==="
ss -tuln | wc -l
echo "Active connections: $(ss -tuln | wc -l)"
echo ""

# Load Average
echo "=== Load Average ==="
uptime
echo ""

# File Descriptors
echo "=== File Descriptors ==="
echo "Open files: $(lsof | wc -l)"
echo "Max open files: $(ulimit -n)"
echo ""

# Network Statistics
echo "=== Network Statistics ==="
netstat -i
echo ""

# Process Count
echo "=== Process Count ==="
echo "Total processes: $(ps aux | wc -l)"
echo ""

# System Uptime
echo "=== System Uptime ==="
uptime -p
echo ""
EOF
    
    chmod +x /usr/local/bin/server-monitor.sh
    
    print_success "Monitoring script created at /usr/local/bin/server-monitor.sh"
}

# Create systemd service for automatic tuning
create_tuning_service() {
    print_status "Creating systemd service for automatic tuning..."
    
    cat > /etc/systemd/system/server-tuning.service << EOF
[Unit]
Description=Server Performance Tuning
After=network.target

[Service]
Type=oneshot
ExecStart=/bin/bash -c 'echo deadline > /sys/block/sda/queue/scheduler 2>/dev/null || true'
ExecStart=/bin/bash -c 'echo 4096 > /sys/block/sda/queue/read_ahead_kb 2>/dev/null || true'
ExecStart=/bin/bash -c 'sysctl -p'
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF
    
    # Enable the service
    systemctl daemon-reload
    systemctl enable server-tuning.service
    
    print_success "Tuning service created and enabled"
}

# Show current system information
show_system_info() {
    print_status "Current System Information:"
    echo ""
    
    echo "=== System Information ==="
    echo "OS: $(lsb_release -d | cut -f2)"
    echo "Kernel: $(uname -r)"
    echo "Architecture: $(uname -m)"
    echo "CPU Cores: $(nproc)"
    echo "Total Memory: $(free -h | grep Mem | awk '{print $2}')"
    echo ""
    
    echo "=== Current Limits ==="
    echo "Max open files: $(ulimit -n)"
    echo "Max processes: $(ulimit -u)"
    echo ""
    
    echo "=== Network Interfaces ==="
    ip addr show | grep -E "^[0-9]+:" | cut -d: -f2 | tr -d ' '
    echo ""
    
    echo "=== Disk Information ==="
    lsblk
    echo ""
}

# Show tuning recommendations
show_recommendations() {
    print_status "Tuning Recommendations:"
    echo ""
    echo "1. Monitor system performance after tuning:"
    echo "   /usr/local/bin/server-monitor.sh"
    echo ""
    echo "2. Check system logs for any issues:"
    echo "   journalctl -f"
    echo ""
    echo "3. Monitor network performance:"
    echo "   iperf3 -s  # On this server"
    echo "   iperf3 -c <server_ip>  # From client"
    echo ""
    echo "4. Test disk I/O performance:"
    echo "   dd if=/dev/zero of=/tmp/test bs=1M count=1000"
    echo ""
    echo "5. Monitor system resources:"
    echo "   htop"
    echo "   iotop"
    echo "   nethogs"
    echo ""
    echo "6. Check if tuning is applied:"
    echo "   sysctl -a | grep -E '(net\.|vm\.|fs\.)'"
    echo ""
}

# Main function
main() {
    echo "🚀 Production Server Tuning Script"
    echo "=================================="
    echo ""
    
    # Check prerequisites
    check_root
    check_distro
    
    # Show current system info
    show_system_info
    
    # Create backup
    backup_config
    
    # Apply tuning
    tune_file_descriptors
    tune_kernel_parameters
    tune_systemd
    tune_disk_io
    tune_network
    install_monitoring_tools
    create_monitoring_script
    create_tuning_service
    
    echo ""
    print_success "🎉 Server tuning completed successfully!"
    echo ""
    
    # Show recommendations
    show_recommendations
    
    print_warning "A reboot is recommended to apply all changes:"
    echo "  sudo reboot"
    echo ""
}

# Run main function
main "$@" 