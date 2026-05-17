# 🐉 Yamata no Orochi API

[![CI/CD Pipeline](https://github.com/amirphl/yamata-no-orochi/actions/workflows/ci.yml/badge.svg)](https://github.com/amirphl/yamata-no-orochi/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/amirphl/yamata-no-orochi)](https://goreportcard.com/report/github.com/amirphl/yamata-no-orochi)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org)
[![Docker](https://img.shields.io/badge/Docker-Ready-blue.svg)](https://docker.com)
[![Security](https://img.shields.io/badge/Security-OWASP%20Compliant-green.svg)](./PRODUCTION_SECURITY_CHECKLIST.md)

A production-ready, secure, and scalable Go API built with clean architecture principles for user authentication and management. Features comprehensive security, monitoring, and deployment automation.

## 🎯 **Quick Start**

**Your API will be running at `https://thewritingonthewall.com` with full monitoring!**

### **🚀 Production Deployment (3 Commands)**
```bash
git clone https://github.com/amirphl/yamata-no-orochi.git
cd yamata-no-orochi
cp env.production.template .env.production
# Edit .env.production with your values
./scripts/deploy-docker-compose.sh
```
**Your API will be running at `https://your-domain.com` with full monitoring!**

### **💻 Manual Local Development**
```bash
git clone https://github.com/amirphl/yamata-no-orochi.git
cd yamata-no-orochi
go mod download
docker-compose up -d postgres redis  # Start dependencies
go run main.go                       # Start application
```

📖 **See [Local Deployment Guide](LOCAL_DEPLOYMENT.md) for detailed instructions.**

## 🏗️ **Architecture Overview**

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│     Client      │    │      Nginx      │    │   Go App (API)  │
│   (Frontend)    │◄──►│ (Reverse Proxy) │◄──►│   (Fiber v3)    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                                                        │
                       ┌─────────────────┐              │
                       │   Monitoring    │              │
                       │ Prometheus +    │◄─────────────┤
                       │    Grafana      │              │
                       └─────────────────┘              │
                                                        │
                       ┌─────────────────┐              │
                       │   PostgreSQL    │◄─────────────┤
                       │   (Database)    │              │
                       └─────────────────┘              │
                                                        │
                       ┌─────────────────┐              │
                       │     Redis       │◄─────────────┘
                       │    (Cache)      │
                       └─────────────────┘
```

## ✨ **Key Features**

### 🔒 **Security First**
- ✅ **OWASP Compliant** security implementation
- ✅ **JWT Authentication** with secure token rotation
- ✅ **OTP Verification** system (SMS/Email)
- ✅ **Rate Limiting** (5 auth requests/minute, 1000 global/minute)
- ✅ **Input Validation** with custom rules
- ✅ **SQL Injection Prevention** via parameterized queries
- ✅ **XSS Protection** with security headers
- ✅ **bcrypt Password Hashing** (cost 12)
- ✅ **Audit Logging** for all user actions

### 🚀 **Production Ready**
- ✅ **Docker Compose** deployment with Nginx
- ✅ **Kubernetes** manifests included
- ✅ **Auto-SSL** with Let's Encrypt
- ✅ **Health Checks** and monitoring
- ✅ **Graceful Shutdown** handling
- ✅ **Log Rotation** and structured logging
- ✅ **Resource Limits** and security hardening
- ✅ **Backup Scripts** and disaster recovery

### 📊 **Monitoring & Observability**
- ✅ **Prometheus** metrics collection
- ✅ **Grafana** dashboards
- ✅ **Structured JSON Logging**
- ✅ **Request Tracing** with correlation IDs
- ✅ **Performance Metrics** (response times, throughput)
- ✅ **Database Monitoring** (slow queries, connections)
- ✅ **Security Monitoring** (failed attempts, anomalies)

### 🧪 **Quality Assurance**
- ✅ **CI/CD Pipeline** with GitHub Actions
- ✅ **Automated Testing** (unit, integration, security)
- ✅ **Code Quality** scanning (golangci-lint)
- ✅ **Security Scanning** (Trivy, Gosec)
- ✅ **Dependency Updates** (Dependabot)
- ✅ **Code Coverage** reporting

## 🏛️ **Clean Architecture**

Following Uncle Bob's Clean Architecture principles:

```
┌─────────────────┐
│   Presentation  │  ← app/handlers, app/router
├─────────────────┤
│   Use Cases     │  ← business_flow/
├─────────────────┤
│   Services      │  ← app/services/
├─────────────────┤
│   Repository    │  ← repository/
├─────────────────┤
│    Models       │  ← models/
└─────────────────┘
```

### **Layer Responsibilities**
- **🎯 Presentation**: HTTP handlers, request/response validation
- **🔄 Use Cases**: Business logic orchestration
- **🔌 Services**: External integrations (SMS, Email, JWT)
- **💾 Repository**: Data access abstraction
- **📋 Models**: Core business entities

## 📋 **Project Structure**

```
yamata-no-orochi/
├── 🗃️  migrations/           # Database schema migrations
├── 📊 models/               # Core business entities
├── 🗄️  repository/          # Data access layer
├── 🔄 business_flow/        # Business logic
├── 🌐 app/
│   ├── dto/                # Data Transfer Objects
│   ├── handlers/           # HTTP handlers
│   ├── services/           # External services
│   └── router/             # Route configuration
├── 🐳 docker/              # Docker configurations
├── ☸️  deployments/         # Kubernetes manifests
├── 🔧 scripts/             # Deployment scripts
├── 📚 docs/                # Documentation
└── 🧪 .github/             # CI/CD workflows
```

## 🗄️ **Database Schema**

### **Core Tables**
- **`account_types`**: Account type definitions (individual, company, agency)
- **`customers`**: Unified customer entity with conditional fields
- **`customer_sessions`**: JWT session tracking
- **`audit_log`**: Comprehensive audit trail

### **Migration Management**
```bash
# Apply all migrations
psql -U postgres -d yamata_db -f migrations/run_all_up.sql

# Rollback all migrations
psql -U postgres -d yamata_db -f migrations/run_all_down.sql

# Apply specific migration
psql -U postgres -d yamata_db -f migrations/0001_create_account_types.sql
```

## 🔌 **API Endpoints**

### **Authentication**

#### **User Registration**
```http
POST /api/v1/auth/signup
Content-Type: application/json

{
  "account_type": "individual",
  "representative_first_name": "John",
  "representative_last_name": "Doe",
  "representative_mobile": "+989123456789",
  "email": "john@example.com",
  "password": "SecurePass123!",
  "confirm_password": "SecurePass123!"
}
```

#### **OTP Verification**
```http
POST /api/v1/auth/verify
Content-Type: application/json

{
  "customer_id": 123,
  "otp_code": "123456",
  "otp_type": "mobile"
}
```

#### **Resend OTP**
```http
POST /api/v1/auth/resend-otp/123
```

#### **User Login** ✨ **NEW**
```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "identifier": "john@example.com",
  "password": "SecurePass123!"
}
```

#### **Forgot Password** ✨ **NEW**
```http
POST /api/v1/auth/forgot-password
Content-Type: application/json

{
  "identifier": "john@example.com"
}
```

#### **Reset Password** ✨ **NEW**
```http
POST /api/v1/auth/reset
Content-Type: application/json

{
  "customer_id": 123,
  "otp_code": "654321",
  "new_password": "NewSecurePass123!",
  "confirm_password": "NewSecurePass123!"
}
```

### **System**
```http
GET /api/v1/health
GET /api/v1/docs
GET /metrics (Prometheus metrics)
```

## 🔧 **Development**

### **Prerequisites**
- **Go 1.21+**
- **Docker & Docker Compose**
- **PostgreSQL 15+**
- **Redis 7+**
- **Git**

### **Code Quality**
```bash
# Run tests with coverage
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Lint code
golangci-lint run

# Security scan
gosec ./...

# Format code
gofmt -w .
goimports -w .
```

### **Docker Development**
```bash
# Build application
docker build -f docker/Dockerfile.production -t yamata-no-orochi .

# Run full stack
docker-compose -f docker-compose.production.yml up -d

# View logs
docker-compose logs -f app
```

## 📚 **Documentation**

### **📖 Comprehensive Guides**
- **[🚀 Production Deployment](./PRODUCTION_DEPLOYMENT.md)** - Complete deployment guide
- **[🌐 Multi-Server Deployment](./MULTI_SERVER_DEPLOYMENT.md)** - Deploy to multiple servers with different domains
- **[🔒 Security Checklist](./PRODUCTION_SECURITY_CHECKLIST.md)** - Security best practices
- **[🏗️ Clean Architecture](./CLEAN_ARCHITECTURE_README.md)** - Architecture principles
- **[🤝 Contributing Guide](./CONTRIBUTING.md)** - How to contribute
- **[📋 Database Migrations](./migrations/README.md)** - Schema management
- **[📝 Changelog](./CHANGELOG.md)** - Release history

### **🔗 Quick Links**
- **[Issue Templates](.github/ISSUE_TEMPLATE/)** - Bug reports & feature requests
- **[PR Template](.github/pull_request_template.md)** - Pull request guidelines
- **[CI/CD Pipeline](.github/workflows/)** - Automated workflows
- **[Docker Configs](./docker/)** - Container configurations

## 🌟 **Production Features**

### **🔐 Security Hardening**
```yaml
Security Headers:
  X-Frame-Options: DENY
  X-Content-Type-Options: nosniff
  X-XSS-Protection: "1; mode=block"
  Strict-Transport-Security: "max-age=31536000"
  Content-Security-Policy: "default-src 'self'..."
  
Rate Limiting:
  Auth Endpoints: 5 requests/minute
  Global API: 1000 requests/minute
  Connection Limit: 20 per IP
```

### **⚡ Performance Optimizations**
```yaml
Database:
  Connection Pool: 200 max connections
  Query Optimization: Indexed searches
  Connection Timeout: 30s
  
Caching:
  Redis: 256MB with LRU eviction
  Nginx: Static file caching
  Application: Query result caching
  
Compression:
  Gzip: Enabled for text content
  Response Size: Up to 90% reduction
```

### **📊 Monitoring Stack**
```yaml
Metrics:
  - Application performance
  - Database statistics
  - Cache hit rates
  - Security events
  
Dashboards:
  - System overview
  - API performance
  - Security monitoring
  - Error tracking
  
Alerts:
  - High error rates
  - Performance degradation
  - Security incidents
  - System resource usage
```

## 🚀 **Deployment Options**

### **🐳 Docker Compose (Recommended for start)**
```bash
./scripts/deploy-docker-compose.sh
```
- **One-command deployment**
- **Automatic SSL certificates**
- **Full monitoring stack**
- **Production-ready configuration**

### **☸️ Kubernetes (Enterprise scale)**
```bash
kubectl apply -f deployments/k8s/production.yaml
```
- **High availability**
- **Auto-scaling**
- **Rolling updates**
- **Advanced networking**

## 🔄 **CI/CD Pipeline**

### **Automated Workflows**
- ✅ **Code Quality**: Linting, formatting, complexity analysis
- ✅ **Security**: Vulnerability scanning, secret detection
- ✅ **Testing**: Unit tests, integration tests, race detection
- ✅ **Building**: Multi-arch Docker images
- ✅ **Deployment**: Automated staging/production deployment
- ✅ **Monitoring**: Health checks and rollback automation

### **Quality Gates**
- **Test Coverage**: >80%
- **Code Quality**: A grade
- **Security**: No high/critical vulnerabilities
- **Performance**: <100ms avg response time

## 🤝 **Contributing**

We welcome contributions! Please see our [Contributing Guide](./CONTRIBUTING.md) for details.

### **Getting Started**
1. **Fork** the repository
2. **Create** a feature branch (`git checkout -b feature/amazing-feature`)
3. **Commit** your changes (`git commit -m 'feat: add amazing feature'`)
4. **Push** to the branch (`git push origin feature/amazing-feature`)
5. **Open** a Pull Request

### **Development Setup**
```bash
git clone https://github.com/your-username/yamata-no-orochi.git
cd yamata-no-orochi
go mod download
docker-compose up -d postgres redis
go run main.go
```

## 📊 **Project Status**

- ✅ **Production Ready**: Full deployment automation
- ✅ **Security Audited**: OWASP compliance verified
- ✅ **Performance Tested**: Load testing completed
- ✅ **Documentation Complete**: Comprehensive guides available
- ✅ **CI/CD Implemented**: Automated quality gates
- ✅ **Monitoring Configured**: Full observability stack

## 📞 **Support & Community**

### **Getting Help**
- **📚 Documentation**: Check our comprehensive guides
- **🐛 Bug Reports**: Use our [issue templates](.github/ISSUE_TEMPLATE/)
- **💡 Feature Requests**: Submit detailed proposals
- **💬 Discussions**: Join our GitHub Discussions

### **Professional Support**
- **🔒 Security Issues**: security@yamata-no-orochi.com
- **🚀 Enterprise Support**: enterprise@yamata-no-orochi.com
- **📧 General Inquiries**: dev@yamata-no-orochi.com

## 📄 **License**

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 **Acknowledgments**

- **Clean Architecture** principles by Robert C. Martin
- **Go community** for excellent tooling and libraries
- **OWASP** for security guidelines
- **Docker** and **Kubernetes** communities
- **Open source contributors** who make this possible

---

**🎉 Ready to deploy a production-grade Go API? Get started with our [3-command deployment](#-production-deployment-3-commands)!** 
