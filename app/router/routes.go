// Package router provides HTTP routing, middleware configuration, and server setup for the web application
package router

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cache"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

// Router interface for HTTP routing
type Router interface {
	SetupRoutes()
	Start(address string) error
	GetApp() *fiber.App
}

// FiberRouter implements Router using Fiber v3
type FiberRouter struct {
	app         *fiber.App
	authHandler AuthHandler
}

// AuthHandler interface for authentication endpoints
type AuthHandler interface {
	Signup(c fiber.Ctx) error
	VerifyOTP(c fiber.Ctx) error
	ResendOTP(c fiber.Ctx) error
	Login(c fiber.Ctx) error
	ForgotPassword(c fiber.Ctx) error
	ResetPassword(c fiber.Ctx) error
}

// NewFiberRouter creates a new Fiber router
func NewFiberRouter(authHandler AuthHandler) Router {
	// Configure Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "Yamata no Orochi API",
		ServerHeader: "Yamata-no-Orochi",
		ErrorHandler: errorHandler,
		BodyLimit:    4 * 1024 * 1024, // 4MB
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
		JSONEncoder:  json.Marshal,
		JSONDecoder:  json.Unmarshal,
	})

	return &FiberRouter{
		app:         app,
		authHandler: authHandler,
	}
}

// SetupRoutes configures all application routes
func (r *FiberRouter) SetupRoutes() {
	log.Println("Setting up routes...")

	// Global middleware
	r.setupMiddleware()

	// API routes
	api := r.app.Group("/api/v1")

	// Health check route (no rate limiting)
	api.Get("/health", r.healthCheck)

	// Auth routes with rate limiting
	auth := api.Group("/auth")

	// Apply rate limiting to auth endpoints
	auth.Use(limiter.New(limiter.Config{
		Max:        30,              // Maximum 10 requests
		Expiration: 1 * time.Minute, // Per minute
		KeyGenerator: func(c fiber.Ctx) string {
			return c.IP() // Rate limit by IP
		},
		LimitReached: func(c fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   "rate_limit_exceeded",
				"message": "Too many requests. Please try again later.",
			})
		},
	}))

	// Auth endpoints
	auth.Post("/signup", r.authHandler.Signup)
	auth.Post("/verify", r.authHandler.VerifyOTP)
	auth.Post("/resend-otp/:customer_id", r.authHandler.ResendOTP)
	auth.Post("/login", r.authHandler.Login)
	auth.Post("/forgot-password", r.authHandler.ForgotPassword)
	auth.Post("/reset", r.authHandler.ResetPassword)

	// API documentation route
	api.Get("/docs", r.getAPIDocumentation)

	// Not found handler
	r.app.Use(r.notFoundHandler)

	log.Println("Routes configured successfully")
}

