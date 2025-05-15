# Adapters Package

This package provides adapter functions to bridge different layers of the application, specifically converting between handler DTOs and business flow DTOs.

## Purpose

The adapter pattern is used to solve the problem of incompatible interfaces between different layers of the application. In our case, the handlers use their own DTO types while the business flows use different DTO types. The adapters provide a clean way to convert between these types without modifying the core business logic.

## Components

### HandlerSignupFlowAdapter

Converts handler signup DTOs to business flow DTOs and vice versa.

**Methods:**
- `InitiateSignup(ctx, req)`: Converts handler signup request to business flow request
- `VerifyOTP(ctx, req)`: Converts handler OTP verification request to business flow request
- `ResendOTP(ctx, customerID, otpType)`: Delegates OTP resend to business flow

### HandlerLoginFlowAdapter

Converts handler login DTOs to business flow DTOs and vice versa.

**Methods:**
- `Login(ctx, req, ipAddress, userAgent)`: Converts handler login request to business flow request
- `ForgotPassword(ctx, req, ipAddress, userAgent)`: Converts handler forgot password request to business flow request
- `ResetPassword(ctx, req, ipAddress, userAgent)`: Converts handler reset password request to business flow request

## Usage

```go
import "github.com/amirphl/Yamata-no-Orochi/app/adapters"

// Create business flows
signupFlow := businessflow.NewSignupFlow(...)
loginFlow := businessflow.NewLoginFlow(...)

// Create adapters
signupFlowAdapter := adapters.NewHandlerSignupFlowAdapter(signupFlow)
loginFlowAdapter := adapters.NewHandlerLoginFlowAdapter(loginFlow)

// Use adapters with handlers
authHandler := handlers.NewAuthHandler(signupFlowAdapter, loginFlowAdapter)
```

## Field Mapping

### Signup Request Mapping

| Handler Field | Business Flow Field | Notes |
|---------------|-------------------|-------|
| `AccountType` | `AccountType` | Direct mapping |
| `CompanyName` | `CompanyName` | Direct mapping |
| `NationalID` | `NationalID` | Direct mapping |
| `CompanyPhone` | `CompanyPhone` | Direct mapping |
| `CompanyAddress` | `CompanyAddress` | Direct mapping |
| `CompanyPostalCode` | `PostalCode` | Field name conversion |
| `RepresentativeFirstName` | `RepresentativeFirstName` | Direct mapping |
| `RepresentativeLastName` | `RepresentativeLastName` | Direct mapping |
| `RepresentativeMobile` | `RepresentativeMobile` | Direct mapping |
| `Email` | `Email` | Direct mapping |
| `Password` | `Password` | Direct mapping |
| `ConfirmPassword` | `ConfirmPassword` | Direct mapping |
| `ReferrerAgencyRefererCode` | `ReferrerAgencyCode` | Field name conversion |

### OTP Verification Mapping

| Handler Field | Business Flow Field | Notes |
|---------------|-------------------|-------|
| `CustomerID` | `CustomerID` | Direct mapping |
| `OTPCode` | `OTPCode` | Direct mapping |
| `OTPType` | `OTPType` | Direct mapping |

### Login Request Mapping

| Handler Field | Business Flow Field | Notes |
|---------------|-------------------|-------|
| `Identifier` | `Identifier` | Direct mapping |
| `Password` | `Password` | Direct mapping |

## Benefits

1. **Separation of Concerns**: Handlers and business flows can use their own DTOs
2. **Maintainability**: Changes to DTOs in one layer don't affect the other
3. **Testability**: Each layer can be tested independently
4. **Flexibility**: Easy to add new fields or modify existing ones
5. **Type Safety**: Compile-time checking of field mappings

## Error Handling

The adapters preserve error handling from the business flows:
- Business flow errors are returned as-is
- No additional error wrapping is performed
- Context is preserved for proper error tracking

## Performance

The adapters are lightweight and perform minimal operations:
- Field-by-field copying
- No deep copying of complex structures
- No network calls or I/O operations
- Memory allocation is minimal

## Future Extensions

The adapter pattern can be extended to support:
- Additional business flows (e.g., profile management, settings)
- Different response formats (e.g., GraphQL, gRPC)
- Caching layers
- Validation transformations
- Logging and monitoring hooks 