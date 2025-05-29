# Production Configuration Guide

This guide explains how to configure the Yamata no Orochi application for production deployment.

## 🔧 Configuration Overview

The application uses environment variables for configuration, making it suitable for containerized deployments and cloud environments.

## 📋 Environment Variables

### Application Environment
- `APP_ENV`: Set to `production` for production deployments

### Database Configuration
- `DB_HOST`: Database host (e.g., `db.example.com`)
- `DB_PORT`: Database port (default: `5432`)
- `DB_NAME`: Database name (e.g., `yamata_production`)
- `DB_USER`: Database username
- `DB_PASSWORD`: Database password (required in production)
- `DB_SSL_MODE`: SSL mode (`require`, `verify-ca`, `verify-full` for production)
- `DB_MAX_OPEN_CONNS`: Maximum open connections (default: `25`)
- `DB_MAX_IDLE_CONNS`: Maximum idle connections (default: `5`)
- `DB_CONN_MAX_LIFETIME_MINUTES`: Connection lifetime (default: `15`)

### Server Configuration
- `SERVER_HOST`: Server host (use `0.0.0.0` for containerized deployments)
- `SERVER_PORT`: Server port (default: `8080`)
- `SERVER_READ_TIMEOUT_SECONDS`: Read timeout (default: `30`)
- `SERVER_WRITE_TIMEOUT_SECONDS`: Write timeout (default: `30`)
- `SERVER_IDLE_TIMEOUT_SECONDS`: Idle timeout (default: `60`)

### JWT Configuration
- `JWT_ISSUER`: JWT issuer (e.g., `yamata-orochi`)
- `JWT_AUDIENCE`: JWT audience (e.g., `yamata-api`)
- `JWT_ACCESS_TOKEN_TTL`: Access token lifetime (e.g., `15m`)
- `JWT_REFRESH_TOKEN_TTL`: Refresh token lifetime (e.g., `168h`)

### SMS Configuration
- `SMS_PROVIDER`: SMS provider (`mock`, `iranian`)
- `SMS_USERNAME`: SMS provider username
- `SMS_PASSWORD`: SMS provider password
- `SMS_FROM_NUMBER`: Sender phone number
- `SMS_API_KEY`: SMS provider API key
- `SMS_API_URL`: SMS provider API URL

### Email Configuration
- `EMAIL_HOST`: SMTP host (e.g., `smtp.gmail.com`)
- `EMAIL_PORT`: SMTP port (e.g., `587`)
- `EMAIL_USERNAME`: SMTP username
- `EMAIL_PASSWORD`: SMTP password
- `EMAIL_FROM_EMAIL`: Sender email address
- `EMAIL_USE_TLS`: Use TLS (default: `true`)

### Logging Configuration
- `LOG_LEVEL`: Log level (`debug`, `info`, `warn`, `error`)
- `LOG_FORMAT`: Log format (`json`, `text`)
- `LOG_OUTPUT_PATH`: Log output path (`stdout`, file path)

## 🚀 Production Deployment

### 1. Environment Setup

Create a `.env` file based on the template:

```bash
cp config.env.template .env
```

Edit the `.env` file with your production values:

```env
APP_ENV=production

# Database (use managed database service)
DB_HOST=your-production-db.example.com
DB_PORT=5432
DB_NAME=yamata_production
DB_USER=yamata_user
DB_PASSWORD=your_very_secure_password
DB_SSL_MODE=require

# Server
SERVER_HOST=0.0.0.0
SERVER_PORT=8080

# JWT
JWT_ISSUER=yamata-orochi
JWT_AUDIENCE=yamata-api
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TOKEN_TTL=168h

# SMS (use real provider)
SMS_PROVIDER=iranian
SMS_USERNAME=your_sms_username
SMS_PASSWORD=your_sms_password
SMS_FROM_NUMBER=+989000000000

# Email (use real SMTP)
EMAIL_HOST=smtp.gmail.com
EMAIL_PORT=587
EMAIL_USERNAME=your_email@gmail.com
EMAIL_PASSWORD=your_app_password
EMAIL_FROM_EMAIL=noreply@yamata-no-orochi.com

# Logging
LOG_LEVEL=info
LOG_FORMAT=json
```

