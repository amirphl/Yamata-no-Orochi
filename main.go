// Package main provides the main entry point for the Yamata no Orochi authentication system
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/handlers"
	"github.com/amirphl/Yamata-no-Orochi/app/middleware"
	"github.com/amirphl/Yamata-no-Orochi/app/router"
	"github.com/amirphl/Yamata-no-Orochi/app/services"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/amirphl/Yamata-no-Orochi/config"
	_ "github.com/amirphl/Yamata-no-Orochi/docs"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"

	"github.com/amirphl/Yamata-no-Orochi/app/scheduler"
)

// Application represents the main application structure
type Application struct {
	router    *router.FiberRouter
	config    *config.ProductionConfig
	server    *fiber.App
	stopFuncs []func()
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

	// Stop background workers
	for _, fn := range app.stopFuncs {
		fn()
	}

	// Graceful  shutdown
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

// initializeCache initializes the Cache client and verifies connectivity
func initializeCache(cfg config.CacheConfig) (*redis.Client, error) {
	if !cfg.Enabled || cfg.Provider != "redis" {
		return nil, nil
	}

	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}
	// Override DB if provided in config
	opt.DB = cfg.RedisDB

	rc := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rc.Ping(ctx).Err(); err != nil {
		_ = rc.Close()
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	log.Printf("Redis connection established to %s (db=%d)", cfg.RedisURL, cfg.RedisDB)
	return rc, nil
}

// startCacheHealthMonitor starts a background goroutine that periodically pings Redis
// to detect connectivity issues. The returned cancel function stops the monitor.
func startCacheHealthMonitor(parent context.Context, client *redis.Client, interval time.Duration) func() {
	monitorCtx, cancel := context.WithCancel(parent)
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-monitorCtx.Done():
				return
			case <-ticker.C:
				ctx, c := context.WithTimeout(context.Background(), 3*time.Second)
				if err := client.Ping(ctx).Err(); err != nil {
					log.Printf("Redis healthcheck failed: %v", err)
				}
				c()
			}
		}
	}()
	return cancel
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

	// Create email provider (mock for now)
	emailProvider = services.NewMockEmailProvider()

	return services.NewNotificationService(smsService, emailProvider)
}

