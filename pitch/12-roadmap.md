# Roadmap — Completed & Planned

## Completed (Year 1404)

```mermaid
gantt
    title Year 1404 Deliverables
    dateFormat YYYY-MM
    axisFormat %b

    section Data & AI
    Behavioral event schema & data model     :done, 2024-01, 2024-03
    Data cleaning & integration (campaigns)  :done, 2024-02, 2024-04
    Feature engineering (RFV, time/channel)  :done, 2024-03, 2024-06
    Clustering v1 — K-Means / DBSCAN         :done, 2024-04, 2024-07
    Behavioral tag list v1 — 1000+ tags      :done, 2024-05, 2024-09

    section Platform
    Customer panel (OTP, campaigns, billing) :done, 2024-03, 2024-08
    Back-office panel                        :done, 2024-04, 2024-09
    If/Else automation + UTM tracker         :done, 2024-06, 2024-10
    Multi-channel (SMS, Bale, Rubika, Splus) :done, 2024-07, 2024-11

    section Documentation
    Architecture & API docs                  :done, 2024-09, 2024-12
    Ops runbook                              :done, 2024-10, 2024-12
    Pilot case studies                       :done, 2024-11, 2024-12
```

---

## Planned (Year 1405)

```mermaid
mindmap
    root(1405 Roadmap)
        AI & Analysis
            Interaction prediction models
            Continuous learning & drift monitoring
            Behavioral tags v2 — enrich & prune
            Cross-industry stability assessment
        Platform
            Visual campaign flow designer
            A/B testing — auto winner selection
            Message / timing / channel recommender
            Multi-channel fallback routing improvements
        Data & Scalability
            Real-time event pipeline
            Data lineage & quality control
            Performance dashboards & alerts
            Queue / cache / load distribution
        Business Domains
            Expand from 4 to 30 industry segments
            Industry-specific campaign playbooks
        Reporting
            Full funnel reporting to conversion
            Simple attribution model
        UX & Operations
            Easier campaign builder UX
            Stable test environment for agencies
        Integrations
            Import/export APIs
            Lightweight web/mobile SDK
        Jazebeh Guide Chatbot
            Brand onboarding conversation
            Automatic message & channel recommendation
            Conversation → flow export
            Compliance & audit logging
```

---

## Current Status Summary

| Area | Status |
|---|---|
| Customer Panel | Stable pilot |
| Back-Office Panel | Stable pilot |
| SMS Channel | Production |
| Bale Messenger | Production |
| Rubika Messenger | Production |
| Soroush Plus | Production |
| Behavioral Tags v1 | Complete (1000+ tags) |
| Clustering v1 | Complete |
| UTM / Short Link Tracker | Production |
| Online Payments (Atipay) | Production |
| Manual Deposit Receipts | Production |
| Agency Sub-account System | Production |
| Support Tickets | Production |
| Prometheus / Grafana | Production |
| GlitchTip Error Tracking | Production |
| Interaction Prediction Models | Planned (1405) |
| Flow Designer | Planned (1405) |
| A/B Testing | Planned (1405) |
| Jazebeh Guide Chatbot | Planned (1405) |

---

## Quality Targets

| Metric | Target | Notes |
|---|---|---|
| Platform availability | ≥ 99% | Pilot period |
| Clustering Silhouette score | ≥ 0.45 | v1 models |
| Behavioral tag coverage | ≥ 60% of valid data | v1 |
| CTR/LTR improvement vs baseline | ≥ 2× | Pilot campaigns |
| Prediction AUC (future) | ≥ 0.75 | v2 models |
| Drift detection alert latency | < 15 min | Real-time path |
| Recommendation response time (MVP) | < 4 sec | |
| Recommendation response time (advanced) | < 500 ms | |
| Campaign flow change application | < 1 sec | Future |
| Channel failover time | < 3 sec | Future |
