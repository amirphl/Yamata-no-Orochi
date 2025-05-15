// Package main provides the main entry point for the Yamata no Orochi authentication system
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/adapters"
	"github.com/amirphl/Yamata-no-Orochi/app/handlers"
	"github.com/amirphl/Yamata-no-Orochi/app/router"
	"github.com/amirphl/Yamata-no-Orochi/app/services"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Application represents the main application structure
type Application struct {
	router Router
}

// Router interface for HTTP routing
type Router interface {
	SetupRoutes()
	Start(address string) error
}

// Config represents application configuration
type Config struct {
	Environment string         `json:"environment"`
	Database    DatabaseConfig `json:"database"`
	Server      ServerConfig   `json:"server"`
	JWT         JWTConfig      `json:"jwt"`
	SMS         SMSConfig      `json:"sms"`
	Email       EmailConfig    `json:"email"`
	Logging     LoggingConfig  `json:"logging"`
}

// DatabaseConfig represents database configuration
type DatabaseConfig struct {
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Name            string `json:"name"`
	User            string `json:"user"`
	Password        string `json:"password"`
	SSLMode         string `json:"ssl_mode"`
	MaxOpenConns    int    `json:"max_open_conns"`
	MaxIdleConns    int    `json:"max_idle_conns"`
	ConnMaxLifetime int    `json:"conn_max_lifetime_minutes"`
}

// ServerConfig represents server configuration
type ServerConfig struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	ReadTimeout  int    `json:"read_timeout_seconds"`
	WriteTimeout int    `json:"write_timeout_seconds"`
	IdleTimeout  int    `json:"idle_timeout_seconds"`
}

// JWTConfig represents JWT configuration
type JWTConfig struct {
	Issuer          string        `json:"issuer"`
	Audience        string        `json:"audience"`
	AccessTokenTTL  time.Duration `json:"access_token_ttl"`
	RefreshTokenTTL time.Duration `json:"refresh_token_ttl"`
}