// initializeApplication initializes the main application components
func initializeApplication(cfg *config.ProductionConfig) (*Application, error) {
	var stopFuncs []func()

	// Initialize database
	db, err := initializeDatabase(cfg.Database)
	if err != nil {
		return nil, err
	}

	rc, err := initializeCache(cfg.Cache)
	if err != nil {
		return nil, err
	}

	cancel := startCacheHealthMonitor(context.Background(), rc, cfg.Cache.CleanupInterval)
	stopFuncs = append(stopFuncs, cancel)

	// Init system/tax entities dynamically using config
	if err := ensureSystemAndTaxEntities(db, cfg); err != nil {
		return nil, err
	}

	// Initialize repositories
	accountTypeRepo := repository.NewAccountTypeRepository(db)
	customerRepo := repository.NewCustomerRepository(db)
	sessionRepo := repository.NewCustomerSessionRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)
	campaignRepo := repository.NewCampaignRepository(db)
	walletRepo := repository.NewWalletRepository(db)
	paymentRequestRepo := repository.NewPaymentRequestRepository(db)
	balanceSnapshotRepo := repository.NewBalanceSnapshotRepository(db)
	transactionRepo := repository.NewTransactionRepository(db)
	agencyDiscountRepo := repository.NewAgencyDiscountRepository(db)
	adminRepo := repository.NewAdminRepository(db)
	lineNumberRepo := repository.NewLineNumberRepository(db)
	botRepo := repository.NewBotRepository(db)
	audienceProfileRepo := repository.NewAudienceProfileRepository(db)
	tagRepo := repository.NewTagRepository(db)
	sentSMSRepo := repository.NewSentSMSRepository(db)
	processedCampaignRepo := repository.NewProcessedCampaignRepository(db)

	// Initialize services
	notificationService := initializeNotificationService(cfg)

	// Captcha service for admin
	captchaSvc, err := services.NewCaptchaServiceRotate(2*time.Minute, 15, 300)
	if err != nil {
		return nil, err
	}

	// Initialize token service
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

	// Log that services are initialized
	log.Printf("Token service initialized with issuer: %s, audience: %s", cfg.JWT.Issuer, cfg.JWT.Audience)

	// Initialize flows
	signupFlow := businessflow.NewSignupFlow(
		customerRepo,
		accountTypeRepo,
		sessionRepo,
		auditRepo,
		agencyDiscountRepo,
		walletRepo,
		tokenService,
		notificationService,
		cfg.Admin,
		db,
		rc,
	)

	loginFlow := businessflow.NewLoginFlow(
		customerRepo,
		sessionRepo,
		auditRepo,
		accountTypeRepo,
		tokenService,
		notificationService,
		db,
		rc,
	)

	campaignFlow := businessflow.NewCampaignFlow(
		campaignRepo,
		customerRepo,
		walletRepo,
		balanceSnapshotRepo,
		transactionRepo,
		auditRepo,
		lineNumberRepo,
		db,
		rc,
		notificationService,
		cfg.Admin,
		&cfg.Cache,
	)

	// Initialize PaymentFlow
	paymentFlow := businessflow.NewPaymentFlow(
		paymentRequestRepo,
		walletRepo,
		customerRepo,
		auditRepo,
		balanceSnapshotRepo,
		transactionRepo,
		agencyDiscountRepo,
		db,
		cfg.Atipay,
		cfg.System,
		cfg.Deployment,
	)

	// Initialize AgencyFlow
	agencyFlow := businessflow.NewAgencyFlow(
		customerRepo,
		campaignRepo,
		agencyDiscountRepo,
		transactionRepo,
		auditRepo,
		db,
	)

	adminAuthFlow := businessflow.NewAdminAuthFlow(
		adminRepo,
		tokenService,
		captchaSvc,
	)

	botAuthFlow := businessflow.NewBotAuthFlow(
		botRepo,
		tokenService,
	)

	adminCampaignFlow := businessflow.NewAdminCampaignFlow(
		campaignRepo,
		customerRepo,
		walletRepo,
		balanceSnapshotRepo,
		transactionRepo,
		auditRepo,
		db,
		notificationService,
		cfg.Admin,
	)

	lineNumberFlow := businessflow.NewLineNumberFlow(lineNumberRepo)

	adminLineNumberFlow := businessflow.NewAdminLineNumberFlow(lineNumberRepo, db)

	adminCustomerManagementFlow := businessflow.NewAdminCustomerManagementFlow(customerRepo, campaignRepo, transactionRepo)

	botCampaignFlow := businessflow.NewBotCampaignFlow(campaignRepo, &cfg.Cache, db, rc)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(signupFlow, loginFlow)
	campaignHandler := handlers.NewCampaignHandler(campaignFlow)
	paymentHandler := handlers.NewPaymentHandler(paymentFlow)
	agencyHandler := handlers.NewAgencyHandler(agencyFlow)
	authAdminHandler := handlers.NewAuthAdminHandler(adminAuthFlow)
	authBotHandler := handlers.NewAuthBotHandler(botAuthFlow)
	campaignAdminHandler := handlers.NewCampaignAdminHandler(adminCampaignFlow)
	lineNumberHandler := handlers.NewLineNumberHandler(lineNumberFlow)
	lineNumberAdminHandler := handlers.NewLineNumberAdminHandler(adminLineNumberFlow)
	adminCustomerManagementHandler := handlers.NewAdminCustomerManagementHandler(adminCustomerManagementFlow)
	campaignBotHandler := handlers.NewCampaignBotHandler(botCampaignFlow)

	// Initialize auth middleware
	authMiddleware := middleware.NewAuthMiddleware(tokenService)

	// Initialize router
	appRouter := router.NewFiberRouter(
		authHandler,
		campaignHandler,
		paymentHandler,
		agencyHandler,
		authMiddleware,
		authAdminHandler,
		authBotHandler,
		campaignAdminHandler,
		lineNumberHandler,
		lineNumberAdminHandler,
		adminCustomerManagementHandler,
		campaignBotHandler,
	)

	if cfg.Scheduler.CampaignExecutionEnabled {
		// Start campaign scheduler (every 1 minute)
		sched := scheduler.NewCampaignScheduler(audienceProfileRepo, tagRepo, sentSMSRepo, processedCampaignRepo, notificationService, db, log.Default(), cfg.Scheduler.CampaignExecutionInterval, cfg.PayamSMS, cfg.Bot)
		stopScheduler := sched.Start(context.Background())
		stopFuncs = append(stopFuncs, stopScheduler)
	}

	// Create application struct from FiberRouter
	fiberRouter := appRouter.(*router.FiberRouter)
	application := &Application{
		router:    fiberRouter,
		config:    cfg,
		server:    fiberRouter.GetApp(),
		stopFuncs: stopFuncs,
	}

	return application, nil
}

