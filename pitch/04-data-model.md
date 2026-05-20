# Data Model — Core Entities

## Entity Relationship Overview

```mermaid
erDiagram
    CUSTOMER ||--o{ CAMPAIGN : creates
    CUSTOMER ||--o{ WALLET : owns
    CUSTOMER ||--o{ TICKET : submits
    CUSTOMER ||--o{ PLATFORM_SETTINGS : configures
    CUSTOMER ||--|| ACCOUNT_TYPE : has

    WALLET ||--o{ TRANSACTION : records
    WALLET ||--o{ BALANCE_SNAPSHOT : snapshots

    CAMPAIGN ||--o{ PROCESSED_CAMPAIGN : produces
    CAMPAIGN ||--o{ CAMPAIGN_STATUS_JOB : schedules
    CAMPAIGN ||--o{ SHORT_LINK : generates

    PROCESSED_CAMPAIGN ||--o{ SENT_SMS : tracks
    PROCESSED_CAMPAIGN ||--o{ SENT_BALE_MESSAGE : tracks
    PROCESSED_CAMPAIGN ||--o{ SENT_RUBIKA_MESSAGE : tracks
    PROCESSED_CAMPAIGN ||--o{ SENT_SPLUS_MESSAGE : tracks

    SENT_SMS ||--o{ SMS_STATUS_RESULT : receives
    SENT_BALE_MESSAGE ||--o{ BALE_STATUS_RESULT : receives
    SENT_RUBIKA_MESSAGE ||--o{ RUBIKA_STATUS_RESULT : receives
    SENT_SPLUS_MESSAGE ||--o{ SPLUS_STATUS_RESULT : receives

    SHORT_LINK ||--o{ SHORT_LINK_CLICK : registers

    AUDIENCE_PROFILE }|--o{ CAMPAIGN : targets
    TAG }|--o{ AUDIENCE_PROFILE : labels

    PAYMENT_REQUEST ||--|| CUSTOMER : belongs_to
    DEPOSIT_RECEIPT ||--|| CUSTOMER : submitted_by

    ADMIN ||--o{ ACL_CHANGE_REQUEST : initiates
    BOT ||--o{ CAMPAIGN : executes
```

---

## Campaign Entity

```mermaid
classDiagram
    class Campaign {
        UUID uuid
        string platform
        CampaignStatus status
        string messageText
        string audienceLevel1
        string audienceLevel3
        datetime scheduledAt
        int numTarget
        decimal totalCost
        string lineNumber
        bool approved
        datetime createdAt
    }

    class CampaignStatus {
        <<enumeration>>
        initiated
        in-progress
        waiting-for-approval
        approved
        running
        executed
        expired
        rejected
        cancelled
        cancelled-by-admin
    }

    class CampaignPlatform {
        <<enumeration>>
        sms
        bale
        rubika
        splus
    }

    Campaign --> CampaignStatus
    Campaign --> CampaignPlatform
```

---

## Financial Entities

```mermaid
classDiagram
    class Wallet {
        UUID uuid
        int customerID
        decimal balance
        decimal creditBalance
        JSON metadata
    }

    class Transaction {
        UUID uuid
        int walletID
        string type
        decimal amount
        string description
        JSON metadata
        datetime createdAt
    }

    class BalanceSnapshot {
        int walletID
        decimal balance
        decimal creditBalance
        decimal campaignBalance
        decimal agencyBalance
        datetime takenAt
    }

    class PaymentRequest {
        UUID uuid
        int customerID
        decimal amount
        string status
        string invoiceNumber
        string lang
        datetime createdAt
    }

    class DepositReceipt {
        UUID uuid
        int customerID
        decimal amount
        string status
        string invoiceNumber
        string fileUUID
        datetime createdAt
    }

    Wallet "1" --> "*" Transaction
    Wallet "1" --> "*" BalanceSnapshot
    PaymentRequest --> Wallet
    DepositReceipt --> Wallet
```

---

## Audience & Tagging

```mermaid
classDiagram
    class AudienceProfile {
        UUID uuid
        string phone
        string uid
        string code
        string level1Segment
        string level3Segment
        JSON behavioralTags
        datetime createdAt
    }

    class Tag {
        int id
        string name
        string category
        string description
        datetime createdAt
    }

    class AudienceSelection {
        int campaignID
        int selectionID
        string[] matchedUIDs
        string[] unmatchedUIDs
        datetime createdAt
    }

    AudienceProfile "*" --> "*" Tag : labeled_by
    AudienceSelection --> AudienceProfile
```

---

## Account & Auth Entities

```mermaid
classDiagram
    class Customer {
        UUID uuid
        int accountTypeID
        string firstName
        string lastName
        string mobile
        string email
        string passwordHash
        string agencyRefererCode
        string shebaNumber
        bool isActive
        bool isEmailVerified
        bool isMobileVerified
        datetime createdAt
    }

    class AccountType {
        int id
        string typeName
    }

    class CustomerSession {
        UUID uuid
        int customerID
        string accessToken
        string refreshToken
        datetime expiresAt
        datetime updatedAt
    }

    class Admin {
        int id
        string username
        string passwordHash
        JSON permissions
        datetime createdAt
    }

    class Bot {
        int id
        string name
        string apiKey
        datetime createdAt
    }

    Customer --> AccountType
    Customer "1" --> "*" CustomerSession
```

---

## Key Account Types

| Type | Description |
|---|---|
| `MarketingAgency` | Partner agency — manages sub-accounts, earns commission |
| `Individual` | Regular brand/company user |

---

## Messaging Sent-Message Entities

| Entity | Platform | Key Fields |
|---|---|---|
| `SentSMS` | SMS | trackingID, phone, status, providerRef |
| `SentBaleMessage` | Bale | recipientID, messageID, status |
| `SentRubikaMessage` | Rubika | recipientID, messageID, status |
| `SentSplusMessage` | Soroush Plus | recipientID, messageID, status |

Each has a corresponding `*StatusResult` table that stores provider delivery callbacks.

---

## Short Link & UTM Tracking

```mermaid
classDiagram
    class ShortLink {
        UUID uuid
        string uid
        string originalURL
        string campaignUUID
        string customerUID
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

    ShortLink "1" --> "*" ShortLinkClick
```
