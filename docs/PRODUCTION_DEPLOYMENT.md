# 🚀 Production Deployment Guide - Docker Compose

This guide covers deploying Yamata no Orochi to production using Docker Compose with Nginx reverse proxy.

## 📋 Prerequisites

### Required Software
- **Docker**: Version 20.10+ 
- **Docker Compose**: Version 2.0+
- **Git**: For source code management
- **OpenSSL**: For SSL certificate generation
- **Domain**: A registered domain pointing to your server

### Server Requirements
- **RAM**: Minimum 4GB, Recommended 8GB+
- **CPU**: Minimum 2 cores, Recommended 4+ cores  
- **Storage**: Minimum 20GB free space
- **OS**: Ubuntu 20.04+ / CentOS 8+ / Debian 11+

---

## 🏗️ Architecture Overview

```
Internet → Nginx (443/80) → Go App (8080) → PostgreSQL (5432)
                         → Redis (6379)
                         → Prometheus (9091)
                         → Grafana (3000)
```

### Services Included:
- **Nginx**: Reverse proxy with SSL termination
- **Go Application**: Main API service
- **PostgreSQL**: Primary database with optimized configuration
- **Redis**: Caching and session storage
- **Prometheus**: Metrics collection
- **Grafana**: Monitoring dashboards

---

## 🚀 Quick Start

### 1. Clone and Setup
```bash
git clone https://github.com/your-username/yamata-no-orochi.git
cd yamata-no-orochi
```

### 2. Configure Domains and Environment

**Option A: Automated Domain Configuration (Recommended)**
```bash
# Interactive domain setup
./scripts/configure-domains.sh

# Or command line setup
./scripts/configure-domains.sh production example.com api.example.com monitoring.example.com admin@example.com
```

**Option B: Manual Configuration**
```bash
# Copy environment template
cp env.production.template .env.production

# Edit with your actual values
nano .env.production
```

**Required Variables:**
```bash
# Domain Configuration - CUSTOMIZE FOR YOUR DEPLOYMENT
DOMAIN=your-domain.com
API_DOMAIN=api.your-domain.com  
MONITORING_DOMAIN=monitoring.your-domain.com
CERTBOT_EMAIL=admin@your-domain.com

# Security Configuration
DB_PASSWORD=your_secure_database_password
JWT_SECRET_KEY=your_256_bit_jwt_secret_key_here
SMS_API_KEY=your_sms_provider_api_key
EMAIL_PASSWORD=your_email_service_password
```

### 3. Deploy
```bash
# Run the automated deployment script
./scripts/deploy-docker-compose.sh
```

That's it! 🎉 Your application will be available at:
- **Main Site**: https://your-domain.com
- **API**: https://api.your-domain.com
- **Health Check**: https://your-domain.com/health

---

## 🔧 Manual Deployment Steps

If you prefer to deploy manually or understand each step:

### Step 1: Environment Setup
```bash
# Create .env.production file
cat > .env.production <<EOF
DB_NAME=yamata_no_orochi
DB_USER=yamata_user
DB_PASSWORD=your_secure_password_here
JWT_SECRET_KEY=your_256_bit_secret_key_here
# ... other variables
EOF

# Secure the environment file
chmod 600 .env.production
```

### Step 2: SSL Certificates
```bash
# Option A: Let's Encrypt (Recommended)
./scripts/deploy-docker-compose.sh --ssl-only

# Option B: Self-signed (Development/Testing)
mkdir -p docker/nginx/ssl
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout docker/nginx/ssl/privkey.pem \
    -out docker/nginx/ssl/fullchain.pem \
    -subj "/CN=your-domain.com"
```

### Step 3: Deploy Services
```bash
# Build and start all services
docker-compose -f docker-compose.production.yml up -d

# Check service status
docker-compose -f docker-compose.production.yml ps
```

### Step 4: Initialize Database
```bash
# Run migrations (automatic on first start)
docker-compose -f docker-compose.production.yml logs postgres

# Verify database connection
docker-compose -f docker-compose.production.yml exec postgres \
    psql -U yamata_user -d yamata_no_orochi -c "SELECT version();"
```

---

## 🔒 Security Configuration

### Firewall Setup
```bash
# Ubuntu/Debian
sudo ufw allow 22/tcp   # SSH
sudo ufw allow 80/tcp   # HTTP
sudo ufw allow 443/tcp  # HTTPS
sudo ufw enable

# CentOS/RHEL
sudo firewall-cmd --permanent --add-service=ssh
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --reload
```

### Docker Security
- All containers run as non-root users
- Read-only root filesystems where possible
- Security-hardened configurations
- Resource limits enforced

### Network Security
- Private Docker network (172.20.0.0/24)
- Services only accessible via Nginx
- Rate limiting configured at multiple levels
- Security headers enforced

---

## 📊 Monitoring & Logging

### Grafana Dashboard
```bash
# Access Grafana (Internal only)
http://localhost:3000
# Default: admin / admin (change on first login)
```

### Prometheus Metrics
```bash
# Access Prometheus (Internal only)
http://localhost:9091
```

### Log Management
```bash
# View application logs
docker-compose -f docker-compose.production.yml logs -f app

# View Nginx access logs
docker-compose -f docker-compose.production.yml logs -f nginx

# View all service logs
docker-compose -f docker-compose.production.yml logs -f
```

### Log Rotation
Automatic log rotation is configured via logrotate:
- Daily rotation
- 7 days retention
- Compression enabled

---

## 🔄 Management Commands