func ensureSystemAndTaxEntities(db *gorm.DB, cfg *config.ProductionConfig) error {
	customerRepo := repository.NewCustomerRepository(db)
	accountTypeRepo := repository.NewAccountTypeRepository(db)
	walletRepo := repository.NewWalletRepository(db)

	// Ensure system user
	if cfg.System.SystemUserUUID != "" {
		if err := ensureCustomerByUUID(
			customerRepo,
			accountTypeRepo,
			cfg.System.SystemUserUUID,
			models.AccountTypeMarketingAgency,
			"System",
			"Account",
			cfg.System.SystemUserEmail,
			cfg.System.SystemUserMobile,
			"jaazebeh.ir",
			cfg.System.SystemShebaNumber,
		); err != nil {
			return err
		}
	}
	// Ensure tax user
	if cfg.System.TaxUserUUID != "" {
		if err := ensureCustomerByUUID(
			customerRepo,
			accountTypeRepo,
			cfg.System.TaxUserUUID,
			models.AccountTypeIndividual,
			"Tax",
			"Collector",
			cfg.System.TaxUserEmail,
			cfg.System.TaxUserMobile,
			"tax.jaazebeh.ir",
			"",
		); err != nil {
			return err
		}
	}

	// Ensure system wallet
	if cfg.System.SystemWalletUUID != "" && cfg.System.SystemUserUUID != "" {
		if err := ensureWalletByUUID(
			customerRepo,
			walletRepo,
			cfg.System.SystemWalletUUID,
			cfg.System.SystemUserUUID,
			map[string]any{"type": "system_wallet", "owner": "system", "source": "ensure_system_wallet"},
		); err != nil {
			return err
		}
	}

	// Ensure tax wallet
	if cfg.System.TaxWalletUUID != "" && cfg.System.TaxUserUUID != "" {
		if err := ensureWalletByUUID(
			customerRepo,
			walletRepo,
			cfg.System.TaxWalletUUID,
			cfg.System.TaxUserUUID,
			map[string]any{"type": "tax_wallet", "owner": "system", "source": "ensure_tax_wallet"},
		); err != nil {
			return err
		}
	}

	return nil
}

func ensureCustomerByUUID(
	customerRepo repository.CustomerRepository,
	accountTypeRepo repository.AccountTypeRepository,
	uuidStr, accountTypeName, firstName, lastName, email, mobile, agencyRefererCode, shebaNumber string,
) error {
	parsed, err := uuid.Parse(uuidStr)
	if err != nil {
		return err
	}
	customers, err := customerRepo.ByFilter(context.Background(), models.CustomerFilter{UUID: &parsed}, "", 1, 0)
	if err != nil {
		return err
	}
	if len(customers) > 0 {
		return nil
	}

	accountType, err := accountTypeRepo.ByTypeName(context.Background(), accountTypeName)
	if err != nil {
		return err
	}

	// Create minimal customer
	a := models.Customer{
		UUID:                    parsed,
		AgencyRefererCode:       agencyRefererCode,
		ShebaNumber:             utils.ToPtr(shebaNumber),
		AccountTypeID:           accountType.ID,
		RepresentativeFirstName: firstName,
		RepresentativeLastName:  lastName,
		RepresentativeMobile:    mobile,
		Email:                   email,
		PasswordHash:            "", // not used
		IsActive:                utils.ToPtr(true),
		IsEmailVerified:         utils.ToPtr(false),
		IsMobileVerified:        utils.ToPtr(false),
		CreatedAt:               utils.UTCNow(),
		UpdatedAt:               utils.UTCNow(),
	}
	err = customerRepo.Save(context.Background(), &a)
	if err != nil {
		return err
	}
	return nil
}

func ensureWalletByUUID(
	customerRepo repository.CustomerRepository,
	walletRepo repository.WalletRepository,
	walletUUID, ownerUUID string,
	metadata map[string]any) error {
	// Lookup owner by UUID
	ownerParsed, err := uuid.Parse(ownerUUID)
	if err != nil {
		return err
	}
	customers, err := customerRepo.ByFilter(context.Background(), models.CustomerFilter{UUID: &ownerParsed}, "", 1, 0)
	if err != nil {
		return err
	}
	if len(customers) == 0 {
		return fmt.Errorf("owner not found")
	}
	owner := customers[0]
	// Check wallet exists
	w, err := walletRepo.ByUUID(context.Background(), walletUUID)
	if err != nil {
		return err
	}
	if w != nil {
		return nil
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	// Create wallet with initial snapshot
	err = walletRepo.SaveWithInitialSnapshot(context.Background(), &models.Wallet{
		UUID:       uuid.MustParse(walletUUID),
		CustomerID: owner.ID,
		Metadata:   metadataJSON,
	})
	if err != nil {
		return err
	}
	return nil
}
