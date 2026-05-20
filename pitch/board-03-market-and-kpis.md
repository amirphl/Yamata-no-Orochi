# Market Coverage & Key Performance Indicators

---

## Market Expansion: 4 → 30 Business Categories

```mermaid
flowchart LR
    subgraph Done1404["Completed — Year 1404"]
        C1[Financial & Investment]
        C2[Cosmetics]
        C3[Health & Beauty]
        C4[Clothing & Fashion]
    end

    subgraph Planned1405["Planned — Year 1405"]
        P1[Food & Restaurants]
        P2[Multi-category Stores]
        P3[Travel & Tourism]
        P4[Real Estate]
        P5[Automotive]
        P6[Education]
        P7[Healthcare]
        P8[+ 23 more\nhigh-demand categories]
    end

    Done1404 -->|expand to| Planned1405
```

---

## Category Coverage Growth

```mermaid
xychart-beta
    title "Business Category Coverage"
    x-axis ["Year 1404\nCompleted", "Year 1405\nTarget"]
    y-axis "Number of Categories" 0 --> 35
    bar [4, 30]
```

Each category includes:
- Operational behavioral segment outputs (tagged audience profiles)
- Ready-to-use campaign playbook (message templates, timing, frequency, channels)

---

## Behavioral Tag Growth

```mermaid
xychart-beta
    title "Behavioral Tag List Growth"
    x-axis ["1404 — v1\nDelivered", "1405 — v2\nTarget"]
    y-axis "Number of Tags" 0 --> 2000
    bar [1000, 1500]
```

Tags v2 adds:
- Low-impact tags removed
- Use cases clarified per real campaign scenario
- Cross-industry tags for multi-sector stability

---

## Quality KPI Targets

```mermaid
graph LR
    subgraph Clustering["Clustering Quality"]
        KPI1["Silhouette Score\nTarget ≥ 0.45"]
        KPI2["Labeling Coverage\nTarget ≥ 60% of valid data"]
    end

    subgraph PredictionModels["Prediction Models"]
        KPI3["AUC Score\nTarget ≥ 0.75"]
        KPI4["Calibration Error\nBrier Score — improved"]
    end

    subgraph Performance["Campaign Performance"]
        KPI5["CTR / LTR Improvement\nvs. baseline: ≥ 2×"]
        KPI6["Recommendation Response\nMVP: < 4 sec | Advanced: < 500 ms"]
    end

    subgraph SystemReliability["System Reliability"]
        KPI7["Platform Availability\n≥ 99% in pilot"]
        KPI8["Drift Detection Alert\n< 15 minutes"]
        KPI9["Channel Failover\n< 3 seconds"]
    end
```

---

## KPI Dashboard (Current vs. Target)

| KPI | Category | Current (1404) | Target (1405) |
|---|---|---|---|
| Business categories covered | Market | 4 | 30 |
| Behavioral tags | AI | 1,000+ | 1,500+ |
| Clustering Silhouette score | AI | Established | ≥ 0.45 |
| Tag labeling coverage | AI | Established | ≥ 60% |
| CTR/LTR improvement vs. baseline | AI | Measured | ≥ 2× |
| Prediction AUC | AI | — | ≥ 0.75 |
| Recommendation response time | Platform | — | < 500 ms |
| Platform availability | Platform | Pilot | ≥ 99% |
| Drift detection alert | Platform | — | < 15 min |
| Channel failover time | Platform | — | < 3 sec |
| Active campaign flows (future) | Platform | — | ≥ 1,000 |
| Campaign creation time (Chatbot) | UX | Baseline set | Reduced vs. manual |

---

## Customer Journey: Brand to Results

```mermaid
journey
    title A Brand's Journey on Jazebeh
    section Onboarding
        Register with OTP: 5: Brand
        Top up wallet: 4: Brand
        Configure channel settings: 4: Brand
    section Campaign Creation
        Select audience segment: 5: Brand
        Compose message: 5: Brand
        Review cost estimate: 5: Brand
        Submit for approval: 4: Brand, Admin
        Admin approves: 5: Admin
    section Execution
        Platform sends across channels: 5: Platform
        Tracking links record clicks: 5: Platform
        Delivery status updated: 5: Platform
    section Results
        View campaign report (CTR, delivery): 5: Brand
        Export results: 4: Brand
        Behavioral data feeds next campaign: 5: Platform
```

---

## Agency Business Model

```mermaid
flowchart TD
    Agency[Marketing Agency\n— signs up as partner]
    Agency -->|invites brands via referral code| Brand1[Brand A]
    Agency -->|invites brands via referral code| Brand2[Brand B]
    Agency -->|invites brands via referral code| Brand3[Brand C ...]

    Brand1 --> Campaign1[Launches campaigns]
    Brand2 --> Campaign2[Launches campaigns]
    Brand3 --> Campaign3[Launches campaigns]

    Campaign1 -->|commission| Agency
    Campaign2 -->|commission| Agency
    Campaign3 -->|commission| Agency

    Agency -->|applies discounts for clients| Brand1
    Agency -->|views sub-account reports| Dashboard[Agency Dashboard]
```

Agencies grow their business by bringing more brands onto the platform — Jazebeh scales through the agency network.

---

## The Flywheel Effect

```mermaid
flowchart LR
    MoreBrands[More Brands\njoin platform] -->|generate| MoreCampaigns[More Campaigns\n& Interactions]
    MoreCampaigns -->|feed| BetterAI[Better Behavioral\nProfiles & Tags]
    BetterAI -->|enable| BetterTargeting[Better Targeting\n& Recommendations]
    BetterTargeting -->|delivers| BetterResults[Higher CTR\nLower Cost per Result]
    BetterResults -->|attract| MoreBrands
```

The platform improves with scale: more campaigns → richer behavioral data → smarter targeting → better results → more brands.
