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
	"github.com/amirphl/Yamata-no-Orochi/app/middleware"
	"github.com/amirphl/Yamata-no-Orochi/app/router"
	"github.com/amirphl/Yamata-no-Orochi/app/services"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/amirphl/Yamata-no-Orochi/config"
	_ "github.com/amirphl/Yamata-no-Orochi/docs"
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

	// Setup routes
	app.router.SetupRoutes()

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
	// Create SMS service based on configuration
	var smsService services.SMSService
	var emailProvider services.EmailProvider

	switch cfg.SMS.ProviderDomain {
	case "mock":
		smsService = services.NewMockSMSService()
	default:
		smsService = services.NewSMSService(&cfg.SMS)
	}

	// Initialize email provider with mock for now
	emailProvider = services.NewMockEmailProvider()

	return services.NewNotificationService(smsService, emailProvider)
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
	smsCampaignRepo := repository.NewSMSCampaignRepository(db)

	// Initialize services
	tokenService, err := services.NewTokenService(
		cfg.JWT.AccessTokenTTL,
		cfg.JWT.RefreshTokenTTL,
		cfg.JWT.Issuer,
		cfg.JWT.Audience,
		cfg.JWT.UseRSAKeys,
		cfg.JWT.PrivateKey,
		cfg.JWT.PublicKey,
		cfg.JWT.SecretKey,
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

	smsCampaignFlow := businessflow.NewSMSCampaignFlow(
		smsCampaignRepo,
		customerRepo,
		auditRepo,
		db,
	)

	// Initialize additional repositories needed for PaymentFlow
	walletRepo := repository.NewWalletRepository(db)
	paymentRequestRepo := repository.NewPaymentRequestRepository(db)
	balanceSnapshotRepo := repository.NewBalanceSnapshotRepository(db)
	transactionRepo := repository.NewTransactionRepository(db)

	// Initialize PaymentFlow
	paymentFlow := businessflow.NewPaymentFlow(
		paymentRequestRepo,
		walletRepo,
		customerRepo,
		auditRepo,
		balanceSnapshotRepo,
		transactionRepo,
		db,
		cfg.Atipay.APIKey,
		cfg.Atipay.Terminal,
		cfg.Deployment.Domain, // domain from config
	)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(signupFlow, loginFlow)
	smsCampaignHandler := handlers.NewSMSCampaignHandler(smsCampaignFlow)
	paymentHandler := handlers.NewPaymentHandler(paymentFlow)

	// Initialize auth middleware
	authMiddleware := middleware.NewAuthMiddleware(tokenService)

	// Initialize router
	appRouter := router.NewFiberRouter(authHandler, smsCampaignHandler, paymentHandler, authMiddleware)

	// Log that services are initialized
	log.Printf("Token service initialized with issuer: %s, audience: %s", cfg.JWT.Issuer, cfg.JWT.Audience)
	log.Printf("Notification service initialized with SMS domain: %s", cfg.SMS.ProviderDomain)
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
