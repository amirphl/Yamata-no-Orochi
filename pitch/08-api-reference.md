# API Reference — Endpoint Summary

Base path: `/api/v1`

All protected endpoints require `Authorization: Bearer <token>` unless stated otherwise.

---

## Auth — Customer (`/auth`)

> Rate limit: 20 req/min per IP

| Method | Path | Description | Auth |
|---|---|---|---|
| POST | `/auth/signup` | Register new customer | Public |
| POST | `/auth/verify` | Verify OTP after signup | Public |
| POST | `/auth/resend-otp` | Resend signup OTP | Public |
| POST | `/auth/login` | Password login | Public |
| POST | `/auth/login/otp` | Request OTP for login | Public |
| POST | `/auth/login/otp/verify` | Verify OTP and receive tokens | Public |
| POST | `/auth/forgot-password` | Initiate password reset | Public |
| POST | `/auth/reset` | Reset password with token | Public |

---

## Auth — Admin (`/admin/auth`)

> Rate limit: 20 req/min per IP

| Method | Path | Description | Auth |
|---|---|---|---|
| GET | `/admin/auth/captcha/init` | Get CAPTCHA for admin login | Public |
| POST | `/admin/auth/login` | Admin login (username + password + CAPTCHA) | Public |
| POST | `/admin/auth/login/verify-otp` | Complete admin login with OTP | Public |

---

## Auth — Bot (`/bot/auth`)

| Method | Path | Description | Auth |
|---|---|---|---|
| POST | `/bot/auth/login` | Bot API key exchange for JWT | Public |

---

## Public Platform Endpoints

| Method | Path | Description | Auth |
|---|---|---|---|
| GET | `/health` | Reverse-proxy health endpoint backed by `GET /api/v1/health` | Public |
| GET | `/s/:uid` | Redirect short link (click tracking) | Public |
| GET | `/:uid` | Redirect short link (root) | Public |
| GET | `/s/tst:uid` | Redirect test short link | Public |
| GET | `/tst:uid` | Redirect test short link | Public |

---

## Campaigns — Customer (`/campaigns`)

| Method | Path | Description |
|---|---|---|
| POST | `/campaigns` | Create new campaign |
| PUT | `/campaigns/:uuid` | Update draft campaign |
| GET | `/campaigns` | List customer's campaigns |
| POST | `/campaigns/:uuid/clone` | Clone an existing campaign |
| POST | `/campaigns/:uuid/test-send` | Send test message |
| POST | `/campaigns/calculate-capacity` | Estimate audience size |
| POST | `/campaigns/calculate-cost` | Estimate campaign cost |
| POST | `/campaigns/calculate-cost-v2` | Estimate cost (v2, page-based) |
| GET | `/campaigns/page-prices` | Get page pricing table |
| GET | `/campaigns/audience-spec` | List available audience segments |
| GET | `/campaigns/summary` | Active/running campaign summary |
| GET | `/campaigns/initiated/last` | Get last initiated (draft) campaign |
| GET | `/campaigns/:id/export` | Export campaign report (Excel) |
| POST | `/campaigns/:id/cancel` | Cancel a campaign |

---

## Campaigns — Admin (`/admin/campaigns`)

| Method | Path | Description |
|---|---|---|
| GET | `/admin/campaigns` | List all campaigns |
| GET | `/admin/campaigns/page-prices` | Get page pricing |
| GET | `/admin/campaigns/:id` | Get campaign detail |
| POST | `/admin/campaigns/approve` | Approve a campaign |
| POST | `/admin/campaigns/reject` | Reject a campaign |
| POST | `/admin/campaigns/reschedule` | Reschedule a campaign |
| POST | `/admin/campaigns/cancel` | Cancel a campaign |
| DELETE | `/admin/campaigns/audience-spec` | Remove an audience spec |
| PUT | `/admin/campaigns/page-prices` | Update page pricing |

---

## Campaigns — Bot (`/bot/campaigns`)

| Method | Path | Description |
|---|---|---|
| GET | `/bot/campaigns/ready` | Fetch approved campaigns ready to send |
| POST | `/bot/campaigns/audience-spec` | Upload computed audience spec |
| POST | `/bot/campaigns/audience-spec/reset` | Reset audience spec |
| POST | `/bot/campaigns/:id/executed` | Mark campaign as executed |
| POST | `/bot/campaigns/:id/running` | Mark campaign as running |
| POST | `/bot/campaigns/:id/statistics` | Update delivery statistics |
| GET | `/bot/campaigns/:id/target-audience-excel-file` | Download audience Excel |

---

## Payments — Customer

| Method | Path | Description |
|---|---|---|
| GET | `/wallet/balance` | Get wallet balance |
| POST | `/payments/charge-wallet` | Initiate online payment |
| POST | `/payments/callback/:invoice_number` | Payment gateway callback (Public) |
| GET | `/payments/history` | Transaction history |
| POST | `/payments/deposit-receipts` | Submit manual deposit receipt |
| GET | `/payments/deposit-receipts` | List deposit receipts |
| GET | `/payments/deposit-receipts/:uuid/file` | Download receipt file |
| PUT | `/payments/deposit-receipts/:uuid/file` | Update receipt file |
| DELETE | `/payments/deposit-receipts/:uuid/file` | Delete receipt file |
| POST | `/payments/transactions/invoice-issue-request` | Request invoice for transaction |
| GET | `/payments/proforma/preview` | Preview proforma invoice |
| GET | `/payments/proforma/preview-by-amount` | Preview proforma by amount |

