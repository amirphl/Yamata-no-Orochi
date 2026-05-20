# Background Schedulers — Campaign Execution

## Scheduler Architecture

```mermaid
graph TB
    subgraph App["Go Application"]
        Main[main.go\ninitializeApplication]

        subgraph Schedulers["Campaign Schedulers — one per channel"]
            SMS[SMS Scheduler\nPayamSMS]
            Bale[Bale Scheduler\nBale Messenger]
            Rubika[Rubika Scheduler\nRubika Messenger]
            Splus[Soroush Plus Scheduler]
        end
    end

    subgraph Database["PostgreSQL"]
        Campaigns[(campaigns)]
        AudienceProfiles[(audience_profiles)]
        Tags[(tags)]
        ProcessedCampaigns[(processed_campaigns)]
        SentMessages[(sent_sms / sent_bale /\nsent_rubika / sent_splus)]
        StatusJobs[(campaign_status_jobs)]
        StatusResults[(sms_status_results\nbale_status_results etc.)]
    end

    subgraph ExternalChannels["Messaging Providers"]
        PayamSMS[PayamSMS API]
        BaleAPI[Bale Bot API]
        RubikaAPI[Rubika API]
        SplusAPI[Soroush Plus API]
    end

    Main --> SMS
    Main --> Bale
    Main --> Rubika
    Main --> Splus

    SMS --> Campaigns
    SMS --> AudienceProfiles
    SMS --> Tags
    SMS --> ProcessedCampaigns
    SMS --> SentMessages
    SMS --> StatusJobs
    SMS --> StatusResults
    SMS --> PayamSMS

    Bale --> BaleAPI
    Rubika --> RubikaAPI
    Splus --> SplusAPI
```

---

## Campaign Execution Loop

```mermaid
flowchart TD
    Tick[Scheduler Tick\nevery CampaignExecutionInterval] --> FetchApproved[Fetch Approved Campaigns\nfrom DB]
    FetchApproved --> ForEach{For each campaign}
    ForEach --> CheckSchedule{Scheduled time\nreached?}
    CheckSchedule -->|No| Skip[Skip — check next tick]
    CheckSchedule -->|Yes| FetchAudience[Fetch Audience Profiles\nfrom audience_profiles]
    FetchAudience --> BuildPhoneList[Build Phone / UID List\nApply Tags & Segment Filters]
    BuildPhoneList --> GenerateTracking[Generate Tracking IDs\n16-char hex, sequence counter]
    GenerateTracking --> SendBatches[Send Messages in Batches\nwith configurable delay between batches]
    SendBatches --> RecordSent[Record to sent_* table\nper message]
    RecordSent --> MarkRunning[Mark Campaign → Running\nin DB]
    MarkRunning --> ScheduleStatusJobs[Create CampaignStatusJobs\nfor delivery status polling]
    ScheduleStatusJobs --> WaitForStatus[Status Polling Loop\nfetch delivery callbacks]
    WaitForStatus --> UpdateStats[Update ProcessedCampaign\nstatistics: delivered, failed, etc.]
    UpdateStats --> MarkExecuted[Mark Campaign → Executed]
```

---

## Status Polling Loop

```mermaid
sequenceDiagram
    participant Scheduler
    participant Provider as Messaging Provider
    participant DB as PostgreSQL

    Scheduler->>DB: Fetch pending CampaignStatusJobs\n(batch of 100)
    loop For each job
        Scheduler->>Provider: Query delivery status\n(tracking_id or message_id)
        Provider-->>Scheduler: Delivery status response
        Scheduler->>DB: Insert *StatusResult record
        Scheduler->>DB: Update sent_* record status
    end
    Scheduler->>DB: Update ProcessedCampaign statistics\n(delivered, failed, pending counts)
    Note over Scheduler: Repeat every statusJobWorkerInterval (5 min)
```

---

## Message Send Delay Strategy

```mermaid
flowchart LR
    Batch1[Batch 1\nN messages] --> Delay1[Delay: MessageSendDelay\n configurable ms]
    Delay1 --> Batch2[Batch 2\nN messages]
    Delay2[Delay: MessageSendDelay] --> Batch3[Batch 3\n...]
    Batch2 --> Delay2
```

- Prevents provider rate-limit rejections
- `MessageSendDelay` is configurable per deployment environment
- Batch size is channel-dependent (tuned per provider throughput)

---

## Channel Clients

| Scheduler | Client | Provider Type |
|---|---|---|
| `CampaignScheduler` (SMS) | `PayamSMSClient` | REST API — PayamSMS |
| `BaleCampaignScheduler` | `BaleClient` | Bot API — Bale Messenger |
| `RubikaCampaignScheduler` | Bot client | Bot API — Rubika |
| `SplusCampaignScheduler` | `SplusClient` | REST API — Soroush Plus |

Each client is initialized with per-channel credentials and timeouts from config.

---

## Audience Cache

```mermaid
flowchart TD
    Scheduler -->|first fetch| CacheCheck{Redis cache hit?}
    CacheCheck -->|Yes| ReturnCached[Return cached phone list]
    CacheCheck -->|No| DBFetch[Query audience_profiles\nwith segment + tag filters]
    DBFetch --> StoreCache[Store result in Redis\nwith TTL]
    StoreCache --> ReturnCached
```

Audience phone lists are cached in Redis to avoid repeated DB scans across scheduler ticks and retries.

---

## Graceful Stop

Each scheduler exposes a `Start(ctx) stopFunc` pattern:
- `Start()` launches goroutines for the send loop and status polling loop
- The returned `stopFunc` cancels the context, signaling goroutines to finish the current batch and exit cleanly
- Called automatically on `SIGTERM`/`SIGINT` in `main.go`
