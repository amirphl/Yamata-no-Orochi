# 1405 Investment Case — What We're Asking For

---

## What Approval Unlocks

```mermaid
flowchart TD
    Approval[1405 Budget Approved] --> A & B & C & D

    A[Smarter AI\nPrediction models update\nContinuous learning\nTags v2]

    B[Better Product\nFlow designer\nA/B testing\nChatbot assistant]

    C[Bigger Market\n4 → 30 business categories\nIndustry playbooks]

    D[Greater Scale\nReal-time data pipeline\nPerformance optimization\nAPI integrations]

    A & B & C & D --> Outcome[Commercially ready\nbehavioral marketing platform]
```

---

## Eight Initiatives — One-Line Summary

```mermaid
graph LR
    I1["1 — AI Learning\nModels improve from live data"] --> Value1["More accurate audience\ntargeting over time"]
    I2["2 — Flow Builder & A/B\nVisual campaign journeys"] --> Value2["Brands run smarter\nself-serve campaigns"]
    I3["3 — Data Pipeline\nReal-time events"] --> Value3["Reports update live\nmodels stay fresh"]
    I4["4 — 30 Categories\nMarket expansion"] --> Value4["Addressable market\ngrows 7.5×"]
    I5["5 — Funnel Reporting\nConversion attribution"] --> Value5["Brands see full ROI\nnot just clicks"]
    I6["6 — UX & Ops\nEasier to use"] --> Value6["Lower churn\nhigher NPS"]
    I7["7 — APIs & SDK\nIntegration layer"] --> Value7["Brands plug in\ntheir own systems"]
    I8["8 — Chatbot\nConversational campaign builder"] --> Value8["10× faster\ncampaign creation"]
```

---

## Market Size Step-Up

```mermaid
xychart-beta
    title "Addressable Behavioral Segments by Category Count"
    x-axis ["1404 Delivered\n(4 categories)", "1405 Target\n(30 categories)"]
    y-axis "Relative Market Coverage" 0 --> 100
    bar [13, 100]
```

Expanding from 4 to 30 categories means reaching **brands in food, travel, real estate, automotive, education, healthcare**, and more — the bulk of the Iranian consumer economy.

---

## The Business Case in Three Numbers

```mermaid
graph TD
    subgraph Number1["2×"]
        N1["Minimum improvement\nin CTR vs. untargeted sending\n(demonstrated in 1404 pilots)"]
    end

    subgraph Number2["30"]
        N2["Business categories\ncovered after 1405\n(vs. 4 today)"]
    end

    subgraph Number3["1,000+"]
        N3["Behavioral tags\nalready built and validated\n(v1, delivered in 1404)"]
    end

    Number1 --- Number2 --- Number3
```

---

## Risk Mitigation

| Risk | Mitigation |
|---|---|
| AI model accuracy below target | Continuous learning loop with drift alerts; human review before deployment |
| Channel provider disruption | Automatic failover across 4 channels in < 3 seconds |
| Data quality degradation | Lineage tracking + automated quality gates in the pipeline |
| Scale / performance | Queue-based sending, Redis caching, horizontal scaling ready |
| Privacy / compliance | Purely pseudonymous data; opt-in/opt-out enforced; full audit log |
| Chatbot producing bad recommendations | Content compliance filter; user always edits before launch |

---

## Summary: What 1404 Proved, What 1405 Will Build On

```mermaid
flowchart LR
    subgraph Proven["Proven in 1404"]
        P1[✅ Platform works at pilot scale]
        P2[✅ 4 channels sending reliably]
        P3[✅ Behavioral tagging works]
        P4[✅ CTR improves with targeting]
        P5[✅ Agencies & brands use it]
    end

    subgraph Next["1405 Builds On This"]
        N1[🔧 Models learn continuously]
        N2[🔧 30 markets, not 4]
        N3[🔧 Visual campaign builder]
        N4[🔧 Chatbot reduces friction]
        N5[🔧 Full funnel attribution]
    end

    Proven --> Next --> Commercial[Commercially scalable\nbehavioral marketing platform]
```
