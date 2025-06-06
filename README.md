# Yamata no Orochi – Backend API

A production-grade Go backend built with Fiber v3, GORM (PostgreSQL), and Clean Architecture. It provides authentication, SMS campaign management, wallet balance, Swagger/OpenAPI for development, robust error handling, and migrations with strict failure reporting.

## Features
- Clean, modular architecture (handlers, business flows, repositories, models)
- JWT auth with access/refresh tokens
- SMS campaign CRUD flows (create, update, list with filters/pagination)
- Pricing and capacity calculators for campaigns
- Wallet balance endpoint (mocked for now)
- Development-only Swagger UI, served locally with CSP-safe assets
- Centralized error handling and unified time constants
- Database migrations with strict failure handling (ON_ERROR_STOP=1)
- Dockerized local deployment with self-signed certificates

## Project Structure
```
cmd/                     # Entrypoints (future)
internal/                # Core (future)
pkg/                     # Shared libs (future)
api/                     # Transport defs (future)
app/                     # DTOs, handlers, middleware, router, services
business_flow/           # Use-cases / domain services
models/                  # GORM models
repository/              # Data access with GORM BaseRepository
migrations/              # SQL migrations and run_all_up/down
scripts/                 # Dev/deploy scripts
utils/                   # Helpers and constants
docs/                    # Swagger outputs and UI assets
```

## Requirements
- Go 1.22+
- PostgreSQL 15+
- Redis (optional, used in scripts)
- Make, Bash, Docker (for dockerized flows)

## Quick Start
### Simple local run (no Docker)
This path is designed for fast development.

1) Run the helper script (interactive, offers to run migrations):
```bash
./scripts/run-dev.sh
```
- Generates temporary JWT keys
- Sets development env vars
- Prompts to initialize DB and run migrations
- Runs `swag init` automatically
- Starts the app at http://localhost:8080

2) Alternatively via Makefile:
```bash
make run-dev-simple  # also runs `make swag`
```

### Dockerized local deployment (full stack, self-signed cert)
```bash
./scripts/deploy-local.sh
```
- Brings up Nginx + App + Postgres + Redis
- Generates self-signed certificates

## Swagger / OpenAPI (development only)
- Generate docs:
```bash
make swag        # or: make swag-init / make swag-clean
```
- Dev-only routes:
  - Swagger JSON: GET `/api/v1/swagger.json`
  - Standalone UI: GET `/swagger-standalone`
  - UI assets served locally: `/swagger-ui-assets/*` (CSP-safe)
- Paths in the spec include `/api/v1` prefix.

Production never exposes Swagger UI.

## Database Migrations
- All migrations are applied via `migrations/run_all_up.sql`, rolled back via `run_all_down.sql`.
- Scripts now use strict error handling and fail fast:
  - `psql -v ON_ERROR_STOP=1` (no output redirection)
  - Errors are surfaced and stop the script

Apply via development script:
```bash
./scripts/run-dev.sh     # prompts to run migrations
# or
./scripts/init-local-database.sh
```

Recent schema changes:
- `0014_create_sms_campaigns.sql`: adds `sms_campaigns` table
- `0015_add_sms_campaign_audit_actions.sql`: adds campaign audit actions to enum
- `0016_add_comment_to_sms_campaigns.sql`: adds nullable `comment TEXT` to `sms_campaigns`

## API Endpoints
All endpoints are under `/api/v1`.

### Health
- GET `/api/v1/health`

### Auth
- POST `/api/v1/auth/signup`
- POST `/api/v1/auth/verify`
- POST `/api/v1/auth/resend-otp`
- POST `/api/v1/auth/login` → returns `access_token`, `refresh_token`, `token_type`, `expires_in`, `customer`
- POST `/api/v1/auth/forgot-password`
- POST `/api/v1/auth/reset` → returns tokens like login

### SMS Campaigns (Bearer auth required)
- POST `/api/v1/sms-campaigns` – Create
- PUT `/api/v1/sms-campaigns/:uuid` – Update (only owner; blocked if waiting-for-approval/approved/rejected)
- GET `/api/v1/sms-campaigns` – List with pagination and filters
  - Query params:
    - `page` (int, default 1)
    - `limit` (int, default 10, max 100)
    - `orderby` (newest|oldest)
    - `title` (contains)
    - `status` (initiated|in-progress|waiting-for-approval|approved|rejected)
- POST `/api/v1/sms-campaigns/calculate-capacity` – returns fixed capacity (e.g., 11000)
- POST `/api/v1/sms-campaigns/calculate-cost` – calculates SMS cost based on content and capacity

Model highlights (`sms_campaigns`):
- `id` (uint PK), `uuid` (UUID unique), `customer_id` (FK), `status` (enum)
- `created_at`, `updated_at`, `spec` (JSONB)
- `comment` (nullable text) – used by admin to explain rejections

### Wallet (Bearer auth required)
- GET `/api/v1/wallet/balance` – returns mocked balance data for now
  - `free`, `locked`, `frozen`, `total`, `currency`, `last_updated`, `pending_transactions`, `minimum_balance`, `credit_limit`, `balance_status`

## Standard Response Shape
```json
{
  "success": true,
  "message": "...",
  "data": { }
}
```
Errors:
```json
{
  "success": false,
  "message": "...",
  "error": { "code": "SOME_CODE", "details": { } }
}
```

## Curl Examples
Signup (self-signed TLS):
```bash
curl -k -X POST "https://yamata-no-orochi.com/api/v1/auth/signup" \
  -H "Content-Type: application/json" \
  -d '{
    "account_type": "individual",
    "representative_first_name": "Jane",
    "representative_last_name": "Doe",
    "representative_mobile": "+989123456789",
    "email": "jane@example.com",
    "password": "StrongPass1",
    "confirm_password": "StrongPass1"
  }'
```
Wallet balance:
```bash
curl -X GET "http://localhost:8080/api/v1/wallet/balance" \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```
List campaigns:
```bash
curl -X GET "http://localhost:8080/api/v1/sms-campaigns?page=1&limit=10&orderby=newest&title=promo&status=initiated" \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

## Makefile Targets
- `make swag` / `make swag-init` / `make swag-clean` – generate/clean Swagger docs
- `make run-dev-simple` – simple local run; also runs `make swag`
- `make deploy-local` – dockerized local deployment with self-signed certs
- `make migrate` / `make migrate-create` – migration helpers
- `make swagger-ui` – open standalone Swagger UI

## Observability & Context
- Request-scoped context created in handlers with timeouts
- Request ID middleware; IDs propagated to responses and logs

## Coding Standards
- Go fmt, goimports, golangci-lint recommended
- Small, single-responsibility functions; explicit error handling with wrapping (fmt.Errorf("context: %w", err))
- Interface-driven development and dependency injection

## Testing
Run all tests:
```bash
go test ./...
```

---
If anything looks out of date, please open an issue or ping maintainers. This README reflects the latest implemented features, including strict migration error handling and the new `comment` field on `sms_campaigns`. 