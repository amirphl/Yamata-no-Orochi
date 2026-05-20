# Authentication & Security

## Authentication Roles

```mermaid
graph LR
    subgraph Principals
        Customer[Customer\nRegular User / Agency]
        Admin[Admin\nBack-Office Staff]
        Bot[Bot\nInternal Automation Service]
    end

    subgraph Methods
        OTP[OTP via SMS]
        Password[Username + Password\n+ CAPTCHA]
        APIKey[Bot API Key]
    end

    subgraph Tokens
        JWT[JWT Access Token\n+ Refresh Token]
        AdminJWT[Admin JWT]
        BotJWT[Bot JWT]
    end

    Customer --> OTP --> JWT
    Admin --> Password --> AdminJWT
    Bot --> APIKey --> BotJWT
```

---

## Customer Auth Flow (OTP)

```mermaid
sequenceDiagram
    participant User
    participant API as Jazebeh API
    participant SMS as SMS Provider

    User->>API: POST /auth/signup\n{mobile, name, ...}
    API->>SMS: Send OTP code
    SMS-->>User: OTP via SMS
    User->>API: POST /auth/verify\n{mobile, otp}
    API->>API: Validate OTP, create session
    API-->>User: Access token + Refresh token

    note over User,API: Login (existing user)
    User->>API: POST /auth/login/otp\n{mobile}
    API->>SMS: Send login OTP
    User->>API: POST /auth/login/otp/verify\n{mobile, otp}
    API-->>User: Access token + Refresh token
```

---

## Admin Auth Flow (CAPTCHA + Password)

```mermaid
sequenceDiagram
    participant Admin
    participant API as Jazebeh API

    Admin->>API: GET /admin/auth/captcha/init
    API-->>Admin: Captcha image + token
    Admin->>API: POST /admin/auth/login\n{username, password, captcha_token, captcha_answer}
    API->>API: Verify CAPTCHA (rotate captcha service)
    API->>API: Verify username + bcrypt password
    API-->>Admin: Admin JWT token
```

---

## JWT Token Structure

```mermaid
classDiagram
    class JWTClaims {
        string sub       // customer UUID or admin ID
        string role      // customer | admin | bot
        string issuer
        string audience
        datetime issuedAt
        datetime expiresAt
    }
```

- Supports **RSA** (asymmetric) or **HMAC** (symmetric) signing, configurable
- Access token TTL: configurable (short-lived)
- Refresh token TTL: configurable (long-lived)
- Stored in `CustomerSession` table per active session

---

## Authorization (RBAC)

```mermaid
flowchart TD
    Request[Incoming Request] --> AuthMiddleware[Auth Middleware\nValidate JWT]
    AuthMiddleware -->|Invalid| 401[401 Unauthorized]
    AuthMiddleware -->|Valid| RoleCheck{Role?}
    RoleCheck -->|customer| CustomerRoutes[Customer Routes]
    RoleCheck -->|admin| AdminAuthMiddleware[Admin Auth Middleware]
    RoleCheck -->|bot| BotAuthMiddleware[Bot Auth Middleware]
    AdminAuthMiddleware --> AuthzMiddleware[Authorization Middleware\nCheck Permission]
    AuthzMiddleware -->|Permitted| AdminRoutes[Admin Routes]
    AuthzMiddleware -->|Denied| 403[403 Forbidden]
    BotAuthMiddleware --> BotRoutes[Bot Routes]
```

Admin permissions are stored in a JSON field on the `Admin` entity and validated by `AuthorizationMiddleware` using the ACL registry.

---

## Maker-Checker (Access Control)

```mermaid
sequenceDiagram
    participant Maker as Admin (Maker)
    participant API as Jazebeh API
    participant Checker as Admin (Checker)

    Maker->>API: POST /admin/access-control/requests\n{action, resource, payload}
    API->>API: Create ACLChangeRequest (pending)
    Checker->>API: POST /admin/access-control/requests/:uuid/decision\n{approved: true/false}
    API->>API: Execute action if approved\nRecord audit log
```

Sensitive admin actions (e.g., bulk charge, pricing changes) require a second admin to approve.

---

## Security Layers

```mermaid
graph TB
    subgraph NetworkLayer["Network Layer"]
        TLS[TLS 1.2+ / HTTPS]
        RateLimit[Rate Limiting\nAPI: 2000/min · Auth: 20/min]
        IPBlock[IP Blocklist]
    end

    subgraph AppLayer["Application Layer"]
        Helmet[Security Headers\nHSTS · CSP · X-Frame-Options]
        CORS[CORS — Allowlist Origins]
        RequestID[Request ID Tracing]
        Compression[Response Compression]
        Recovery[Panic Recovery]
    end

    subgraph AuthLayer["Auth Layer"]
        JWT2[JWT Validation]
        RBAC[Role-Based Access Control]
        CAPTCHA[Admin CAPTCHA]
        OTP2[OTP — SMS Verification]
    end

    subgraph DataLayer["Data Layer"]
        BCrypt[Bcrypt Password Hashing]
        Tokenization[Identifier Tokenization]
        AuditLog[Audit Log — all sensitive actions]
        NoPersonalData[Behavioral data — anonymous / pseudonymous]
    end

    subgraph Observability
        Sentry[GlitchTip / Sentry — error tracking]
        Prometheus2[Prometheus — metrics]
        Grafana2[Grafana — dashboards]
    end

    NetworkLayer --> AppLayer --> AuthLayer --> DataLayer
    DataLayer --> Observability
```

---

## Audit Log

Every sensitive action produces an `AuditLog` entry with:
- `customerID` or `adminID`
- `action` (enum: signup, login, campaign_launch, payment, ...)
- `success` (bool)
- `ipAddress`
- `correlationID`
- `createdAt`

This provides a complete, immutable trail for compliance and investigation.

---

## Security Headers (Helmet)

| Header | Value |
|---|---|
| `X-XSS-Protection` | `1; mode=block` |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` |
| `Content-Security-Policy` | `default-src 'self'; ...` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Cross-Origin-Opener-Policy` | `same-origin` |