// SMSConfig represents SMS configuration
type SMSConfig struct {
	Provider   string `json:"provider"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	FromNumber string `json:"from_number"`
	APIKey     string `json:"api_key"`
	APIURL     string `json:"api_url"`
}

// EmailConfig represents email configuration
type EmailConfig struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	FromEmail string `json:"from_email"`
	UseTLS    bool   `json:"use_tls"`
}

// LoggingConfig represents logging configuration
type LoggingConfig struct {
	Level      string `json:"level"`
	Format     string `json:"format"`
	OutputPath string `json:"output_path"`
}

func main() {
	log.Println("Starting Yamata no Orochi application...")

	// Load configuration
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize application
	app, err := initializeApplication(config)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	// Setup routes
	app.router.SetupRoutes()

	// Start server
	address := fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port)
	log.Printf("Server starting on %s", address)

	if err := app.router.Start(address); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// loadConfig loads configuration from environment variables and config files
func loadConfig() (*Config, error) {
	config := &Config{}

	// Set environment
	config.Environment = getEnv("APP_ENV", "development")

	// Load database configuration
	config.Database = DatabaseConfig{
		Host:            getEnv("DB_HOST", "localhost"),
		Port:            getEnvAsInt("DB_PORT", 5432),
		Name:            getEnv("DB_NAME", "yamata_no_orochi"),
		User:            getEnv("DB_USER", "postgres"),
		Password:        getEnv("DB_PASSWORD", "password"),
		SSLMode:         getEnv("DB_SSL_MODE", "disable"),
		MaxOpenConns:    getEnvAsInt("DB_MAX_OPEN_CONNS", 25),
		MaxIdleConns:    getEnvAsInt("DB_MAX_IDLE_CONNS", 5),
		ConnMaxLifetime: getEnvAsInt("DB_CONN_MAX_LIFETIME_MINUTES", 15),
	}

	// Load server configuration
	config.Server = ServerConfig{
		Host:         getEnv("SERVER_HOST", "0.0.0.0"),
		Port:         getEnvAsInt("SERVER_PORT", 8080),
		ReadTimeout:  getEnvAsInt("SERVER_READ_TIMEOUT_SECONDS", 30),
		WriteTimeout: getEnvAsInt("SERVER_WRITE_TIMEOUT_SECONDS", 30),
		IdleTimeout:  getEnvAsInt("SERVER_IDLE_TIMEOUT_SECONDS", 60),
	}

	// Load JWT configuration
	config.JWT = JWTConfig{
		Issuer:          getEnv("JWT_ISSUER", "yamata-orochi"),
		Audience:        getEnv("JWT_AUDIENCE", "yamata-api"),
		AccessTokenTTL:  getEnvAsDuration("JWT_ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL: getEnvAsDuration("JWT_REFRESH_TOKEN_TTL", 7*24*time.Hour),
	}

	// Load SMS configuration
	config.SMS = SMSConfig{
		Provider:   getEnv("SMS_PROVIDER", "mock"),
		Username:   getEnv("SMS_USERNAME", ""),
		Password:   getEnv("SMS_PASSWORD", ""),
		FromNumber: getEnv("SMS_FROM_NUMBER", "+989000000000"),
		APIKey:     getEnv("SMS_API_KEY", ""),
		APIURL:     getEnv("SMS_API_URL", ""),
	}

	// Load email configuration
	config.Email = EmailConfig{
		Host:      getEnv("EMAIL_HOST", "smtp.gmail.com"),
		Port:      getEnvAsInt("EMAIL_PORT", 587),
		Username:  getEnv("EMAIL_USERNAME", ""),
		Password:  getEnv("EMAIL_PASSWORD", ""),
		FromEmail: getEnv("EMAIL_FROM_EMAIL", "noreply@yamata-no-orochi.com"),
		UseTLS:    getEnvAsBool("EMAIL_USE_TLS", true),
	}

	// Load logging configuration
	config.Logging = LoggingConfig{
		Level:      getEnv("LOG_LEVEL", "info"),
		Format:     getEnv("LOG_FORMAT", "json"),
		OutputPath: getEnv("LOG_OUTPUT_PATH", "stdout"),
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Log configuration summary (without sensitive data)
	logConfigSummary(config)

	return config, nil
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt gets an environment variable as integer with a default value
func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
		log.Printf("Warning: Invalid integer value for %s, using default: %d", key, defaultValue)
	}
	return defaultValue
}

// getEnvAsBool gets an environment variable as boolean with a default value
func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
		log.Printf("Warning: Invalid boolean value for %s, using default: %t", key, defaultValue)
	}
	return defaultValue
}

// getEnvAsDuration gets an environment variable as duration with a default value
func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
		log.Printf("Warning: Invalid duration value for %s, using default: %v", key, defaultValue)
	}
	return defaultValue
}

// validateConfig validates the configuration
func validateConfig(config *Config) error {
	var errors []string

	// Validate environment
	if config.Environment != "development" && config.Environment != "staging" && config.Environment != "production" {
		errors = append(errors, "environment must be one of: development, staging, production")
	}

	// Validate database configuration
	if config.Database.Host == "" {
		errors = append(errors, "database host is required")
	}
	if config.Database.Port <= 0 || config.Database.Port > 65535 {
		errors = append(errors, "database port must be between 1 and 65535")
	}
	if config.Database.Name == "" {
		errors = append(errors, "database name is required")
	}
	if config.Database.User == "" {
		errors = append(errors, "database user is required")
	}
	if config.Environment == "production" && config.Database.Password == "" {
		errors = append(errors, "database password is required in production")
	}
	if config.Database.SSLMode != "disable" && config.Database.SSLMode != "require" && config.Database.SSLMode != "verify-ca" && config.Database.SSLMode != "verify-full" {
		errors = append(errors, "database ssl_mode must be one of: disable, require, verify-ca, verify-full")
	}

	// Validate server configuration
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		errors = append(errors, "server port must be between 1 and 65535")
	}
	if config.Server.ReadTimeout <= 0 {
		errors = append(errors, "server read_timeout must be greater than 0")
	}
	if config.Server.WriteTimeout <= 0 {
		errors = append(errors, "server write_timeout must be greater than 0")
	}
	if config.Server.IdleTimeout <= 0 {
		errors = append(errors, "server idle_timeout must be greater than 0")
	}

	// Validate JWT configuration
	if config.JWT.Issuer == "" {
		errors = append(errors, "JWT issuer is required")
	}
	if config.JWT.Audience == "" {
		errors = append(errors, "JWT audience is required")
	}
	if config.JWT.AccessTokenTTL <= 0 {
		errors = append(errors, "JWT access_token_ttl must be greater than 0")
	}
	if config.JWT.RefreshTokenTTL <= 0 {
		errors = append(errors, "JWT refresh_token_ttl must be greater than 0")
	}

	// Validate SMS configuration
	if config.SMS.Provider != "mock" && config.SMS.Provider != "iranian" {
		errors = append(errors, "SMS provider must be one of: mock, iranian")
	}
	if config.SMS.Provider != "mock" && config.SMS.Username == "" {
		errors = append(errors, "SMS username is required for non-mock providers")
	}
	if config.SMS.Provider != "mock" && config.SMS.Password == "" {
		errors = append(errors, "SMS password is required for non-mock providers")
	}

	// Validate email configuration
	if config.Email.Host == "" {
		errors = append(errors, "email host is required")
	}
	if config.Email.Port <= 0 || config.Email.Port > 65535 {
		errors = append(errors, "email port must be between 1 and 65535")
	}
	if config.Email.Username == "" {
		errors = append(errors, "email username is required")
	}
	if config.Email.Password == "" {
		errors = append(errors, "email password is required")
	}

	// Validate logging configuration
	if config.Logging.Level != "debug" && config.Logging.Level != "info" && config.Logging.Level != "warn" && config.Logging.Level != "error" {
		errors = append(errors, "log level must be one of: debug, info, warn, error")
	}
	if config.Logging.Format != "json" && config.Logging.Format != "text" {
		errors = append(errors, "log format must be one of: json, text")
	}

	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}

// logConfigSummary logs a summary of the configuration (without sensitive data)
func logConfigSummary(config *Config) {
	log.Printf("Configuration loaded:")
	log.Printf("  Environment: %s", config.Environment)
	log.Printf("  Database: %s:%d/%s (SSL: %s)", config.Database.Host, config.Database.Port, config.Database.Name, config.Database.SSLMode)
	log.Printf("  Server: %s:%d", config.Server.Host, config.Server.Port)
	log.Printf("  JWT: Issuer=%s, Audience=%s, AccessTTL=%v, RefreshTTL=%v",
		config.JWT.Issuer, config.JWT.Audience, config.JWT.AccessTokenTTL, config.JWT.RefreshTokenTTL)
	log.Printf("  SMS: Provider=%s, From=%s", config.SMS.Provider, config.SMS.FromNumber)
	log.Printf("  Email: %s:%d, From=%s", config.Email.Host, config.Email.Port, config.Email.FromEmail)
	log.Printf("  Logging: Level=%s, Format=%s", config.Logging.Level, config.Logging.Format)
}

// initializeDatabase initializes the database connection with connection pooling
func initializeDatabase(config DatabaseConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.Name, config.SSLMode)

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
	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(config.ConnMaxLifetime) * time.Minute)

	// Test the connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Printf("Database connection established with %d max open connections, %d max idle connections",
		config.MaxOpenConns, config.MaxIdleConns)

	return db, nil
}

// initializeNotificationService initializes the notification service
func initializeNotificationService(config *Config) services.NotificationService {
	// Create mock providers for development
	smsProvider := services.NewMockSMSProvider()
	emailProvider := services.NewMockEmailProvider()

	return services.NewNotificationService(smsProvider, emailProvider)
}

// initializeApplication initializes the application components
func initializeApplication(config *Config) (*Application, error) {
	// Initialize database
	db, err := initializeDatabase(config.Database)
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
		config.JWT.AccessTokenTTL,
		config.JWT.RefreshTokenTTL,
		config.JWT.Issuer,   // issuer
		config.JWT.Audience, // audience
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize token service: %w", err)
	}

	notificationService := initializeNotificationService(config)

	// Initialize business flows
	signupFlow := businessflow.NewSignupFlow(
		customerRepo,
		accountTypeRepo,
		otpRepo,
		sessionRepo,
		auditRepo,
		tokenService,
		notificationService,
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

	// Create adapters for handlers
	signupFlowAdapter := adapters.NewHandlerSignupFlowAdapter(signupFlow)
	loginFlowAdapter := adapters.NewHandlerLoginFlowAdapter(loginFlow)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(signupFlowAdapter, loginFlowAdapter)

	// Initialize router
	appRouter := router.NewFiberRouter(authHandler)

	// Log that services are initialized
	log.Printf("Token service initialized with issuer: %s, audience: %s", config.JWT.Issuer, config.JWT.Audience)
	log.Printf("Notification service initialized")
	log.Printf("Database initialized: %s:%d/%s", config.Database.Host, config.Database.Port, config.Database.Name)

	log.Println("Application components initialized successfully")

	return &Application{
		router: appRouter,
	}, nil
}
