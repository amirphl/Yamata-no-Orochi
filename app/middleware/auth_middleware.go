// Package middleware contains HTTP middleware functions for request processing
package middleware

import (
	"errors"
	"strings"

	"github.com/amirphl/Yamata-no-Orochi/app/services"
	"github.com/gofiber/fiber/v3"
)

// AuthMiddleware handles JWT token validation for protected endpoints
type AuthMiddleware struct {
	tokenService services.TokenService
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(tokenService services.TokenService) *AuthMiddleware {
	return &AuthMiddleware{
		tokenService: tokenService,
	}
}

// Authenticate is the middleware function that validates JWT tokens
func (m *AuthMiddleware) Authenticate() fiber.Handler {
	return func(c fiber.Ctx) error {
		// Get the Authorization header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": "Authorization header is required",
				"error": fiber.Map{
					"code": "MISSING_AUTHORIZATION_HEADER",
				},
			})
		}

		// Check if the header starts with "Bearer "
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": "Invalid authorization header format. Expected 'Bearer <token>'",
				"error": fiber.Map{
					"code": "INVALID_AUTHORIZATION_FORMAT",
				},
			})
		}

		// Extract the token (remove "Bearer " prefix)
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": "Access token is required",
				"error": fiber.Map{
					"code": "MISSING_ACCESS_TOKEN",
				},
			})
		}

		// Validate the token
		claims, err := m.tokenService.ValidateToken(token)
		if err != nil {
			var errorCode string
			var message string

			// Determine the specific error type
			if errors.Is(err, services.ErrTokenExpired) {
				errorCode = "TOKEN_EXPIRED"
				message = "Access token has expired"
			} else if errors.Is(err, services.ErrTokenInvalid) {
				errorCode = "TOKEN_INVALID"
				message = "Invalid access token"
			} else if errors.Is(err, services.ErrTokenRevoked) {
				errorCode = "TOKEN_REVOKED"
				message = "Access token has been revoked"
			} else {
				errorCode = "TOKEN_VALIDATION_FAILED"
				message = "Token validation failed"
			}

			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": message,
				"error": fiber.Map{
					"code": errorCode,
				},
			})
		}

		// Check if token is revoked
		if m.tokenService.IsTokenRevoked(token) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": "Access token has been revoked",
				"error": fiber.Map{
					"code": "TOKEN_REVOKED",
				},
			})
		}

		// Store user information in context for downstream handlers
		c.Locals("customer_id", claims.CustomerID)
		c.Locals("token_id", claims.TokenID)
		c.Locals("token_claims", claims)

		// Continue to the next handler
		return c.Next()
	}
}

// OptionalAuth is a middleware that validates JWT tokens if present, but doesn't require them
func (m *AuthMiddleware) OptionalAuth() fiber.Handler {
	return func(c fiber.Ctx) error {
		// Get the Authorization header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			// No authorization header, continue without authentication
			return c.Next()
		}

		// Check if the header starts with "Bearer "
		if !strings.HasPrefix(authHeader, "Bearer ") {
			// Invalid format, continue without authentication
			return c.Next()
		}

		// Extract the token (remove "Bearer " prefix)
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			// Empty token, continue without authentication
			return c.Next()
		}

		// Try to validate the token
		claims, err := m.tokenService.ValidateToken(token)
		if err != nil {
			// Token is invalid, but this is optional auth, so continue
			return c.Next()
		}

		// Check if token is revoked
		if m.tokenService.IsTokenRevoked(token) {
			// Token is revoked, but this is optional auth, so continue
			return c.Next()
		}

		// Store user information in context for downstream handlers
		c.Locals("customer_id", claims.CustomerID)
		c.Locals("token_id", claims.TokenID)
		c.Locals("token_claims", claims)

		// Continue to the next handler
		return c.Next()
	}
}

// GetCustomerIDFromContext extracts customer ID from the request context
func GetCustomerIDFromContext(c fiber.Ctx) (uint, bool) {
	customerID, ok := c.Locals("customer_id").(uint)
	return customerID, ok
}

// GetTokenClaimsFromContext extracts token claims from the request context
func GetTokenClaimsFromContext(c fiber.Ctx) (*services.TokenClaims, bool) {
	claims, ok := c.Locals("token_claims").(*services.TokenClaims)
	return claims, ok
}

// RequireAuth is a helper function that ensures authentication is required
func RequireAuth(c fiber.Ctx) error {
	customerID, exists := GetCustomerIDFromContext(c)
	if !exists {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "Authentication required",
			"error": fiber.Map{
				"code": "AUTHENTICATION_REQUIRED",
			},
		})
	}

	// Check if customer ID is valid
	if customerID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "Invalid customer ID",
			"error": fiber.Map{
				"code": "INVALID_CUSTOMER_ID",
			},
		})
	}

	return nil
}