---

## Payments — Admin

| Method | Path | Description |
|---|---|---|
| POST | `/admin/payments/charge-wallet` | Admin manual wallet credit |
| GET | `/admin/payments/transactions` | List all transactions |
| GET | `/admin/payments/deposit-receipts` | List all deposit receipts |
| GET | `/admin/payments/deposit-receipts/:uuid/file` | Download receipt file |
| POST | `/admin/payments/deposit-receipts/status` | Approve/reject deposit receipt |
| POST | `/admin/payments/transactions/invoice` | Attach invoice to transaction |

---

## Crypto Payments

| Method | Path | Description |
|---|---|---|
| POST | `/crypto/payments/request` | Create crypto payment request |
| GET | `/crypto/payments/:uuid/status` | Get payment status |
| POST | `/crypto/payments/verify` | Manual verification |
| POST | `/crypto/providers/:platform/callback` | Provider webhook (Public) |

---

## Agency / Sub-accounts (`/reports/agency`)

| Method | Path | Description |
|---|---|---|
| GET | `/reports/agency/customers` | Agency customer summary |
| GET | `/reports/agency/customers/list` | List agency sub-accounts |
| GET | `/reports/agency/discounts/active` | Active agency discounts |
| GET | `/reports/agency/customers/:id/discounts` | Customer discount history |
| POST | `/reports/agency/discounts` | Create agency discount |

---

## Line Numbers

| Method | Path | Description |
|---|---|---|
| GET | `/line-numbers/active` | List active SMS sending lines |
| GET | `/admin/line-numbers` | List all line numbers |
| POST | `/admin/line-numbers` | Create line number |
| PUT | `/admin/line-numbers` | Batch update line numbers |
| GET | `/admin/line-numbers/report` | Line number usage report |

---

## Multimedia (`/media`)

| Method | Path | Description |
|---|---|---|
| POST | `/media/upload` | Upload file (customer) |
| GET | `/media/:uuid` | Download file |
| GET | `/media/:uuid/preview` | Preview file |
| POST | `/admin/media/upload` | Upload file (admin) |
| GET | `/bot/media/:uuid` | Download file (bot) |

---

## Short Links

| Method | Path | Description | Auth |
|---|---|---|---|
| POST | `/bot/short-links` | Bulk create short links | Bot |
| POST | `/bot/short-links/one` | Create single short link | Bot |
| POST | `/bot/short-links/allocate` | Allocate short link pool | Bot |
| POST | `/admin/short-links/upload-csv` | Upload short link CSV | Admin |
| POST | `/admin/short-links/download` | Download links by scenario | Admin |
| POST | `/admin/short-links/download-with-clicks` | Download with click data | Admin |
| POST | `/admin/short-links/download-with-clicks-range` | Download with date range | Admin |
| POST | `/admin/short-links/download-with-clicks-by-scenario-name` | Download by scenario name | Admin |

---

## Tickets (`/tickets`)

| Method | Path | Description |
|---|---|---|
| POST | `/tickets` | Create support ticket |
| POST | `/tickets/reply` | Reply to ticket (customer) |
| GET | `/tickets` | List tickets |
| GET | `/tickets/:id/attachments/:index` | Download attachment |
| POST | `/admin/tickets/reply` | Admin reply to ticket |
| GET | `/admin/tickets` | Admin list all tickets |

---

## Platform Settings

| Method | Path | Description |
|---|---|---|
| POST | `/platform-settings` | Create channel settings |
| GET | `/platform-settings` | List customer's channel configs |
| GET | `/admin/platform-settings` | Admin list all |
| PUT | `/admin/platform-settings/status` | Change status |
| PUT | `/admin/platform-settings/metadata` | Add metadata |

---

## Pricing

| Method | Path | Description |
|---|---|---|
| GET | `/platform-base-prices` | List base prices (customer) |
| GET | `/admin/platform-base-prices` | Admin list |
| PUT | `/admin/platform-base-prices` | Admin update |
| GET | `/admin/segment-price-factors` | List segment factors |
| GET | `/admin/segment-price-factors/level3-options` | List level3 options |
| POST | `/admin/segment-price-factors` | Create factor |
| GET | `/segment-price-factors` | List latest factors (customer) |

---

## Access Control & Profile

| Method | Path | Description |
|---|---|---|
| GET | `/profile` | Get current customer profile |
| POST | `/admin/access-control/requests` | Create maker-checker request |
| POST | `/admin/access-control/requests/:uuid/decision` | Approve/reject request |

---

## Customer Management — Admin

| Method | Path | Description |
|---|---|---|
| GET | `/admin/customer-management` | List customers |
| GET | `/admin/customer-management/shares` | Customer share report |
| GET | `/admin/customer-management/:id` | Customer detail + campaigns |
| POST | `/admin/customer-management/active-status` | Enable/disable customer |
| GET | `/admin/customer-management/:id/discounts` | Customer discount history |

---

## System

| Method | Path | Description | Auth |
|---|---|---|---|
| GET | `/api/v1/health` | Health check | Public |
