// Package router provides HTTP routing, middleware configuration, and server setup for the web application
package router

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/app/handlers"
	"github.com/amirphl/Yamata-no-Orochi/app/middleware"
	_ "github.com/amirphl/Yamata-no-Orochi/docs"
	"github.com/amirphl/Yamata-no-Orochi/utils"
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
	app                            *fiber.App
	authHandler                    handlers.AuthHandlerInterface
	campaignHandler                handlers.CampaignHandlerInterface
	paymentHandler                 handlers.PaymentHandlerInterface
	agencyHandler                  handlers.AgencyHandlerInterface
	authMiddleware                 *middleware.AuthMiddleware
	authAdminHandler               handlers.AuthAdminHandlerInterface
	authBotHandler                 handlers.AuthBotHandlerInterface
	campaignAdminHandler           handlers.CampaignAdminHandlerInterface
	lineNumberHandler              handlers.LineNumberHandlerInterface
	lineNumberAdminHandler         handlers.LineNumberAdminHandlerInterface
	adminCustomerManagementHandler handlers.AdminCustomerManagementHandlerInterface
	campaignBotHandler             handlers.CampaignBotHandlerInterface
	ticketHandler                  handlers.TicketHandlerInterface
	shortLinkBotHandler            handlers.ShortLinkBotHandlerInterface
	shortLinkHandler               handlers.ShortLinkHandlerInterface
	shortLinkAdminHandler          handlers.ShortLinkAdminHandlerInterface
}

