# Campaign Lifecycle & Multi-Channel Flow

## Campaign Lifecycle

```mermaid
stateDiagram-v2
    [*] --> Initiated : Customer creates campaign
    Initiated --> InProgress : System validates & calculates cost
    InProgress --> WaitingForApproval : Submitted for review
    WaitingForApproval --> Approved : Admin approves
    WaitingForApproval --> Rejected : Admin rejects
    Approved --> Running : Scheduler picks up campaign
    Running --> Executed : All messages sent
    Running --> Failed : Provider error

    Initiated --> Cancelled : Customer cancels
    InProgress --> Cancelled : Customer cancels
```

---

## End-to-End Campaign Execution

```mermaid
sequenceDiagram
    participant Customer
    participant Platform as Jazebeh Platform
    participant Admin
    participant Scheduler
    participant Channel as Messaging Channel
    participant Audience

    Customer->>Platform: Define campaign (message, audience, budget, channel)
    Platform->>Platform: Calculate cost & capacity
    Customer->>Platform: Confirm & submit
    Platform->>Admin: Notify for approval
    Admin->>Platform: Approve campaign
    Scheduler->>Platform: Poll for ready campaigns
    Platform->>Scheduler: Return approved campaign
    Scheduler->>Audience: Fetch target audience list
    Scheduler->>Platform: Generate short/tracking links
    Scheduler->>Channel: Send messages in batches
    Channel-->>Scheduler: Delivery status callbacks
    Scheduler->>Platform: Update delivery status
    Platform->>Customer: Campaign report available
```

---

## Multi-Channel Fallback Routing

```mermaid
flowchart TD
    Campaign[Campaign Ready] --> Primary[Primary Channel]
    Primary -->|Success| Delivered[Message Delivered]
    Primary -->|Channel Disruption| Switch{Auto Switch\nin < 3 sec}
    Switch --> Fallback[Fallback Channel]
    Fallback -->|Success| Delivered
    Fallback -->|Also Fails| Alert[Admin Alert & Report]
```

---

## Supported Channels

| Channel | Type | Notes |
|---|---|---|
| SMS | Text | Bulk, OTP, transactional |
| Bale | Messenger | Rich media, domestic |
| Rubika | Messenger | Rich media, domestic |
| Soroush Plus | Messenger | Direct API, domestic |
