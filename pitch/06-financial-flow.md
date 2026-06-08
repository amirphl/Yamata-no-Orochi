# Financial Flow — Wallet, Payments & Transactions

## Wallet Architecture

```mermaid
graph TB
    subgraph Wallets
        CW[Customer Wallet]
        SW[System Wallet]
        TW[Tax Wallet]
    end

    subgraph Funding
        Atipay[Atipay Payment Gateway]
        Deposit[Manual Deposit Receipt]
        AdminCharge[Admin Manual Charge]
    end

    subgraph Spending
        Campaign[Campaign Launch]
        Refund[Campaign Refund]
        Tax[Tax Deduction]
    end

    Atipay --> CW
    Deposit --> CW
    AdminCharge --> CW
    CW --> Campaign
    Campaign --> SW
    Campaign --> TW
    Campaign -.->|on cancel/fail| Refund
    Refund --> CW
```

---

## Online Payment Flow (Atipay)

```mermaid
sequenceDiagram
    participant Customer
    participant Platform as Jazebeh API
    participant Atipay

    Customer->>Platform: POST /payments/charge-wallet\n{amount, currency}
    Platform->>Platform: Create PaymentRequest record
    Platform->>Atipay: Create invoice (amount, callback_url)
    Atipay-->>Platform: Invoice URL + invoice_number
    Platform-->>Customer: Redirect URL
    Customer->>Atipay: Complete payment
    Atipay->>Platform: POST /payments/callback/:invoice_number
    Platform->>Platform: Verify payment signature
    Platform->>Platform: Credit wallet + record Transaction
    Platform->>Platform: Take BalanceSnapshot
    Platform-->>Atipay: 200 OK
```

---

## Deposit Receipt Flow (Manual Bank Transfer)

```mermaid
sequenceDiagram
    participant Customer
    participant Platform as Jazebeh API
    participant Admin

    Customer->>Platform: POST /payments/deposit-receipts\n{amount, file}
    Platform->>Platform: Store receipt record (pending)
    Platform-->>Customer: Receipt UUID
    Admin->>Platform: GET /admin/payments/deposit-receipts
    Admin->>Platform: POST /admin/payments/deposit-receipts/status\n{uuid, approved}
    Platform->>Platform: Credit wallet + Transaction
    Platform-->>Customer: Notification (SMS/email)
```

---

## Campaign Cost Calculation

```mermaid
flowchart TD
    A[Customer selects audience segment\nlevel1 + level3] --> B[Fetch SegmentPriceFactor\nfor platform + segment]
    B --> C[Fetch PlatformBasePrice\nfor channel]
    C --> D[Calculate: base_price × segment_factor × num_recipients]
    D --> E{Page-based pricing?}
    E -->|Yes| F[Fetch PagePrice\nmessage_length → page_count]
    F --> G[Total = page_cost × recipients]
    E -->|No| H[Total = unit_cost × recipients]
    G --> I[Preview shown to customer]
    H --> I
    I --> J[Customer confirms]
    J --> K[Wallet balance checked]
    K -->|Sufficient| L[Funds reserved\nCampaign → WaitingForApproval]
    K -->|Insufficient| M[Error: insufficient balance]
```

---

## Transaction Types

| Type | Direction | Description |
|---|---|---|
| `deposit` | + | Wallet top-up via payment gateway |
| `campaign_launch` | - | Funds reserved on campaign approval |
| `campaign_refund` | + | Funds returned on cancel/fail |
| `agency_commission` | + | Commission earned by agency |
| `tax` | - | Tax portion transferred to tax wallet |
| `admin_charge` | + | Manual admin credit |

---

## Agency Commission & Discount Flow

```mermaid
flowchart LR
    Agency[Marketing Agency] -->|invites| Customer[Customer signs up\nwith agency referral code]
    Customer -->|launches campaign| Campaign[Campaign Cost]
    Campaign -->|commission %| Agency
    Agency -->|applies| Discount[Agency Discount\nfor customer]
    Discount -->|reduces| Campaign
```

---

## Balance Snapshot

A `BalanceSnapshot` is recorded after every wallet-mutating operation (deposit, spend, refund). It captures:
- `balance` — main wallet balance
- `creditBalance` — bonus/credit balance
- `campaignBalance` — reserved for active campaigns
- `agencyBalance` — agency commission balance
- `takenAt` — UTC timestamp

This enables point-in-time balance reconciliation and audit.
