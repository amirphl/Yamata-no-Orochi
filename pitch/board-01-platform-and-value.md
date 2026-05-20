# Jazebeh Platform — Business Overview for Investors

---

## The Problem We Solve

```mermaid
flowchart LR
    subgraph Old["Traditional Mass Sending"]
        direction TB
        A1[Brand buys a list\nof phone numbers]
        A2[Sends the same message\nto everyone]
        A3[Low CTR · High cost\nHigh unsubscribe rate]
        A1 --> A2 --> A3
    end

    subgraph New["Jazebeh — Behavioral Targeting"]
        direction TB
        B1[Platform learns behavior\nfrom real interactions]
        B2[Sends the right message\nto the right person\nat the right time]
        B3[Higher CTR · Lower waste\nBetter brand trust]
        B1 --> B2 --> B3
    end

    Old -->|"replaced by"| New
```

---

## How Value Is Created

```mermaid
flowchart TD
    subgraph Data["Data Layer"]
        D1[User clicks campaign link]
        D2[UTM tracker records behavior]
        D3[AI tags each user anonymously\nRecency · Frequency · Channel preference\nResponse to reminders]
    end

    subgraph Intelligence["Intelligence Layer"]
        I1[Behavioral Segments\n1000+ Tags per User]
        I2[Audience Matching\nfor each campaign]
        I3[Message · Timing · Channel\nRecommendations]
    end

    subgraph Platform["Platform Layer"]
        P1[Brand defines campaign\nin Jazebeh]
        P2[Platform sends across\nSMS · Bale · Rubika · Splus]
        P3[Campaign report:\nCTR · Delivery · Cost]
    end

    D1 --> D2 --> D3 --> I1 --> I2 --> I3
    I1 --> P1
    I3 --> P1
    P1 --> P2 --> P3
    P3 --> D1
```

---

## Who Uses the Platform

```mermaid
graph TB
    subgraph Users["Three User Roles"]
        Brand["Regular Brand / Company\n— defines campaigns\n— views reports\n— manages budget"]
        Agency["Marketing Agency\n— manages multiple brand accounts\n— earns commission\n— applies discounts"]
        Admin["Back-Office Admin\n— approves campaigns\n— manages pricing\n— handles support"]
    end

    subgraph Platform["Jazebeh"]
        CustomerPanel["Customer Panel"]
        BackOffice["Back-Office Panel"]
    end

    Brand --> CustomerPanel
    Agency --> CustomerPanel
    Admin --> BackOffice
```

---

## Platform Module Map

```mermaid
mindmap
    root(Jazebeh Platform)
        Customer Panel
            OTP Registration & Login
            Campaign Builder
                Audience Selection
                Message Composer
                Budget & Schedule
            Advanced Reporting
                Delivery & CTR
                Cost & Tax Breakdown
                Channel Performance
            Wallet & Billing
                Online Payment
                Crypto Payment
                Manual Deposit
                Invoice Management
            Support
                FAQ
                Tickets with SLA
            Agency Tools
                Sub-account Management
                Discount & Commission
        Back-Office Panel
            Customer & Agency Management
            Campaign Approval & Supervision
            Sending Line Management
            Pricing & Segment Factors
            Financial & Payment Management
            Reports & Analytics
            Support & SLA Monitoring
        Shared Infrastructure
            If/Else Automation
            Link Shortener
            UTM Click Tracker
            Multi-channel Dispatcher
```

---

## Revenue Model

```mermaid
flowchart LR
    Brand[Brand / Agency\ndeposits funds] --> Wallet[Platform Wallet]
    Wallet --> Cost[Campaign Cost\n= recipients × price per segment × channel]
    Cost --> Platform[Platform Revenue]
    Cost --> Tax[Tax]
    Agency[Marketing Agency] -->|earns commission on\nsub-account campaigns| Commission[Commission Income]
    Commission --> Agency
```

**Pricing levers:**
- Base price per channel (SMS / Bale / Rubika / Soroush Plus)
- Segment price factor — premium audiences cost more
- Page-based pricing for multi-page messages
- Agency discounts and coupon management

---

## Supported Channels

```mermaid
graph LR
    Platform[Jazebeh]
    Platform --> SMS[SMS\nPayamSMS]
    Platform --> Bale[Bale Messenger\ndomestic]
    Platform --> Rubika[Rubika\ndomestic]
    Platform --> Splus[Soroush Plus\ndomestic]

    SMS -->|bulk · OTP · transactional| Audience
    Bale -->|rich media| Audience
    Rubika -->|rich media| Audience
    Splus -->|direct API| Audience[Target Audience]
```

**Channel-independent layer:** if one channel fails, the platform automatically switches — route-switch in under 3 seconds.

---

## Competitive Differentiation

```mermaid
quadrantChart
    title Competitive Positioning
    x-axis "Mass Sending" --> "Precision Targeting"
    y-axis "Single Channel" --> "Multi-Channel"
    quadrant-1 Ideal Position
    quadrant-2 Multi-channel\nbut untargeted
    quadrant-3 Basic SMS\nproviders
    quadrant-4 Single channel\nAI tools
    Jazebeh: [0.85, 0.80]
    Traditional SMS: [0.15, 0.20]
    Email Marketing: [0.40, 0.30]
    Generic CRM: [0.50, 0.45]
```
