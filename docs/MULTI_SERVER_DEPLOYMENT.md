# 🌐 Multi-Server Deployment Guide

This guide covers deploying Yamata no Orochi to multiple servers with different domains. Perfect for staging/production separation, multi-tenant deployments, or geographical distribution.

## 🎯 **Overview**

The multi-server deployment system allows you to:
- **Deploy to different domains** for different environments
- **Manage separate configurations** for each server
- **Automate deployments** with environment-specific scripts
- **Maintain consistency** across all deployments
- **Scale horizontally** across multiple servers

---

## 🏗️ **Architecture**

```
Production Server 1    ┌─────────────────────┐
example.com           │  Yamata no Orochi   │
api.example.com       │     (Environment 1)  │
                      └─────────────────────┘

Staging Server        ┌─────────────────────┐
staging.example.com   │  Yamata no Orochi   │
api-staging.example   │     (Environment 2)  │
                      └─────────────────────┘

Development Server    ┌─────────────────────┐
dev.example.com       │  Yamata no Orochi   │
api-dev.example.com   │     (Environment 3)  │
                      └─────────────────────┘
```

Each server has:
- **Independent configuration** with its own domains
- **Separate databases** and Redis instances
- **Environment-specific** SSL certificates
- **Isolated monitoring** and logging

---

## 🛠️ **Setup Process**

### **Step 1: Generate Environment Configuration**

Use the configuration generator to create environment-specific setups:

```bash
# Generate configuration for production server
./scripts/generate-config.sh \
  -e production \
  -d example.com \
  -a api.example.com \
  -m monitoring.example.com \
  -c admin@example.com

# Generate configuration for staging server
./scripts/generate-config.sh \
  -e staging \
  -d staging.example.com \
  -a api-staging.example.com \
  -c admin@example.com

# Generate configuration for development server
./scripts/generate-config.sh \
  -e development \
  -d dev.example.com \
  -a api-dev.example.com \
  -c admin@example.com

# Generate template for custom server
./scripts/generate-config.sh -e server1 -t
```

### **Step 2: Directory Structure**

After generation, your project will have:

```
deployments/environments/
├── production/
│   ├── .env.production          # Production environment variables
│   ├── docker-compose.override.yml  # Production-specific overrides
│   ├── deploy.sh               # One-click deployment script
│   └── README.md               # Environment documentation
├── staging/
│   ├── .env.production         # Staging environment variables
│   ├── docker-compose.override.yml  # Staging-specific overrides
│   ├── deploy.sh               # One-click deployment script
│   └── README.md               # Environment documentation
├── development/
│   └── ... (similar structure)
└── server1/
    └── ... (template structure)
```

### **Step 3: Configure Each Environment**

For each environment, edit the `.env.production` file:

```bash
# Edit production configuration
nano deployments/environments/production/.env.production

# Edit staging configuration  
nano deployments/environments/staging/.env.production
```

**Required Configuration Updates:**

```bash
# Database Configuration
DB_PASSWORD=unique_secure_password_for_this_env

# JWT Configuration (use different key for each environment)
JWT_SECRET_KEY=environment_specific_256_bit_key

# SMS/Email Configuration (can be shared or separate)
SMS_API_KEY=your_sms_api_key
EMAIL_PASSWORD=your_email_password

# Environment-specific settings
GRAFANA_ADMIN_PASSWORD=secure_grafana_password
```

---

## 🚀 **Deployment Methods**

### **Method 1: One-Click Deployment (Recommended)**

Deploy to any environment with a single command:

```bash
# Deploy to production
cd deployments/environments/production
./deploy.sh

# Deploy to staging
cd deployments/environments/staging  
./deploy.sh

# Deploy to development
cd deployments/environments/development
./deploy.sh
```

### **Method 2: Manual Deployment**

For more control over the deployment process:

```bash
# From project root
cp deployments/environments/production/.env.production .env.production
cp deployments/environments/production/docker-compose.override.yml docker-compose.override.yml
./scripts/deploy-docker-compose.sh
```

### **Method 3: Remote Deployment**

Deploy to remote servers using SSH:

```bash
# Copy configuration to remote server
scp -r deployments/environments/production/ user@server:/path/to/yamata-no-orochi/

# SSH to server and deploy
ssh user@server "cd /path/to/yamata-no-orochi/deployments/environments/production && ./deploy.sh"
```

