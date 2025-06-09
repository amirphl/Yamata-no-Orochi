# Database Migrations for Yamata no Orochi

This directory contains database migrations for the customer signup and authentication system.

## Migration Structure

Each migration is numbered sequentially starting with `0001` and includes both up and down migration files:

- `000X_migration_name.sql` - Up migration (applies changes)
- `000X_migration_name_down.sql` - Down migration (rollback changes)

## Migration Order

1. **0001_create_account_types** - Creates account type enum and reference table
2. **0002_create_customers** - Creates main customers table with all user types
3. **0003_create_otp_verifications** - Creates OTP verification system for signup
4. **0004_create_customer_sessions** - Creates session management for authentication
5. **0005_create_audit_log** - Creates audit logging for security tracking
6. **0006_update_customer_fields** - Updates customer field sizes and mobile format
7. **0007_add_missing_audit_actions** - Adds missing audit action enum values
8. **0008_update_audit_log_success_field** - Updates audit log success field type
9. **0009_add_correlation_ids** - Adds correlation ID fields for request tracking
10. **0010_add_customer_uuid_and_agency_id** - Adds UUID and agency ID fields
11. **0011_add_new_audit_actions** - Adds new audit action enum values
12. **0012_update_timestamp_defaults_to_utc** - Updates timestamp defaults to UTC
13. **0013_relax_name_validation** - Removes name format constraints to allow any characters
14. **0014_create_sms_campaigns** - Creates SMS campaign management system
15. **0015_add_sms_campaign_audit_actions** - Adds audit actions for SMS campaign operations
16. **0016_add_comment_to_sms_campaigns** - Adds comment field for admin rejection notes
17. **0017_create_wallet_models** - Creates immutable accounting system for wallet, transactions, and payments
18. **0018_create_agency_commission_models** - Creates agency commission tracking and distribution system

## Database Schema Overview

### Account Types
- `individual` - Personal accounts
- `independent_company` - Business accounts
- `marketing_agency` - Agency accounts that can manage other companies

### Key Features

#### Customers Table
- Unified table for all user types (individuals, companies, agencies)
- Conditional field requirements based on account type
- Built-in validation constraints for Iranian phone numbers, national IDs, etc.
- Agency referral system support
- Email and mobile verification tracking

#### OTP Verification
- Secure 6-digit OTP system
- Attempt limiting and expiration
- Support for both email and mobile verification
- IP and user agent tracking for security

#### Session Management
- JWT-compatible session tokens
- Refresh token support
- Device and location tracking
- Automatic session expiration

#### Audit Logging
- Comprehensive action tracking
- Security event monitoring
- JSONB metadata for flexible data storage
- Request correlation support

#### Wallet & Payment System
- Immutable accounting system with correlation IDs
- Balance snapshots for audit trail
- Atipay payment integration
- Transaction lifecycle tracking
- Support for freeze/unfreeze, lock/unlock operations

## Field Validation Rules

### Common Fields (All Account Types)
- Representative First Name: Any characters, ≤ 255 characters
- Representative Last Name: Any characters, ≤ 255 characters  
- Representative Mobile: Format `+989xxxxxxxxx`, unique
- Email: RFC-compliant format, unique
- Password: ≥ 8 characters, 1 uppercase + 1 number

### Company Fields (Independent Company & Marketing Agency)
- Company Name: Max 60 characters
- National ID: Atleast 10 digits
- Company Phone: Min 10 characters, various formats allowed
- Company Address: Max 255 characters
- Postal Code: Exactly 10 digits

### Agency Referral
- Optional for individuals and independent companies
- Must reference existing marketing agency
- Cannot be changed after signup

## Running Migrations

