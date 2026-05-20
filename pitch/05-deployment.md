# Deployment Architecture

## Infrastructure Overview

```mermaid
graph TB
    subgraph Internet
        Users[Users / Browsers]
        Bots[External Bots / Integrations]
    end

    subgraph DockerNetwork["Docker Network — yamata-network-beta (172.30.0.0/24)"]
        Nginx[Nginx :80/:443\nReverse Proxy + TLS]

        subgraph AppTier["Application Tier"]
            App[Go App\nyamata-app\n:8080]
            Frontend[Frontend\nyamata-frontend\n:3000]
        end

        subgraph DataTier["Data Tier"]
            Postgres[PostgreSQL 15\nyamata-postgres\n:5432]
            Redis[Redis 8\nyamata-redis\n:6379]
            PGBackup[PG Backup\nScheduled]
        end

        subgraph ObservabilityTier["Observability"]
            Prometheus[Prometheus\n:9090]
            Grafana[Grafana\n:3001]
            GlitchTip[GlitchTip / Sentry\n:8000]
            PGExporter[PostgreSQL Exporter]
            NodeExporter[Node Exporter]
            cAdvisor[cAdvisor]
            SentryForwarder[Nginx→Sentry Forwarder]
            CertMonitor[TLS Cert Monitor]
        end
    end

    subgraph ExternalProviders["External Providers"]
        SMS[PayamSMS — SMS]
        Bale[Bale Messenger]
        Rubika[Rubika Messenger]
        Splus[Soroush Plus]
        Atipay[Atipay — Payment Gateway]
        Oxapay[Oxapay — Crypto Payments]
    end

    Users --> Nginx
    Bots --> Nginx
    Nginx --> App
    Nginx --> Frontend
    App --> Postgres
    App --> Redis
    App --> SMS
    App --> Bale
    App --> Rubika
    App --> Splus
    App --> Atipay
    App --> Oxapay
    Prometheus --> App
    Prometheus --> PGExporter
    Prometheus --> NodeExporter
    Prometheus --> cAdvisor
    PGExporter --> Postgres
    Grafana --> Prometheus
    App --> GlitchTip
    SentryForwarder --> GlitchTip
    PGBackup --> Postgres
    CertMonitor --> Nginx
```

---

## Container Inventory

| Container | Image | Role |
|---|---|---|
| `yamata-app` | `yamata-no-orochi` | Go API server |
| `yamata-frontend` | `yamata-frontend` | Frontend SPA |
| `yamata-nginx` | `nginx:1.29-alpine` | Reverse proxy, TLS termination |
| `yamata-postgres` | `postgres:15-alpine` | Primary database |
| `yamata-redis` | `redis:8.0.3-alpine` | Cache & rate-limit store |
| `yamata-postgres-backup` | `postgres:15-alpine` | Scheduled DB backups |
| `yamata-prometheus` | `prom/prometheus:v3.5.0` | Metrics collection |
| `yamata-grafana` | `grafana/grafana:12.1.0` | Dashboards |
| `yamata-sentry` | `glitchtip/glitchtip` | Error tracking (self-hosted) |
| `yamata-sentry-postgres` | `postgres:15-alpine` | GlitchTip database |
| `yamata-sentry-redis` | `redis:8.0.3-alpine` | GlitchTip cache |
| `yamata-postgres-exporter` | `postgres-exporter:v0.15` | DB metrics for Prometheus |
| `yamata-node-exporter` | `node-exporter:v1.10.2` | Host metrics |
| `yamata-cadvisor` | `cadvisor:v0.49.2` | Container metrics |
| `yamata-cert-monitor` | `yamata-cert-monitor` | TLS expiry alerts |
| `yamata-nginx-sentry-forwarder` | `python:3.12-alpine` | Nginx log → GlitchTip |

---

## Network Layout

```mermaid
flowchart LR
    Internet -->|:443 HTTPS| Nginx
    Nginx -->|:8080| App
    Nginx -->|:3000| Frontend
    App -->|:5432| Postgres
    App -->|:6379| Redis
    App -->|:9091| MetricsEndpoint["Prometheus /metrics"]
    Prometheus -->|scrape :9091| MetricsEndpoint
    Prometheus -->|scrape :9187| PGExporter
    Prometheus -->|scrape :9100| NodeExporter
    Prometheus -->|scrape :8080| cAdvisor
    Grafana -->|query| Prometheus
```

---

## TLS & Nginx Configuration

- TLS termination at Nginx layer
- Rate limiting zones:
  - `api` zone: 2000 req/min per IP
  - `auth` zone: 20 req/min per IP (stricter)
- Proxy headers forwarded: `X-Forwarded-For`, `X-Request-ID`
- Compression enabled at application layer (Brotli/gzip via Fiber)
- HSTS enforced (max-age 1 year)

---

## Graceful Shutdown & Health

```mermaid
sequenceDiagram
    participant OS
    participant App
    participant Schedulers
    participant DB
    participant Redis

    OS->>App: SIGTERM / SIGINT
    App->>Schedulers: Stop all background workers
    Schedulers-->>App: Stopped
    App->>App: Wait for in-flight requests (shutdown timeout)
    App->>DB: Close connection pool
    App->>Redis: Close connection
    App-->>OS: Exit 0
```

Health check endpoint: `GET /api/v1/health` — returns `200 OK` with service status, uptime, and version.

---

## Data Volumes

| Volume | Contents |
|---|---|
| `postgres_data` | Primary DB data files |
| `redis_data` | Redis persistence (AOF/RDB) |
| `app_logs` | Rotating application logs (lumberjack) |
| `nginx_logs` | Access and error logs |
| `uploads` | Uploaded multimedia files |
| `prometheus_data` | Prometheus TSDB |
| `grafana_data` | Grafana dashboards & state |
| `postgres_backups` | Scheduled DB dump archives |