### 2. Docker Deployment

Create a `Dockerfile`:

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/main .
COPY --from=builder /app/config.env.template .

EXPOSE 8080
CMD ["./main"]
```

Create a `docker-compose.yml` for local testing:

```yaml
version: '3.8'

services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - APP_ENV=production
      - DB_HOST=db
      - DB_PORT=5432
      - DB_NAME=yamata_production
      - DB_USER=yamata_user
      - DB_PASSWORD=your_password
      - DB_SSL_MODE=disable
    depends_on:
      - db
    volumes:
      - ./logs:/app/logs

  db:
    image: postgres:15-alpine
    environment:
      - POSTGRES_DB=yamata_production
      - POSTGRES_USER=yamata_user
      - POSTGRES_PASSWORD=your_password
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"

volumes:
  postgres_data:
```

### 3. Kubernetes Deployment

Create `k8s/deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: yamata-orochi
spec:
  replicas: 3
  selector:
    matchLabels:
      app: yamata-orochi
  template:
    metadata:
      labels:
        app: yamata-orochi
    spec:
      containers:
      - name: yamata-orochi
        image: your-registry/yamata-orochi:latest
        ports:
        - containerPort: 8080
        env:
        - name: APP_ENV
          value: "production"
        - name: DB_HOST
          valueFrom:
            secretKeyRef:
              name: db-secret
              key: host
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: db-secret
              key: password
        # Add other environment variables...
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
```

### 4. Security Considerations

#### Database Security
- Use managed database services (AWS RDS, Google Cloud SQL, etc.)
- Enable SSL/TLS connections
- Use strong passwords
- Implement connection pooling
- Regular backups

#### JWT Security
- Use strong, unique issuer and audience values
- Set appropriate token expiration times
- Implement token revocation
- Use HTTPS in production

#### SMS/Email Security
- Use API keys instead of passwords where possible
- Store credentials in secrets management systems
- Use dedicated service accounts
- Monitor usage and costs

#### Application Security
- Use HTTPS/TLS
- Implement rate limiting
- Set up monitoring and alerting
- Regular security updates
- Input validation and sanitization

### 5. Monitoring and Logging

#### Health Checks
The application provides a health endpoint at `/health` for monitoring.

#### Logging
- Use structured logging (JSON format)
- Send logs to centralized logging systems
- Set appropriate log levels
- Monitor for errors and performance issues

#### Metrics
Consider adding metrics collection:
- Request rates and response times
- Database connection pool usage
- Error rates
- Custom business metrics

### 6. Environment-Specific Configurations

#### Development
```env
APP_ENV=development
DB_SSL_MODE=disable
SMS_PROVIDER=mock
LOG_LEVEL=debug
```

#### Staging
```env
APP_ENV=staging
DB_SSL_MODE=require
SMS_PROVIDER=iranian
LOG_LEVEL=info
```

#### Production
```env
APP_ENV=production
DB_SSL_MODE=verify-full
SMS_PROVIDER=iranian
LOG_LEVEL=warn
```

### 7. Configuration Validation

The application validates configuration on startup and will fail fast if:
- Required environment variables are missing
- Invalid values are provided
- Production requirements are not met

### 8. Troubleshooting

#### Common Issues
1. **Database Connection**: Check host, port, credentials, and SSL settings
2. **JWT Issues**: Verify issuer, audience, and token expiration
3. **SMS/Email**: Test credentials and API endpoints
4. **Port Conflicts**: Ensure the application port is available

#### Debug Mode
For troubleshooting, temporarily set:
```env
LOG_LEVEL=debug
APP_ENV=development
```

This will provide more detailed logging and relaxed validation. 