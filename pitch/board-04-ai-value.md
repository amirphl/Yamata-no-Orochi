# AI & Behavioral Intelligence — Board Summary

---

## What the AI Does (Simply)

```mermaid
flowchart TD
    subgraph Observe["1 — Observe"]
        O1[Every campaign link is unique\nper user per campaign]
        O2[When someone clicks,\nthe platform records it — anonymously]
    end

    subgraph Learn["2 — Learn"]
        L1[Patterns emerge:\nwho clicks · when · on which channel\nhow often · how fast]
        L2[AI groups users into\nbehavioral clusters]
        L3[Each user receives\nbehavioral tags]
    end

    subgraph Apply["3 — Apply"]
        A1[Next campaign:\nshow only relevant audience segments]
        A2[Platform suggests\nbest message tone · best send time · best channel]
        A3[Result: higher engagement\nlower waste · better ROI]
    end

    Observe --> Learn --> Apply --> Observe
```

---

## From Raw Data to Actionable Tags

```mermaid
flowchart LR
    subgraph Raw["Raw Signal"]
        S1[Click]
        S2[View]
        S3[Sign-up]
        S4[Purchase]
        S5[Cancellation]
    end

    subgraph Features["Features Extracted"]
        F1[Recency\nwhen was last interaction?]
        F2[Frequency\nhow often?]
        F3[Value\nhow engaged?]
        F4[Time preference\nmorning / evening / weekend]
        F5[Channel preference\nSMS vs. messenger]
        F6[Response to reminders]
    end

    subgraph Tags["Behavioral Tags Applied"]
        T1["high_frequency_buyer"]
        T2["evening_engager"]
        T3["sms_preferred"]
        T4["fashion_interested"]
        T5["reminder_responsive"]
        T6["1,000+ other tags"]
    end

    Raw --> Features --> Tags
```

---

## Behavioral Tag Coverage — Four Pilot Categories

```mermaid
pie title Tag Coverage Across 1404 Pilot Categories
    "Financial & Investment" : 25
    "Cosmetics" : 25
    "Health & Beauty" : 25
    "Clothing & Fashion" : 25
```

Each category has its own segment outputs and campaign playbook.

---

## The Recommendation Engine (1405)

```mermaid
flowchart TD
    Brand[Brand prepares a campaign] --> Recommender[Recommendation Engine]

    subgraph Inputs["Recommender Inputs"]
        I1[Brand's product category]
        I2[Target audience behavioral profile]
        I3[Historical campaign results]
        I4[Current season / time context]
    end

    Inputs --> Recommender

    subgraph Outputs["Ready-to-Use Suggestions"]
        O1[Message text options\nin multiple tones]
        O2[Best send time\nday + hour]
        O3[Best channel\nSMS · Bale · Rubika · Splus]
        O4[Recommended audience tags]
        O5[Suggested campaign path]
    end

    Recommender --> Outputs
    Outputs -->|editable by brand| Campaign[Campaign launched]
```

Response time: MVP < 4 seconds · Advanced version < 500 ms

---

## The Jazebeh Guide Chatbot (1405)

```mermaid
sequenceDiagram
    participant Brand
    participant Chatbot as Jazebeh Guide Chatbot

    Brand->>Chatbot: "I sell skincare products.\nBudget: 5M Toman.\nGoal: drive sign-ups."
    Chatbot-->>Brand: Suggests audience segment tags\n(health_beauty, skin_care_interested)
    Chatbot-->>Brand: Suggests 3 message options\n(informative / promotional / reminder tone)
    Chatbot-->>Brand: Recommends channel: Bale + SMS fallback
    Chatbot-->>Brand: Best send time: Tuesday evening
    Brand->>Chatbot: "Use the promotional tone"
    Chatbot-->>Brand: Campaign flow ready →\ntransfer to campaign builder
    Brand->>Chatbot: Confirms
    Note over Brand,Chatbot: Campaign is live — all from conversation
```

---

## Privacy by Design

```mermaid
flowchart LR
    subgraph Real["What the Platform Sees"]
        R1[Pseudonymous UID\nnot a phone number]
        R2[Campaign UUID]
        R3[Click timestamp]
        R4[Scenario / channel]
    end

    subgraph NeverStored["What Is Never Stored"]
        N1[Real name]
        N2[IP address]
        N3[Browser fingerprint]
        N4[Personally identifiable info]
    end

    subgraph Output["What Is Produced"]
        O1[Anonymous behavioral tags]
        O2[Cluster membership]
        O3[Engagement patterns]
    end

    Real --> Output
    NeverStored -.->|excluded| Output
```

- All behavioral analysis is **anonymous and pseudonymous**
- No personal data enters the AI pipeline
- Audit log for every chatbot conversation (compliance)
- Opt-in/opt-out preference center enforced at send time

---

## Model Quality Scorecard

| Model | Metric | Target |
|---|---|---|
| Clustering (K-Means / DBSCAN) | Silhouette score | ≥ 0.45 |
| Behavioral labeling | Coverage of valid data | ≥ 60% |
| CTR / LTR prediction | AUC | ≥ 0.75 |
| Calibration | Brier score | Improved vs. baseline |
| Campaign uplift | CTR vs. non-targeted baseline | ≥ 2× |
| Drift detection | Alert latency | < 15 minutes |
| Model update | Service continuity | Zero downtime |
| Data schema consistency | Error rate | ≤ 2% |
| Historical data field coverage | Standard field coverage | ≥ 90% |