// SetupMiddleware configures global middleware
func (r *FiberRouter) setupMiddleware() {
	// Request ID middleware - must be first
	r.app.Use(requestid.New(requestid.Config{
		Header: "X-Request-ID",
		Generator: func() string {
			return generateRequestID()
		},
	}))

	// Security headers middleware
	r.app.Use(helmet.New(helmet.Config{
		XSSProtection:             "1; mode=block",
		ContentTypeNosniff:        "nosniff",
		XFrameOptions:             "DENY",
		HSTSMaxAge:                31536000, // 1 year
		HSTSExcludeSubdomains:     false,
		ContentSecurityPolicy:     "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' https:; connect-src 'self' https:; frame-ancestors 'none';",
		ReferrerPolicy:            "strict-origin-when-cross-origin",
		CrossOriginEmbedderPolicy: "require-corp",
		CrossOriginOpenerPolicy:   "same-origin",
		CrossOriginResourcePolicy: "cross-origin",
		OriginAgentCluster:        "?1",
		XDNSPrefetchControl:       "off",
		XDownloadOptions:          "noopen",
		XPermittedCrossDomain:     "none",
	}))

	// CORS middleware with production settings
	r.app.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"https://yamata-no-orochi.com",
			"https://api.yamata-no-orochi.com",
			"https://admin.yamata-no-orochi.com",
			"https://app.yamata-no-orochi.com",
			"https://*.j0in.ir",
		},
		AllowMethods: []string{
			"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS",
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
			"X-Requested-With",
			"X-Request-ID",
			"X-API-Key",
			"Cache-Control",
		},
		ExposeHeaders: []string{
			"X-Request-ID",
			"X-Response-Time",
		},
		AllowCredentials: true,
		MaxAge:           86400, // 24 hours
	}))

	// Compression middleware for performance
	r.app.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
		Next: func(c fiber.Ctx) bool {
			// Skip compression for certain content types
			contentType := c.Get("Content-Type")
			return contains(contentType, "image/") ||
				contains(contentType, "video/") ||
				contains(contentType, "audio/")
		},
	}))

	// Cache middleware for static content
	r.app.Use(cache.New(cache.Config{
		Next: func(c fiber.Ctx) bool {
			// Only cache GET requests to specific endpoints
			return c.Method() != "GET" ||
				!contains(c.Path(), "/health") &&
					!contains(c.Path(), "/docs")
		},
		Expiration:   30 * time.Minute,
		CacheControl: true,
	}))

	// Advanced logging middleware
	r.app.Use(logger.New(logger.Config{
		Format:     `{"time":"${time}","pid":"${pid}","request_id":"${locals:requestid}","level":"info","method":"${method}","path":"${path}","protocol":"${protocol}","ip":"${ip}","user_agent":"${ua}","status":${status},"latency":"${latency}","bytes_in":${bytesReceived},"bytes_out":${bytesSent},"referer":"${referer}"}` + "\n",
		TimeFormat: time.RFC3339,
		TimeZone:   "UTC",
		Next: func(c fiber.Ctx) bool {
			// Skip logging for health checks in production
			return c.Path() == "/api/v1/health"
		},
	}))

	// Custom security middleware
	r.app.Use(r.securityMiddleware)

	// API key validation middleware (optional)
	r.app.Use(r.apiKeyMiddleware)

	// Recovery middleware with custom error handling
	r.app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c fiber.Ctx, e interface{}) {
			// Log panic with request context
			log.Printf(`{"time":"%s","level":"error","request_id":"%s","event":"panic","error":"%v","path":"%s","method":"%s","ip":"%s"}`,
				time.Now().Format(time.RFC3339),
				c.Locals("requestid"),
				e,
				c.Path(),
				c.Method(),
				c.IP(),
			)
		},
	}))
}

// Custom security middleware
func (r *FiberRouter) securityMiddleware(c fiber.Ctx) error {
	// Add security headers
	c.Set("X-Response-Time", time.Now().Format(time.RFC3339))
	c.Set("Server", "Yamata-no-Orochi")

	// IP validation (if configured)
	clientIP := c.IP()

	// Simple IP blocking example
	blockedIPs := []string{
		"127.0.0.2", // Example blocked IP
	}

	for _, blockedIP := range blockedIPs {
		if clientIP == blockedIP {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error":   "access_denied",
				"message": "Access denied from this IP address",
			})
		}
	}

	// Continue to next middleware
	return c.Next()
}

// API key validation middleware
func (r *FiberRouter) apiKeyMiddleware(c fiber.Ctx) error {
	// Skip API key validation for certain endpoints
	if c.Path() == "/api/v1/health" || c.Path() == "/api/v1/docs" {
		return c.Next()
	}

	// Check if API key is required (this would come from config)
	requireAPIKey := false // Set from environment/config

	if requireAPIKey {
		apiKey := c.Get("X-API-Key")
		if apiKey == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "missing_api_key",
				"message": "API key is required",
			})
		}

		// Validate API key (this would check against database/config)
		validAPIKeys := []string{
			"your-production-api-key", // Example - load from config
		}

		isValid := false
		for _, validKey := range validAPIKeys {
			if apiKey == validKey {
				isValid = true
				break
			}
		}

		if !isValid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "invalid_api_key",
				"message": "Invalid API key",
			})
		}
	}

	return c.Next()
}

// Start starts the HTTP server
func (r *FiberRouter) Start(address string) error {
	log.Printf("Starting server on %s", address)
	return r.app.Listen(address)
}

// GetApp returns the Fiber app instance
func (r *FiberRouter) GetApp() *fiber.App {
	return r.app
}

// Health check endpoint
func (r *FiberRouter) healthCheck(c fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"version":   "1.0.0",
		"service":   "yamata-no-orochi-api",
	})
}