### Apply All Migrations (Up)
```sql
-- Run in order:
\i migrations/0001_create_account_types.sql
\i migrations/0002_create_customers.sql
\i migrations/0003_create_otp_verifications.sql
\i migrations/0004_create_customer_sessions.sql
\i migrations/0005_create_audit_log.sql
\i migrations/0006_update_customer_fields.sql
\i migrations/0007_add_missing_audit_actions.sql
\i migrations/0008_update_audit_log_success_field.sql
\i migrations/0009_add_correlation_ids.sql
\i migrations/0010_add_customer_uuid_and_agency_id.sql
\i migrations/0011_add_new_audit_actions.sql
\i migrations/0012_update_timestamp_defaults_to_utc.sql
\i migrations/0013_relax_name_validation.sql
\i migrations/0014_create_sms_campaigns.sql
\i migrations/0015_add_sms_campaign_audit_actions.sql
\i migrations/0016_add_comment_to_sms_campaigns.sql
\i migrations/0017_create_wallet_models.sql
\i migrations/0018_create_agency_commission_models.sql
```

### Rollback All Migrations (Down)
```sql
-- Run in reverse order:
\i migrations/0018_create_agency_commission_models_down.sql
\i migrations/0017_create_wallet_models_down.sql
\i migrations/0016_add_comment_to_sms_campaigns_down.sql
\i migrations/0015_add_sms_campaign_audit_actions_down.sql
\i migrations/0014_create_sms_campaigns_down.sql
\i migrations/0013_relax_name_validation_down.sql
\i migrations/0012_update_timestamp_defaults_to_utc_down.sql
\i migrations/0011_add_new_audit_actions_down.sql
\i migrations/0010_add_customer_uuid_and_agency_id_down.sql
\i migrations/0009_add_correlation_ids_down.sql
\i migrations/0008_update_audit_log_success_field_down.sql
\i migrations/0007_add_missing_audit_actions_down.sql
\i migrations/0006_update_customer_fields_down.sql
\i migrations/0005_create_audit_log_down.sql
\i migrations/0004_create_customer_sessions_down.sql
\i migrations/0003_create_otp_verifications_down.sql
\i migrations/0002_create_customers_down.sql
\i migrations/0001_create_account_types_down.sql
```

### Individual Migration Control
```sql
-- Apply specific migration
\i migrations/0003_create_otp_verifications.sql

-- Rollback specific migration
\i migrations/0003_create_otp_verifications_down.sql
```

## Performance Considerations

All tables include comprehensive indexing:

- **Primary keys** for fast lookups
- **Unique constraints** on emails and mobile numbers
- **Foreign key indexes** for join performance
- **Composite indexes** for common query patterns
- **Partial indexes** for conditional fields
- **GIN indexes** for JSONB metadata queries

## Security Features

- Password hashing (application-level)
- OTP attempt limiting
- Session token length requirements
- IP address tracking
- Audit logging for all critical actions
- Constraint validation at database level

## Sample Usage

### Create Individual Account
```sql
INSERT INTO customers (
    account_type_id, 
    representative_first_name, 
    representative_last_name, 
    representative_mobile, 
    email, 
    password_hash
) VALUES (
    (SELECT id FROM account_types WHERE type_name = 'individual'),
    'John',
    'Doe', 
    '+989123456789',
    'john.doe@example.com',
    '$2b$12$...' -- bcrypt hash
);
```

### Create Company with Agency Referral
```sql
INSERT INTO customers (
    account_type_id,
    company_name,
    national_id,
    company_phone,
    company_address,
    postal_code,
    representative_first_name,
    representative_last_name,
    representative_mobile,
    email,
    password_hash,
    referrer_agency_id
) VALUES (
    (SELECT id FROM account_types WHERE type_name = 'independent_company'),
    'Tech Company Ltd',
    '12345678901',
    '02112345678',
    '123 Business St, Tehran',
    '1234567890',
    'Jane',
    'Smith',
    '+989987654321', 
    'jane@techcompany.com',
    '$2b$12$...',
    (SELECT id FROM customers WHERE account_type_id = (SELECT id FROM account_types WHERE type_name = 'marketing_agency') LIMIT 1)
);
```

## Notes

- All timestamps use `TIMESTAMP WITH TIME ZONE` for proper timezone handling
- JSONB fields provide flexibility for future feature expansion
- Constraints ensure data integrity at the database level
- Foreign key cascades handle proper cleanup on deletions 