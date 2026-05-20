# Short Links & UTM Tracking

## Overview

The short link / UTM tracker converts long campaign URLs into unique per-recipient short links. When a recipient clicks, the platform records the event anonymously and redirects to the original URL. This feeds into the behavioral profiling pipeline.

---

## Short Link Lifecycle

```mermaid
sequenceDiagram
    participant Bot as Campaign Bot
    participant API as Jazebeh API
    participant DB as PostgreSQL
    participant Recipient
    participant OriginalURL as Destination URL

    Bot->>API: POST /bot/short-links\n[{original_url, customer_uid, campaign_uuid, scenario_id}]
    API->>DB: Insert short_links records\n(uid = unique hash)
    API-->>Bot: [{uid, short_url}]
    Bot->>Bot: Embed short URLs in campaign messages
    Bot->>Recipient: Send message with short URL

    Recipient->>API: GET /s/:uid  (or /:uid)
    API->>DB: Record ShortLinkClick\n(uid, campaign_uuid, customer_uid, scenario_id, clicked_at)
    API-->>Recipient: 302 Redirect → original URL
    Recipient->>OriginalURL: Load destination
```

---

## Short Link Entity

```mermaid
classDiagram
    class ShortLink {
        UUID uuid
        string uid          // unique 6-8 char code
        string originalURL
        string campaignUUID
        string customerUID  // pseudonymous identifier
        string scenarioID
        string scenarioName
        datetime createdAt
    }

    class ShortLinkClick {
        int id
        string uid
        string campaignUUID
        string customerUID
        string scenarioID
        string scenarioName
        datetime clickedAt
    }

    ShortLink "1" --> "*" ShortLinkClick : registered_by
```

---

## Allocation Model

```mermaid
flowchart TD
    Upload[Admin uploads CSV\nwith pre-generated UIDs] --> Pool[(Short Link Pool\nin DB)]
    Bot -->|POST /bot/short-links/allocate| Pool
    Pool --> Assigned[Assign UIDs to campaign recipients]
    Assigned --> Messages[Embedded in outgoing messages]
```

For large campaigns, UIDs are pre-allocated from a pool. The bot requests a bulk allocation and receives a reserved set of UIDs mapped to recipient identifiers.

---

## Click Denormalization

Click records are **denormalized** — the `short_link_clicks` table stores `campaign_uuid`, `customer_uid`, `scenario_id`, and `scenario_name` directly (copied from the short link at insert time). This allows efficient analytics queries without joining `short_links` on every click lookup.

Migration `0053_denormalize_short_link_clicks.sql` introduced this optimization, backfilled by `0054`.

---

## UTM Tracking → Behavioral Pipeline

```mermaid
flowchart LR
    Click[ShortLinkClick Event\ncampaign + customer + scenario + timestamp] --> EventStore[(Event Store)]
    EventStore --> FeatureEng[Feature Engineering\nRecency · Frequency · Channel · Scenario Response]
    FeatureEng --> Clustering[Clustering\nK-Means / DBSCAN]
    Clustering --> Tags[Behavioral Tags\nassigned to audience_profile]
    Tags --> FutureTargeting[Used in future campaign targeting]
```

---

## Admin Reporting (Short Links)

| Endpoint | Output |
|---|---|
| `POST /admin/short-links/download` | Excel: all links for a scenario |
| `POST /admin/short-links/download-with-clicks` | Excel: links + click counts |
| `POST /admin/short-links/download-with-clicks-range` | Excel: links + clicks in date range |
| `POST /admin/short-links/download-with-clicks-by-scenario-name` | Excel: by scenario name |

---

## Privacy Design

- `customerUID` is a **pseudonymous** identifier — not a real name or phone number
- Click data is linked to campaigns and scenarios, not to personally identifiable fields
- No IP address or user-agent is stored in `short_link_clicks` (removed in migration `0046`)
- Fully compliant with the platform's anonymous behavioral data policy