### Service Management
```bash
# Start services
docker-compose -f docker-compose.production.yml up -d

# Stop services
docker-compose -f docker-compose.production.yml down

# Restart specific service
docker-compose -f docker-compose.production.yml restart app

# View service status
docker-compose -f docker-compose.production.yml ps

# Scale app service
docker-compose -f docker-compose.production.yml up -d --scale app=3
```

### Updates and Maintenance
```bash
# Update application
git pull origin main
docker-compose -f docker-compose.production.yml build app
docker-compose -f docker-compose.production.yml up -d app

# Update all services
docker-compose -f docker-compose.production.yml pull
docker-compose -f docker-compose.production.yml up -d

# Database backup
docker-compose -f docker-compose.production.yml exec postgres \
    pg_dump -U yamata_user yamata_no_orochi > backup_$(date +%Y%m%d).sql

# Database restore
docker-compose -f docker-compose.production.yml exec -T postgres \
    psql -U yamata_user yamata_no_orochi < backup_20240101.sql
```

---

## 🏥 Health Checks & Troubleshooting

### Health Check Endpoints
```bash
# Application health
curl https://your-domain.com/health

# API documentation
curl https://api.your-domain.com/api/v1/docs

# Nginx status
curl http://localhost/nginx_status
```

### Common Issues

#### 1. SSL Certificate Issues
```bash
# Check certificate validity
openssl x509 -in docker/nginx/ssl/fullchain.pem -text -noout

# Renew Let's Encrypt certificates
./scripts/deploy-docker-compose.sh --ssl-only
```

#### 2. Database Connection Issues
```bash
# Check PostgreSQL logs
docker-compose -f docker-compose.production.yml logs postgres

# Test database connection
docker-compose -f docker-compose.production.yml exec app \
    /usr/local/bin/yamata-no-orochi health
```

#### 3. Application Not Starting
```bash
# Check application logs
docker-compose -f docker-compose.production.yml logs app

# Check environment variables
docker-compose -f docker-compose.production.yml exec app env | grep -E "(DB_|JWT_)"

# Restart application
docker-compose -f docker-compose.production.yml restart app
```

#### 4. High Memory Usage
```bash
# Check container resource usage
docker stats

# Check service memory limits
docker-compose -f docker-compose.production.yml config | grep -A5 -B5 memory
```

---

## 🔧 Performance Tuning

### PostgreSQL Optimization
The included configuration is optimized for production:
- Connection pooling (200 max connections)
- Memory settings (256MB shared buffers)
- WAL configuration for performance
- Query logging for slow queries (>1s)

### Redis Configuration
- Memory limit: 256MB with LRU eviction
- Persistence: AOF + RDB snapshots
- Optimized for caching workloads

### Nginx Performance
- Gzip compression enabled
- Static file caching
- Keep-alive connections
- Worker process optimization

---

## 📈 Scaling Considerations

### Horizontal Scaling
```bash
# Scale application instances
docker-compose -f docker-compose.production.yml up -d --scale app=3
```

### Load Balancing
Nginx is configured with `least_conn` load balancing for multiple app instances.

### Database Scaling
- Read replicas can be added
- Connection pooling configured
- Prepared for external database services

---

## 🔐 Backup Strategy

### Automated Backups
```bash
# Create backup script
cat > backup.sh <<'EOF'
#!/bin/bash
BACKUP_DIR="/backups"
DATE=$(date +%Y%m%d_%H%M%S)

# Database backup
docker-compose -f docker-compose.production.yml exec -T postgres \
    pg_dump -U yamata_user yamata_no_orochi | gzip > "$BACKUP_DIR/db_backup_$DATE.sql.gz"

# Application logs backup
docker-compose -f docker-compose.production.yml logs --no-color > "$BACKUP_DIR/logs_$DATE.log"

# Cleanup old backups (keep 7 days)
find "$BACKUP_DIR" -name "*.gz" -mtime +7 -delete
find "$BACKUP_DIR" -name "*.log" -mtime +7 -delete
EOF

chmod +x backup.sh

# Add to crontab for daily backups
echo "0 2 * * * /path/to/backup.sh" | crontab -
```

---

## 🚦 Migration from Kubernetes

If you later want to migrate to Kubernetes, the Docker Compose setup makes it easier:

1. Container images are already built and tested
2. Environment variables are externalized
3. Service dependencies are well-defined
4. Health checks are implemented
5. Resource limits are configured

Use the existing `deployments/k8s/production.yaml` when ready.

---

## 📞 Support & Troubleshooting

### Getting Help
1. Check this documentation first
2. Review application logs
3. Check the security checklist: `PRODUCTION_SECURITY_CHECKLIST.md`
4. Open an issue on the repository

### Emergency Contacts
```
Application Team: dev@yamata-no-orochi.com
Infrastructure: ops@yamata-no-orochi.com
Security Issues: security@yamata-no-orochi.com
```

### Rollback Procedure
```bash
# Quick rollback to previous version
git checkout HEAD~1
docker-compose -f docker-compose.production.yml build app
docker-compose -f docker-compose.production.yml up -d app

# Database rollback (use backup)
docker-compose -f docker-compose.production.yml exec -T postgres \
    psql -U yamata_user yamata_no_orochi < backup_previous.sql
```

---

## ✅ Post-Deployment Checklist

- [ ] Application accessible via HTTPS
- [ ] API endpoints responding correctly
- [ ] Database connections working
- [ ] SSL certificates valid and auto-renewing
- [ ] Monitoring dashboards accessible
- [ ] Log rotation configured
- [ ] Firewall rules in place
- [ ] Backup strategy implemented
- [ ] DNS records configured
- [ ] Rate limiting working
- [ ] Security headers present
- [ ] Performance acceptable

**🎉 Congratulations! Your Yamata no Orochi API is now running in production!** 