// API documentation endpoint
func (r *FiberRouter) getAPIDocumentation(c fiber.Ctx) error {
	docs := GetRouteDocumentation()
	return c.JSON(fiber.Map{
		"title":       "Yamata no Orochi API Documentation",
		"version":     "1.0.0",
		"description": "Customer signup and authentication API",
		"endpoints":   docs,
	})
}

// Not found handler
func (r *FiberRouter) notFoundHandler(c fiber.Ctx) error {
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
		"error":   "not_found",
		"message": "The requested resource was not found",
		"path":    c.Path(),
		"method":  c.Method(),
	})
}

// Global error handler
func errorHandler(c fiber.Ctx, err error) error {
	// Default error code
	code := fiber.StatusInternalServerError

	// Retrieve the custom status code if it's a fiber.*Error
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	// Log the error
	log.Printf("Error %d: %v", code, err)

	// Return JSON error response
	return c.Status(code).JSON(fiber.Map{
		"error":     "internal_error",
		"message":   "An internal server error occurred",
		"timestamp": time.Now().Unix(),
	})
}

// Helper functions

// generateRequestID creates a unique request ID
func generateRequestID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// contains checks if a string contains a substring
func contains(str, substr string) bool {
	return strings.Contains(str, substr)
}

// GetRouteDocumentation returns API documentation
func GetRouteDocumentation() []map[string]any {
	return []map[string]any{
		{
			"method":      "POST",
			"path":        "/api/v1/auth/signup",
			"description": "Initiate customer signup process",
			"parameters": map[string]any{
				"account_type":              "string (required) - individual|independent_company|marketing_agency",
				"representative_first_name": "string (required) - Representative first name",
				"representative_last_name":  "string (required) - Representative last name",
				"representative_mobile":     "string (required) - Mobile number in +989xxxxxxxxx format",
				"email":                     "string (required) - Email address",
				"password":                  "string (required) - Password",
				"confirm_password":          "string (required) - Password confirmation",
				"company_name":              "string (optional) - Required for business accounts",
				"national_id":               "string (optional) - Required for business accounts",
				"company_phone":             "string (optional) - Required for business accounts",
				"company_address":           "string (optional) - Required for business accounts",
				"postal_code":               "string (optional) - Required for business accounts",
				"referrer_agency_code":      "string (optional) - Agency referral code",
			},
		},
		{
			"method":      "POST",
			"path":        "/api/v1/auth/verify",
			"description": "Verify OTP and complete signup",
			"parameters": map[string]any{
				"customer_id": "number (required) - Customer ID from signup response",
				"otp_code":    "string (required) - 6-digit OTP code",
				"otp_type":    "string (required) - mobile|email",
			},
		},
		{
			"method":      "POST",
			"path":        "/api/v1/auth/resend-otp/:customer_id",
			"description": "Resend OTP to customer",
			"parameters": map[string]any{
				"customer_id": "number (required) - Customer ID in URL path",
				"type":        "string (optional) - Query parameter: mobile|email (default: mobile)",
			},
		},
		{
			"method":      "POST",
			"path":        "/api/v1/auth/login",
			"description": "Authenticate user with email/mobile and password",
			"parameters": map[string]any{
				"identifier": "string (required) - Email address or mobile number (+989xxxxxxxxx)",
				"password":   "string (required) - User password",
			},
		},
		{
			"method":      "POST",
			"path":        "/api/v1/auth/forgot-password",
			"description": "Initiate password reset process by sending OTP",
			"parameters": map[string]any{
				"identifier": "string (required) - Email address or mobile number (+989xxxxxxxxx)",
			},
		},
		{
			"method":      "POST",
			"path":        "/api/v1/auth/reset",
			"description": "Complete password reset with OTP verification",
			"parameters": map[string]any{
				"customer_id":      "number (required) - Customer ID from forgot-password response",
				"otp_code":         "string (required) - 6-digit OTP code from SMS",
				"new_password":     "string (required) - New password (min 8 chars, uppercase + number)",
				"confirm_password": "string (required) - Must match new_password",
			},
		},
		{
			"method":      "GET",
			"path":        "/api/v1/health",
			"description": "Health check endpoint",
			"parameters":  map[string]any{},
		},
	}
}