---

## 🔧 **Advanced Configuration**

### **Environment-Specific Overrides**

Each environment can have custom Docker Compose overrides:

```yaml
# deployments/environments/staging/docker-compose.override.yml
version: '3.8'

services:
  app:
    environment:
      - LOG_LEVEL=debug
      - DEBUG_MODE=true
    labels:
      - "environment=staging"
    
  postgres:
    environment:
      - POSTGRES_DB=yamata_staging
    
  nginx:
    ports:
      - "8080:80"  # Different port for staging
      - "8443:443"
```

### **Custom Domain Configurations**

For complex domain setups, you can customize the Nginx configuration:

```bash
# Generate custom nginx config for environment
envsubst < docker/nginx/sites-available/yamata.conf.template > \
  deployments/environments/production/nginx-custom.conf
```

### **Multi-Region Deployment**

Deploy to different geographical regions:

```bash
# US East Coast
./scripts/generate-config.sh \
  -e us-east \
  -d us.example.com \
  -a api-us.example.com \
  -c admin@example.com

# EU Region  
./scripts/generate-config.sh \
  -e eu-west \
  -d eu.example.com \
  -a api-eu.example.com \
  -c admin@example.com

# Asia Pacific
./scripts/generate-config.sh \
  -e ap-southeast \
  -d ap.example.com \
  -a api-ap.example.com \
  -c admin@example.com
```

---

## 🌍 **DNS Configuration**

### **Single Domain with Subdomains**

```
example.com           → Production server
api.example.com       → Production server
staging.example.com   → Staging server
api-staging.example.com → Staging server
dev.example.com       → Development server
api-dev.example.com   → Development server
```

### **Multiple Domains**

```
production.com        → Production server
api.production.com    → Production server
staging.company.dev   → Staging server
api-staging.company.dev → Staging server
```

### **Regional Domains**

```
us.example.com        → US server
api-us.example.com    → US server
eu.example.com        → EU server
api-eu.example.com    → EU server
```

**DNS Record Examples:**

```bash
# A Records
example.com.           300   IN  A     192.168.1.100
api.example.com.       300   IN  A     192.168.1.100
staging.example.com.   300   IN  A     192.168.1.101
api-staging.example.com. 300 IN  A     192.168.1.101

# CNAME Records (alternative)
www.example.com.       300   IN  CNAME example.com.
monitoring.example.com. 300  IN  CNAME example.com.
```

---

## 🔐 **SSL Certificate Management**

### **Automatic SSL with Let's Encrypt**

Each environment automatically gets SSL certificates:

```bash
# Production certificates
/etc/letsencrypt/live/example.com/
├── fullchain.pem
├── privkey.pem
└── chain.pem

# Staging certificates  
/etc/letsencrypt/live/staging.example.com/
├── fullchain.pem
├── privkey.pem
└── chain.pem
```

### **Wildcard Certificates**

For environments with many subdomains:

```bash
# Request wildcard certificate
certbot certonly \
  --dns-cloudflare \
  --dns-cloudflare-credentials ~/.secrets/certbot/cloudflare.ini \
  -d "*.example.com" \
  -d "example.com"
```

### **Custom SSL Certificates**

For enterprise or custom certificates:

```bash
# Place certificates in environment-specific location
deployments/environments/production/ssl/
├── fullchain.pem
├── privkey.pem
└── chain.pem
```

---

## 📊 **Monitoring Multiple Environments**

### **Per-Environment Monitoring**

Each environment has its own monitoring stack:

```bash
# Production monitoring
https://monitoring.example.com/grafana/
https://monitoring.example.com/prometheus/

# Staging monitoring
https://monitoring-staging.example.com/grafana/
https://monitoring-staging.example.com/prometheus/
```

### **Centralized Monitoring**

Aggregate metrics from all environments:

```yaml
# prometheus/federated.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'federate-production'
    scrape_interval: 15s
    honor_labels: true
    metrics_path: '/federate'
    params:
      'match[]':
        - '{job=~"yamata-.*"}'
    static_configs:
      - targets:
        - 'monitoring.example.com:9090'
        
  - job_name: 'federate-staging'
    scrape_interval: 15s
    honor_labels: true
    metrics_path: '/federate'
    params:
      'match[]':
        - '{job=~"yamata-.*"}'
    static_configs:
      - targets:
        - 'monitoring-staging.example.com:9090'
```

