# Authentication Middleware

This middleware provides JWT token validation for protecting API endpoints.

## Features

- **JWT Token Validation**: Validates Bearer tokens from Authorization header
- **Error Handling**: Provides specific error codes for different failure scenarios
- **Context Injection**: Stores user information in request context for downstream handlers
- **Optional Authentication**: Supports endpoints that can work with or without authentication

## Usage

### 1. Initialize the Middleware

```go
import (
    "github.com/amirphl/Yamata-no-Orochi/app/middleware"
    "github.com/amirphl/Yamata-no-Orochi/app/services"
)

// Create token service
tokenService, err := services.NewTokenService(
    15*time.Minute,  // Access token TTL
    7*24*time.Hour,  // Refresh token TTL
    "your-app",      // Issuer
    "your-audience", // Audience
)

// Create auth middleware
authMiddleware := middleware.NewAuthMiddleware(tokenService)
```

### 2. Protect Routes with Authentication

```go
// Routes that require authentication
app.Post("/api/protected", authMiddleware.Authenticate(), yourHandler)
app.Get("/api/profile", authMiddleware.Authenticate(), profileHandler)
app.Put("/api/settings", authMiddleware.Authenticate(), settingsHandler)
```

### 3. Optional Authentication

```go
// Routes that can work with or without authentication
app.Get("/api/public-data", authMiddleware.OptionalAuth(), publicDataHandler)
```

### 4. Access User Information in Handlers

```go
func yourHandler(c fiber.Ctx) error {
    // Get customer ID from context
    customerID, exists := middleware.GetCustomerIDFromContext(c)
    if !exists {
        return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
    }
    
    // Get full token claims
    claims, exists := middleware.GetTokenClaimsFromContext(c)
    if exists {
        // Use claims.CustomerID, claims.TokenID, etc.
    }
    
    // Your handler logic here
    return c.JSON(fiber.Map{"message": "Protected data"})
}
```

## Error Responses

The middleware returns standardized error responses:

### Missing Authorization Header
```json
{
  "success": false,
  "message": "Authorization header is required",
  "error": {
    "code": "MISSING_AUTHORIZATION_HEADER"
  }
}
```

### Invalid Token Format
```json
{
  "success": false,
  "message": "Invalid authorization header format. Expected 'Bearer <token>'",
  "error": {
    "code": "INVALID_AUTHORIZATION_FORMAT"
  }
}
```

### Expired Token
```json
{
  "success": false,
  "message": "Access token has expired",
  "error": {
    "code": "TOKEN_EXPIRED"
  }
}
```

### Invalid Token
```json
{
  "success": false,
  "message": "Invalid access token",
  "error": {
    "code": "TOKEN_INVALID"
  }
}
```

### Revoked Token
```json
{
  "success": false,
  "message": "Access token has been revoked",
  "error": {
    "code": "TOKEN_REVOKED"
  }
}
```

## Helper Functions

### `GetCustomerIDFromContext(c fiber.Ctx) (uint, bool)`
Extracts the customer ID from the request context.

### `GetTokenClaimsFromContext(c fiber.Ctx) (*services.TokenClaims, bool)`
Extracts the full token claims from the request context.

### `RequireAuth(c fiber.Ctx) error`
Helper function to ensure authentication is required in handlers.

## Example Integration

```go
// In your main.go or router setup
func setupProtectedRoutes(app *fiber.App, authMiddleware *middleware.AuthMiddleware) {
    // Protected routes group
    protected := app.Group("/api", authMiddleware.Authenticate())
    
    protected.Get("/profile", profileHandler)
    protected.Put("/profile", updateProfileHandler)
    protected.Get("/settings", settingsHandler)
    protected.Post("/logout", logoutHandler)
}

func profileHandler(c fiber.Ctx) error {
    customerID, _ := middleware.GetCustomerIDFromContext(c)
    
    // Fetch user profile using customerID
    profile := getUserProfile(customerID)
    
    return c.JSON(fiber.Map{
        "success": true,
        "data": profile,
    })
}
```

## Security Notes

1. **Token Storage**: In production, implement proper token revocation using Redis or database
2. **HTTPS**: Always use HTTPS in production to protect tokens in transit
3. **Token Expiration**: Set appropriate token expiration times
4. **Rate Limiting**: Consider adding rate limiting to authentication endpoints
5. **Logging**: Log authentication failures for security monitoring 