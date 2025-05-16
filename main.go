// Package main provides the main entry point for the Yamata no Orochi authentication system
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/amirphl/Yamata-no-Orochi/app/handlers"
	"github.com/amirphl/Yamata-no-Orochi/app/router"
	"github.com/amirphl/Yamata-no-Orochi/app/services"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/gofiber/fiber/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Application represents the main application structure
type Application struct {
	router *router.FiberRouter
	config *config.ProductionConfig
	server *fiber.App
}

func main() {
	log.Println("Starting Yamata no Orochi application...")

	// Load production configuration
	cfg, err := config.LoadProductionConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize application
	app, err := initializeApplication(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	// Setup graceful shutdown
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		address := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		log.Printf("Server starting on %s", address)

		if err := app.server.Listen(address); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutting down gracefully...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	if err := app.server.ShutdownWithContext(shutdownCtx); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	log.Println("Server stopped")
}

// initializeDatabase initializes the database connection with connection pooling
func initializeDatabase(cfg config.DatabaseConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying sql.DB for connection pooling configuration
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// Configure connection pooling
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// Test the connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Printf("Database connection established with %d max open connections, %d max idle connections",
		cfg.MaxOpenConns, cfg.MaxIdleConns)

	return db, nil
}

// initializeNotificationService initializes the notification service
func initializeNotificationService(cfg *config.ProductionConfig) services.NotificationService {
	// Create providers based on configuration
	var smsProvider services.SMSProvider
	var emailProvider services.EmailProvider

	switch cfg.SMS.Provider {
	case "mock":
		smsProvider = services.NewMockSMSProvider()
	default:
		log.Printf("Unknown SMS provider: %s, using mock", cfg.SMS.Provider)
		smsProvider = services.NewMockSMSProvider()
	}

	// Initialize email provider with mock for now
	emailProvider = services.NewMockEmailProvider()

	return services.NewNotificationService(smsProvider, emailProvider)
}

// initializeApplication initializes the application components
func initializeApplication(cfg *config.ProductionConfig) (*Application, error) {
	// Initialize database
	db, err := initializeDatabase(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Initialize repositories
	customerRepo := repository.NewCustomerRepository(db)
	accountTypeRepo := repository.NewAccountTypeRepository(db)
	otpRepo := repository.NewOTPVerificationRepository(db)
	sessionRepo := repository.NewCustomerSessionRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	// Initialize services
	tokenService, err := services.NewTokenService(
		cfg.JWT.AccessTokenTTL,
		cfg.JWT.RefreshTokenTTL,
		cfg.JWT.Issuer,
		cfg.JWT.Audience,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize token service: %w", err)
	}

	notificationService := initializeNotificationService(cfg)

	// Initialize business flows
	signupFlow := businessflow.NewSignupFlow(
		customerRepo,
		accountTypeRepo,
		otpRepo,
		sessionRepo,
		auditRepo,
		tokenService,
		notificationService,
		db,
	)

	loginFlow := businessflow.NewLoginFlow(
		customerRepo,
		sessionRepo,
		otpRepo,
		auditRepo,
		accountTypeRepo,
		tokenService,
		notificationService,
		db,
	)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(signupFlow, loginFlow)

	// Initialize router
	appRouter := router.NewFiberRouter(authHandler)

	// Log that services are initialized
	log.Printf("Token service initialized with issuer: %s, audience: %s", cfg.JWT.Issuer, cfg.JWT.Audience)
	log.Printf("Notification service initialized with provider: %s", cfg.SMS.Provider)
	log.Printf("Database initialized: %s:%d/%s", cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)

	log.Println("Application components initialized successfully")

	// Type assertion to get the concrete FiberRouter
	fiberRouter := appRouter.(*router.FiberRouter)

	return &Application{
		router: fiberRouter,
		config: cfg,
		server: fiberRouter.GetApp(),
	}, nil
}