// NewFiberRouter creates a new Fiber router
func NewFiberRouter(
	authHandler handlers.AuthHandlerInterface,
	campaignHandler handlers.CampaignHandlerInterface,
	paymentHandler handlers.PaymentHandlerInterface,
	agencyHandler handlers.AgencyHandlerInterface,
	authMiddleware *middleware.AuthMiddleware,
	authAdminHandler handlers.AuthAdminHandlerInterface,
	authBotHandler handlers.AuthBotHandlerInterface,
	campaignAdminHandler handlers.CampaignAdminHandlerInterface,
	lineNumberHandler handlers.LineNumberHandlerInterface,
	lineNumberAdminHandler handlers.LineNumberAdminHandlerInterface,
	adminCustomerManagemetHandler handlers.AdminCustomerManagementHandlerInterface,
	campaignBotHandler handlers.CampaignBotHandlerInterface,
	ticketHandler handlers.TicketHandlerInterface,
	shortLinkBotHandler handlers.ShortLinkBotHandlerInterface,
	shortLinkHandler handlers.ShortLinkHandlerInterface,
	shortLinkAdminHandler handlers.ShortLinkAdminHandlerInterface,
) Router {
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
		app:                    app,
		authHandler:            authHandler,
		campaignHandler:        campaignHandler,
		paymentHandler:         paymentHandler,
		agencyHandler:          agencyHandler,
		authMiddleware:         authMiddleware,
		authAdminHandler:       authAdminHandler,
		authBotHandler:         authBotHandler,
		campaignAdminHandler:   campaignAdminHandler,
		lineNumberHandler:      lineNumberHandler,
		lineNumberAdminHandler: lineNumberAdminHandler,

		adminCustomerManagementHandler: adminCustomerManagemetHandler,
		campaignBotHandler:             campaignBotHandler,
		ticketHandler:                  ticketHandler,
		shortLinkBotHandler:            shortLinkBotHandler,
		shortLinkHandler:               shortLinkHandler,
		shortLinkAdminHandler:          shortLinkAdminHandler,
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

	// API documentation route (development only)
	if os.Getenv("APP_ENV") == "development" || os.Getenv("APP_ENV") == "local" {
		api.Get("/docs", r.getAPIDocumentation)
		api.Get("/swagger.json", r.serveSwaggerJSON)
		// Serve Swagger UI
		r.app.Get("/swagger", r.serveSwaggerUI)
		// Serve standalone Swagger UI
		r.app.Get("/swagger-standalone", r.serveStandaloneSwaggerUI)
		// Serve Swagger UI static assets
		r.app.Get("/swagger-ui-assets/*", func(c fiber.Ctx) error {
			filePath := c.Params("*")
			return c.SendFile("./docs/swagger-ui-assets/" + filePath)
		})
		log.Println("API documentation enabled for development")
	}

	// Apply general rate limiting to all API routes (aligned with nginx)
	api.Use(limiter.New(limiter.Config{
		Max:        2000,            // Maximum 2000 requests (matches nginx api zone)
		Expiration: 1 * time.Minute, // Per minute
		KeyGenerator: func(c fiber.Ctx) string {
			return c.IP() // Rate limit by IP
		},
		LimitReached: func(c fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(dto.APIResponse{
				Success: false,
				Message: "Too many requests. Please try again later.",
				Error: dto.ErrorDetail{
					Code: "RATE_LIMIT_EXCEEDED",
				},
			})
		},
		Next: func(c fiber.Ctx) bool {
			// Skip rate limiting for health checks
			return c.Path() == "/api/v1/health"
		},
	}))

	// Auth routes with stricter rate limiting
	auth := api.Group("/auth")

	// Apply stricter rate limiting to auth endpoints (aligned with nginx)
	auth.Use(limiter.New(limiter.Config{
		Max:        20,              // Maximum 20 requests (matches nginx auth zone)
		Expiration: 1 * time.Minute, // Per minute
		KeyGenerator: func(c fiber.Ctx) string {
			return c.IP() // Rate limit by IP
		},
		LimitReached: func(c fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(dto.APIResponse{
				Success: false,
				Message: "Too many requests. Please try again later.",
				Error: dto.ErrorDetail{
					Code: "RATE_LIMIT_EXCEEDED",
				},
			})
		},
	}))

	// Auth endpoints
	auth.Post("/signup", r.authHandler.Signup)
	auth.Post("/verify", r.authHandler.VerifyOTP)
	auth.Post("/resend-otp", r.authHandler.ResendOTP)
	auth.Post("/login", r.authHandler.Login)
	auth.Post("/forgot-password", r.authHandler.ForgotPassword)
	auth.Post("/reset", r.authHandler.ResetPassword)

	// Admin auth routes (separate group; can have separate rate limit if needed)
	adminAuth := api.Group("/admin/auth")
	adminAuth.Use(limiter.New(limiter.Config{
		Max:          20,
		Expiration:   1 * time.Minute,
		KeyGenerator: func(c fiber.Ctx) string { return c.IP() },
		LimitReached: func(c fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(dto.APIResponse{
				Success: false,
				Message: "Too many requests. Please try again later.",
				Error:   dto.ErrorDetail{Code: "RATE_LIMIT_EXCEEDED"},
			})
		},
	}))
	adminAuth.Get("/captcha/init", r.authAdminHandler.InitCaptcha)
	adminAuth.Post("/login", r.authAdminHandler.VerifyLogin)

	// Bot auth
	botAuth := api.Group("/bot/auth")
	botAuth.Post("/login", r.authBotHandler.Login)

	// Campaign routes (protected with authentication)
	campaigns := api.Group("/campaigns")
	campaigns.Use(r.authMiddleware.Authenticate()) // Require authentication
	campaigns.Post("/", r.campaignHandler.CreateCampaign)
	campaigns.Put("/:uuid", r.campaignHandler.UpdateCampaign)
	campaigns.Get("/", r.campaignHandler.ListCampaigns)
	campaigns.Post("/calculate-capacity", r.campaignHandler.CalculateCampaignCapacity)
	campaigns.Post("/calculate-cost", r.campaignHandler.CalculateCampaignCost)
	campaigns.Get("/audience-spec", r.campaignHandler.ListAudienceSpec)

	// Admin campaigns listing and actions
	adminCampaigns := api.Group("/admin/campaigns")
	adminCampaigns.Use(r.authMiddleware.AdminAuthenticate())
	adminCampaigns.Use(func(c fiber.Ctx) error { return middleware.RequireAdminAuth(c) })
	adminCampaigns.Get("/", r.campaignAdminHandler.ListCampaigns)
	adminCampaigns.Get("/:id", r.campaignAdminHandler.GetCampaign)
	adminCampaigns.Post("/approve", r.campaignAdminHandler.ApproveCampaign)
	adminCampaigns.Post("/reject", r.campaignAdminHandler.RejectCampaign)

	// Bot campaigns routes (protected)
	botCampaigns := api.Group("/bot/campaigns")
	botCampaigns.Use(r.authMiddleware.BotAuthenticate())
	botCampaigns.Use(func(c fiber.Ctx) error { return middleware.RequireBotAuth(c) })
	botCampaigns.Get("/ready", r.campaignBotHandler.ListReadyCampaigns)
	botCampaigns.Post("/audience-spec", r.campaignBotHandler.UpdateAudienceSpec)
	botCampaigns.Post("/audience-spec/reset", r.campaignBotHandler.ResetAudienceSpec)
	botCampaigns.Post("/:id/executed", r.campaignBotHandler.MoveCampaignToExecuted)
	botCampaigns.Post("/:id/running", r.campaignBotHandler.MoveCampaignToRunning)

	// Bot short-links routes (protected)
	botShortLinks := api.Group("/bot/short-links")
	botShortLinks.Use(r.authMiddleware.BotAuthenticate())
	botShortLinks.Use(func(c fiber.Ctx) error { return middleware.RequireBotAuth(c) })
	botShortLinks.Post("/", r.shortLinkBotHandler.CreateShortLinks)
	botShortLinks.Post("/one", r.shortLinkBotHandler.CreateShortLink)

	// Admin short-links routes (protected)
	adminShortLinks := api.Group("/admin/short-links")
	adminShortLinks.Use(r.authMiddleware.AdminAuthenticate())
	adminShortLinks.Use(func(c fiber.Ctx) error { return middleware.RequireAdminAuth(c) })
	adminShortLinks.Post("/upload-csv", r.shortLinkAdminHandler.UploadCSV)
	adminShortLinks.Post("/download", r.shortLinkAdminHandler.DownloadByScenario)
	adminShortLinks.Post("/download-with-clicks", r.shortLinkAdminHandler.DownloadWithClicksByScenario)

	// Admin customer reports
	adminCustomers := api.Group("/admin/customer-management")
	adminCustomers.Use(r.authMiddleware.AdminAuthenticate())
	adminCustomers.Use(func(c fiber.Ctx) error { return middleware.RequireAdminAuth(c) })
	adminCustomers.Get("/shares", r.adminCustomerManagementHandler.GetCustomersShares)
	adminCustomers.Get("/:customer_id", r.adminCustomerManagementHandler.GetCustomerWithCampaigns)
	adminCustomers.Post("/active-status", r.adminCustomerManagementHandler.SetCustomerActiveStatus)
	adminCustomers.Get("/:customer_id/discounts", r.adminCustomerManagementHandler.GetCustomerDiscountsHistory)

	// Line numbers
	lineNumbers := api.Group("/line-numbers")
	lineNumbers.Use(r.authMiddleware.Authenticate()) // Require authentication
	lineNumbers.Get("/active", r.lineNumberHandler.ListActive)

	// Admin line numbers protected routes
	adminLineNumbers := api.Group("/admin/line-numbers")
	adminLineNumbers.Use(r.authMiddleware.AdminAuthenticate())
	adminLineNumbers.Use(func(c fiber.Ctx) error { return middleware.RequireAdminAuth(c) })
	adminLineNumbers.Get("/", r.lineNumberAdminHandler.ListLineNumbers)
	adminLineNumbers.Post("/", r.lineNumberAdminHandler.CreateLineNumber)
	adminLineNumbers.Put("/", r.lineNumberAdminHandler.UpdateLineNumbersBatch)
	adminLineNumbers.Get("/report", r.lineNumberAdminHandler.GetLineNumbersReport)

	// Tickets
	tickets := api.Group("/tickets")
	tickets.Use(r.authMiddleware.Authenticate())
	tickets.Post("/", r.ticketHandler.Create)
	tickets.Post("/reply", r.ticketHandler.CreateResponse)
	tickets.Get("/", r.ticketHandler.List)

	// Admin tickets (reply)
	adminTickets := api.Group("/admin/tickets")
	adminTickets.Use(r.authMiddleware.AdminAuthenticate())
	adminTickets.Use(func(c fiber.Ctx) error { return middleware.RequireAdminAuth(c) })
	adminTickets.Post("/reply", r.ticketHandler.AdminCreateResponse)
	adminTickets.Get("/", r.ticketHandler.AdminList)

	// Wallet routes (protected with authentication)
	wallet := api.Group("/wallet")
	wallet.Use(r.authMiddleware.Authenticate()) // Require authentication
	wallet.Get("/balance", r.paymentHandler.GetWalletBalance)

	// Payment routes
	payments := api.Group("/payments")
	// Charge wallet endpoint (protected with authentication)
	payments.Post("/charge-wallet", r.authMiddleware.Authenticate(), r.paymentHandler.ChargeWallet)
	// Payment callback endpoint (unprotected - called by Atipay)
	payments.Post("/callback/:invoice_number", r.paymentHandler.PaymentCallback)
	// Transaction history endpoint (protected with authentication)
	payments.Get("/history", r.authMiddleware.Authenticate(), r.paymentHandler.GetTransactionHistory)

	// Agency routes (protected)
	agency := api.Group("/reports")
	agency.Use(r.authMiddleware.Authenticate())
	agency.Get("/agency/customers", r.agencyHandler.GetAgencyCustomerReport)
	agency.Get("/agency/customers/list", r.agencyHandler.ListAgencyCustomers)
	agency.Get("/agency/discounts/active", r.agencyHandler.ListAgencyActiveDiscounts)
	agency.Get("/agency/customers/:customer_id/discounts", r.agencyHandler.ListAgencyCustomerDiscounts)
	agency.Post("/agency/discounts", r.agencyHandler.CreateAgencyDiscount)

	// Public short-link redirect (no auth)
	r.app.Get("/s/:uid", r.shortLinkHandler.Visit)

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

	// Prometheus HTTP metrics (concise)
	r.app.Use(middleware.Metrics())

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
			"https://monitoring.yamata-no-orochi.com",
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
		MaxAge:           utils.CORSMaxAge,
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
		StackTraceHandler: func(c fiber.Ctx, e any) {
			// Log panic with request context
			log.Printf(`{"time":"%s","level":"error","request_id":"%s","event":"panic","error":"%v","path":"%s","method":"%s","ip":"%s"}`,
				utils.UTCNow().Format(time.RFC3339),
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
	c.Set("X-Response-Time", utils.UTCNow().Format(time.RFC3339))
	c.Set("Server", "Yamata-no-Orochi")

	// IP validation (if configured)
	clientIP := c.IP()

	// Simple IP blocking example
	blockedIPs := []string{
		"127.0.0.2", // Example blocked IP
	}

	for _, blockedIP := range blockedIPs {
		if clientIP == blockedIP {
			return c.Status(fiber.StatusForbidden).JSON(dto.APIResponse{
				Success: false,
				Message: "Access denied from this IP address",
				Error: dto.ErrorDetail{
					Code: "ACCESS_DENIED",
				},
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
			return c.Status(fiber.StatusUnauthorized).JSON(dto.APIResponse{
				Success: false,
				Message: "API key is required",
				Error: dto.ErrorDetail{
					Code: "MISSING_API_KEY",
				},
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
			return c.Status(fiber.StatusUnauthorized).JSON(dto.APIResponse{
				Success: false,
				Message: "Invalid API key",
				Error: dto.ErrorDetail{
					Code: "INVALID_API_KEY",
				},
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
	return c.JSON(dto.APIResponse{
		Success: true,
		Message: "Service is healthy",
		Data: fiber.Map{
			"status":    "ok",
			"timestamp": utils.UTCNow().Unix(),
			"version":   "1.0.0",
			"service":   "yamata-no-orochi-api",
		},
	})
}

// API documentation endpoint
func (r *FiberRouter) getAPIDocumentation(c fiber.Ctx) error {
	docs := GetRouteDocumentation()
	return c.JSON(dto.APIResponse{
		Success: true,
		Message: "API documentation retrieved successfully",
		Data: fiber.Map{
			"title":       "Yamata no Orochi API Documentation",
			"version":     "1.0.0",
			"description": "Customer signup and authentication API",
			"endpoints":   docs,
		},
	})
}

// Serve Swagger UI HTML page
func (r *FiberRouter) serveSwaggerUI(c fiber.Ctx) error {
	htmlContent := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Yamata no Orochi API - Swagger UI</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui.css" />
    <style>
        html {
            box-sizing: border-box;
            overflow: -moz-scrollbars-vertical;
            overflow-y: scroll;
        }
        *, *:before, *:after {
            box-sizing: inherit;
        }
        body {
            margin:0;
            background: #fafafa;
        }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            const ui = SwaggerUIBundle({
                url: '/api/v1/swagger.json',
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout",
                validatorUrl: null,
                onComplete: function() {
                    console.log("Swagger UI loaded successfully");
                }
            });
        };
    </script>
</body>
</html>`

	c.Set("Content-Type", "text/html")
	return c.SendString(htmlContent)
}

// Serve Swagger JSON specification
func (r *FiberRouter) serveSwaggerJSON(c fiber.Ctx) error {
	// Read the generated swagger.json file
	swaggerData, err := os.ReadFile("docs/swagger.json")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.APIResponse{
			Success: false,
			Message: "Failed to load Swagger documentation",
			Error: dto.ErrorDetail{
				Code: "SWAGGER_LOAD_ERROR",
			},
		})
	}

	c.Set("Content-Type", "application/json")
	return c.Send(swaggerData)
}

// Serve standalone Swagger UI HTML page
func (r *FiberRouter) serveStandaloneSwaggerUI(c fiber.Ctx) error {
	// Read the standalone HTML file
	htmlData, err := os.ReadFile("docs/swagger-ui-standalone.html")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.APIResponse{
			Success: false,
			Message: "Failed to load standalone Swagger UI",
			Error: dto.ErrorDetail{
				Code: "SWAGGER_UI_LOAD_ERROR",
			},
		})
	}

	c.Set("Content-Type", "text/html")
	return c.Send(htmlData)
}

// Not found handler
func (r *FiberRouter) notFoundHandler(c fiber.Ctx) error {
	requestID := c.Locals("requestid")

	return c.Status(fiber.StatusNotFound).JSON(dto.APIResponse{
		Success: false,
		Message: "The requested resource was not found",
		Error: dto.ErrorDetail{
			Code: "NOT_FOUND",
			Details: fiber.Map{
				"path":       c.Path(),
				"method":     c.Method(),
				"request_id": requestID,
			},
		},
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

	// Get RequestID for tracing
	requestID := c.Locals("requestid")

	// Return JSON error response
	return c.Status(code).JSON(dto.APIResponse{
		Success: false,
		Message: "An internal server error occurred",
		Error: dto.ErrorDetail{
			Code: "INTERNAL_ERROR",
			Details: fiber.Map{
				"timestamp":  utils.UTCNow().Unix(),
				"request_id": requestID,
			},
		},
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
	return []map[string]any{}
}