---

## 🔄 **CI/CD for Multi-Server Deployment**

### **GitHub Actions Workflow**

```yaml
# .github/workflows/multi-deploy.yml
name: Multi-Server Deployment

on:
  push:
    branches: [ main, develop ]
  workflow_dispatch:
    inputs:
      environment:
        description: 'Target environment'
        required: true
        default: 'staging'
        type: choice
        options:
        - staging
        - production
        - development

jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: ${{ github.event.inputs.environment || 'staging' }}
    
    steps:
    - uses: actions/checkout@v4
    
    - name: Deploy to Environment
      run: |
        ENV=${{ github.event.inputs.environment || 'staging' }}
        echo "Deploying to environment: $ENV"
        
        # Copy environment configuration
        cp deployments/environments/$ENV/.env.production .env.production
        
        # Update secrets from GitHub
        sed -i "s/YOUR_SECURE_DATABASE_PASSWORD_HERE/${{ secrets.DB_PASSWORD }}/" .env.production
        sed -i "s/YOUR_256_BIT_JWT_SECRET_KEY_HERE/${{ secrets.JWT_SECRET_KEY }}/" .env.production
        
        # Deploy using SSH
        echo "${{ secrets.SSH_PRIVATE_KEY }}" > ~/.ssh/id_rsa
        chmod 600 ~/.ssh/id_rsa
        
        rsync -avz --delete ./ ${{ secrets.SSH_USER }}@${{ secrets.SSH_HOST }}:/path/to/yamata/
        ssh ${{ secrets.SSH_USER }}@${{ secrets.SSH_HOST }} \
          "cd /path/to/yamata/deployments/environments/$ENV && ./deploy.sh"
```

### **Automated Environment Promotion**

```bash
#!/bin/bash
# scripts/promote-environment.sh

# Promote staging to production
STAGING_TAG=$(git describe --tags --abbrev=0)
git tag "production-$STAGING_TAG"
git push origin "production-$STAGING_TAG"

# Deploy to production
cd deployments/environments/production
./deploy.sh
```

---

## 🛡️ **Security Considerations**

### **Environment Isolation**

- **Separate databases** for each environment
- **Different JWT secrets** to prevent token sharing
- **Environment-specific** API keys and passwords
- **Isolated networking** between environments

### **Access Control**

```bash
# Production access (restricted)
production:
  - admin@company.com
  - devops@company.com

# Staging access (broader)
staging:
  - admin@company.com
  - devops@company.com
  - developers@company.com

# Development access (open to team)
development:
  - "*@company.com"
```

### **Secrets Management**

```bash
# Use different secret management per environment
production:
  secrets_provider: "aws-secrets-manager"
  secrets_region: "us-east-1"
  
staging:
  secrets_provider: "hashicorp-vault"
  vault_address: "https://vault-staging.company.com"
  
development:
  secrets_provider: "local-env"
```

---

## 🚨 **Troubleshooting**

### **Common Issues**

#### **1. Domain Resolution Problems**
```bash
# Check DNS resolution
dig example.com
nslookup api.example.com

# Test connectivity
curl -I https://example.com/health
```

#### **2. SSL Certificate Issues**
```bash
# Check certificate validity
openssl x509 -in /etc/letsencrypt/live/example.com/fullchain.pem -text -noout

# Renew certificates manually
certbot renew --cert-name example.com
```

#### **3. Environment Variable Problems**
```bash
# Validate environment configuration
cd deployments/environments/production
source .env.production
echo "Domain: $DOMAIN"
echo "API Domain: $API_DOMAIN"
```

#### **4. Port Conflicts**
```bash
# Check port usage
netstat -tulpn | grep :80
netstat -tulpn | grep :443

# Use different ports for staging
# In docker-compose.override.yml:
ports:
  - "8080:80"
  - "8443:443"
```

### **Health Check Script**

