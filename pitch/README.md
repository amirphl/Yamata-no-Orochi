# Jazebeh Platform — Documentation Index

This directory contains board-facing and technical architecture documents for the Jazebeh (Yamata-no-Orochi) platform. The documents explain product intent and system design; runtime code and migration manifests remain authoritative for exact implementation details.

All diagrams use [Mermaid](https://mermaid.js.org/) and render natively in GitHub, GitLab, and most Markdown viewers.

---

## Board & Investor Documents

*High-level, business-focused — no technical depth required.*

| File | Contents |
|---|---|
| [board-01-platform-and-value.md](board-01-platform-and-value.md) | Problem/solution, value chain, platform map, revenue model, competitive positioning |
| [board-02-milestones.md](board-02-milestones.md) | 1404 delivery timeline, commitments vs. actuals, 1405 roadmap, maturity scores |
| [board-03-market-and-kpis.md](board-03-market-and-kpis.md) | Market expansion (4→30 categories), KPI targets, customer journey, agency model, flywheel |
| [board-04-ai-value.md](board-04-ai-value.md) | How AI works (non-technical), recommendation engine, chatbot, privacy design, model scorecard |
| [board-05-next-year-asks.md](board-05-next-year-asks.md) | 1405 investment case, initiative summaries, business case in 3 numbers, risk mitigation |

---

## Technical Documents

*Detailed architecture, data models, and engineering reference.*

| # | File | Contents |
|---|---|---|
| 01 | [01-architecture.md](01-architecture.md) | Overall system architecture, user roles, data & event flow |
| 02 | [02-campaign-flow.md](02-campaign-flow.md) | Campaign lifecycle, end-to-end execution, multi-channel fallback |
| 03 | [03-ai-pipeline.md](03-ai-pipeline.md) | Behavioral profiling pipeline, continuous learning, tag coverage |
| 04 | [04-data-model.md](04-data-model.md) | ER diagram, core entities, financial model, auth model |
| 05 | [05-deployment.md](05-deployment.md) | Infrastructure, Docker containers, networking, graceful shutdown |
| 06 | [06-financial-flow.md](06-financial-flow.md) | Wallet, online payments, deposit receipts, transactions |
| 07 | [07-auth-security.md](07-auth-security.md) | Authentication flows, RBAC, maker-checker, security headers |
| 08 | [08-api-reference.md](08-api-reference.md) | Complete API endpoint reference by role |
| 09 | [09-scheduler.md](09-scheduler.md) | Background campaign schedulers, status polling, audience cache |
| 10 | [10-observability.md](10-observability.md) | Prometheus metrics, error tracking, structured logging |
| 11 | [11-short-links-utm.md](11-short-links-utm.md) | Short link system, UTM click tracking, privacy design |
| 12 | [12-roadmap.md](12-roadmap.md) | Completed deliverables (1404) and planned work (1405) |
| 13 | [13-pentest-handoff.md](13-pentest-handoff.md) | Pentest scope, exposed surfaces, trust boundaries, prerequisites |
| 14 | [14-bundles-and-scoring.md](14-bundles-and-scoring.md) | Bundle audience uniqueness, test-vs-execution selection, scoring behavior |

---

## Current Implementation Delta

The current backend schema head is `0119`. In addition to the flows described across these documents, the running code now includes:

- Authenticated bundle create, list, get, and update endpoints.
- Queued bundle tag-evaluation requests, current evaluation status, and paginated tag-score results.
- A bounded-concurrency smart-tag scheduler using an OpenAI-compatible Responses API for persona analysis and batched tag scoring.
- Persisted evaluation configuration/prompt snapshots, events, attempts, raw responses, validation outcomes, and bundle-fit scores.
- Platform-scoped campaign status jobs for SMS, Bale, Rubika, and Soroush Plus.

For endpoint truth, see [`app/router/routes.go`](../app/router/routes.go). For the current schema and known aggregate-manifest issues, see the [migration README](../migrations/README.md). For smart-tag configuration, see [`env.template`](../env.template) and the [root README](../README.md).

---

## Quick Reference

**Supported channels**: SMS · Bale · Rubika · Soroush Plus

**Customer account types**: Individual · Independent Company · Marketing Agency

**Authenticated principals**: Customer · Admin (Back-Office) · Bot (Internal)

**Payment methods**: Online (Atipay) · Manual Deposit Receipt · Configured Crypto Provider

**Tech stack**: Go 1.26 · Fiber v3 · PostgreSQL 15 · Redis 8 · GORM · JWT · Prometheus · Grafana · GlitchTip · Nginx · Docker