```bash
#!/bin/bash
# scripts/health-check-all.sh

environments=("production" "staging" "development")

for env in "${environments[@]}"; do
    echo "Checking $env environment..."
    
    # Load environment variables
    source "deployments/environments/$env/.env.production"
    
    # Check health endpoint
    if curl -f -s "https://$DOMAIN/health" > /dev/null; then
        echo "✅ $env ($DOMAIN) - Healthy"
    else
        echo "❌ $env ($DOMAIN) - Unhealthy"
    fi
    
    # Check API endpoint
    if curl -f -s "https://$API_DOMAIN/api/v1/docs" > /dev/null; then
        echo "✅ $env API ($API_DOMAIN) - Healthy"
    else
        echo "❌ $env API ($API_DOMAIN) - Unhealthy"
    fi
    
    echo ""
done
```

---

## 📋 **Deployment Checklist**

### **Pre-Deployment**
- [ ] DNS records configured and propagated
- [ ] Environment configuration generated
- [ ] Secrets and passwords updated
- [ ] SSL email address configured
- [ ] Firewall rules configured (ports 80, 443, 22)

### **During Deployment**
- [ ] Monitor deployment logs
- [ ] Verify SSL certificate acquisition
- [ ] Check service health endpoints
- [ ] Validate database connectivity
- [ ] Test API endpoints

### **Post-Deployment**
- [ ] Verify all URLs are accessible
- [ ] Check monitoring dashboards
- [ ] Validate SSL certificates
- [ ] Test authentication flows
- [ ] Verify backup systems
- [ ] Update DNS if needed

---

## 📚 **Examples**

### **Example 1: Company with Staging/Production**

```bash
# Generate production environment
./scripts/generate-config.sh \
  -e production \
  -d mycompany.com \
  -a api.mycompany.com \
  -m monitoring.mycompany.com \
  -c admin@mycompany.com

# Generate staging environment
./scripts/generate-config.sh \
  -e staging \
  -d staging.mycompany.com \
  -a api-staging.mycompany.com \
  -c admin@mycompany.com

# Deploy staging first
cd deployments/environments/staging
./deploy.sh

# Deploy production after testing
cd ../production
./deploy.sh
```

### **Example 2: Multi-Tenant SaaS**

```bash
# Tenant 1
./scripts/generate-config.sh \
  -e tenant1 \
  -d client1.mysaas.com \
  -a api.client1.mysaas.com \
  -c admin@client1.com

# Tenant 2  
./scripts/generate-config.sh \
  -e tenant2 \
  -d client2.mysaas.com \
  -a api.client2.mysaas.com \
  -c admin@client2.com

# Deploy each tenant to separate servers
```

### **Example 3: Geographic Distribution**

```bash
# US Region
./scripts/generate-config.sh \
  -e us-east \
  -d us.globalapp.com \
  -a api-us.globalapp.com \
  -c admin@globalapp.com

# EU Region
./scripts/generate-config.sh \
  -e eu-west \
  -d eu.globalapp.com \
  -a api-eu.globalapp.com \
  -c admin@globalapp.com

# Asia Pacific
./scripts/generate-config.sh \
  -e ap-southeast \
  -d ap.globalapp.com \
  -a api-ap.globalapp.com \
  -c admin@globalapp.com
```

---

## 🎯 **Best Practices**

### **Configuration Management**
- ✅ Use environment-specific configuration files
- ✅ Never hardcode domains in source code
- ✅ Keep secrets separate from configuration
- ✅ Version control environment templates
- ✅ Document all environment differences

### **Deployment Strategy**
- ✅ Always deploy to staging first
- ✅ Use blue-green deployments for zero downtime
- ✅ Automate as much as possible
- ✅ Have rollback procedures ready
- ✅ Monitor deployments closely

### **Security**
- ✅ Use different credentials for each environment
- ✅ Restrict access based on environment sensitivity
- ✅ Keep production completely isolated
- ✅ Use VPNs for internal monitoring access
- ✅ Regular security audits

### **Monitoring**
- ✅ Set up alerts for each environment
- ✅ Monitor resource usage trends
- ✅ Track deployment success/failure rates
- ✅ Set up centralized logging
- ✅ Regular health checks

---

**🎉 You now have a complete multi-server deployment system! Each environment can be deployed independently with its own domain configuration while maintaining consistency and security across all deployments.** 