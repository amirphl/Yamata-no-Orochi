// Package config provides configuration management and environment variable handling for the application
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProductionConfig holds all configuration for production environment
type ProductionConfig struct {
	Database   DatabaseConfig   `json:"database"`
	Server     ServerConfig     `json:"server"`
	Security   SecurityConfig   `json:"security"`
	JWT        JWTConfig        `json:"jwt"`
	SMS        SMSConfig        `json:"sms"`
	Email      EmailConfig      `json:"email"`
	Logging    LoggingConfig    `json:"logging"`
	Metrics    MetricsConfig    `json:"metrics"`
	Cache      CacheConfig      `json:"cache"`
	Deployment DeploymentConfig `json:"deployment"`
	Atipay     AtipayConfig     `json:"atipay"`
	Admin      AdminConfig      `json:"admin"`
}

type DatabaseConfig struct {
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	Name            string        `json:"name"`
	User            string        `json:"user"`
	Password        string        `json:"password"`
	SSLMode         string        `json:"ssl_mode"`
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time"`
	SlowQueryLog    bool          `json:"slow_query_log"`
	SlowQueryTime   time.Duration `json:"slow_query_time"`
}

type ServerConfig struct {
	Host              string        `json:"host"`
	Port              int           `json:"port"`
	ReadTimeout       time.Duration `json:"read_timeout"`
	WriteTimeout      time.Duration `json:"write_timeout"`
	IdleTimeout       time.Duration `json:"idle_timeout"`
	ShutdownTimeout   time.Duration `json:"shutdown_timeout"`
	BodyLimit         int           `json:"body_limit"`
	EnablePprof       bool          `json:"enable_pprof"`
	EnableMetrics     bool          `json:"enable_metrics"`
	TrustedProxies    []string      `json:"trusted_proxies"`
	ProxyHeader       string        `json:"proxy_header"`
	EnableCompression bool          `json:"enable_compression"`
	CompressionLevel  int           `json:"compression_level"`
}

type SecurityConfig struct {
	// TLS/HTTPS
	TLSEnabled         bool   `json:"tls_enabled"`
	TLSCertFile        string `json:"tls_cert_file"`
	TLSKeyFile         string `json:"tls_key_file"`
	TLSMinVersion      string `json:"tls_min_version"`
	HSTSMaxAge         int    `json:"hsts_max_age"`
	HSTSIncludeSubDoms bool   `json:"hsts_include_subdomains"`
	HSTSPreload        bool   `json:"hsts_preload"`

	// CORS
	AllowedOrigins   []string `json:"allowed_origins"`
	AllowedMethods   []string `json:"allowed_methods"`
	AllowedHeaders   []string `json:"allowed_headers"`
	AllowCredentials bool     `json:"allow_credentials"`
	CORSMaxAge       int      `json:"cors_max_age"`

	// Rate Limiting
	AuthRateLimit   int           `json:"auth_rate_limit"`   // requests per minute
	GlobalRateLimit int           `json:"global_rate_limit"` // requests per minute
	RateLimitWindow time.Duration `json:"rate_limit_window"`
	RateLimitMemory int           `json:"rate_limit_memory"` // MB

	// Content Security
	CSPPolicy           string `json:"csp_policy"`
	XFrameOptions       string `json:"x_frame_options"`
	XContentTypeOptions string `json:"x_content_type_options"`
	XSSProtection       string `json:"xss_protection"`
	ReferrerPolicy      string `json:"referrer_policy"`

	// API Security
	RequireAPIKey  bool     `json:"require_api_key"`
	APIKeyHeader   string   `json:"api_key_header"`
	AllowedAPIKeys []string `json:"allowed_api_keys"`
	IPWhitelist    []string `json:"ip_whitelist"`
	IPBlacklist    []string `json:"ip_blacklist"`

	// Password & Auth
	PasswordMinLength     int  `json:"password_min_length"`
	PasswordRequireUpper  bool `json:"password_require_upper"`
	PasswordRequireLower  bool `json:"password_require_lower"`
	PasswordRequireNum    bool `json:"password_require_number"`
	PasswordRequireSymbol bool `json:"password_require_symbol"`
	BcryptCost            int  `json:"bcrypt_cost"`

	// Session Security
	SessionCookieSecure    bool          `json:"session_cookie_secure"`
	SessionCookieHTTPOnly  bool          `json:"session_cookie_httponly"`
	SessionCookieSameSite  string        `json:"session_cookie_samesite"`
	SessionTimeout         time.Duration `json:"session_timeout"`
	SessionCleanupInterval time.Duration `json:"session_cleanup_interval"`
}

type JWTConfig struct {
	SecretKey       string        `json:"secret_key"`
	PrivateKey      string        `json:"private_key"`  // RSA private key in PEM format
	PublicKey       string        `json:"public_key"`   // RSA public key in PEM format
	UseRSAKeys      bool          `json:"use_rsa_keys"` // Whether to use RSA keys instead of secret key
	AccessTokenTTL  time.Duration `json:"access_token_ttl"`
	RefreshTokenTTL time.Duration `json:"refresh_token_ttl"`
	Issuer          string        `json:"issuer"`
	Audience        string        `json:"audience"`
	Algorithm       string        `json:"algorithm"`
}

type SMSConfig struct {
	ProviderDomain string        `json:"provider_domain"`
	APIKey         string        `json:"api_key"`
	SourceNumber   string        `json:"source_number"`
	RetryCount     int           `json:"retry_count"`
	ValidityPeriod int           `json:"validity_period"`
	Timeout        time.Duration `json:"timeout"`
}

type EmailConfig struct {
	Host          string        `json:"host"`
	Port          int           `json:"port"`
	Username      string        `json:"username"`
	Password      string        `json:"password"`
	FromEmail     string        `json:"from_email"`
	FromName      string        `json:"from_name"`
	UseTLS        bool          `json:"use_tls"`
	UseSTARTTLS   bool          `json:"use_starttls"`
	RateLimit     int           `json:"rate_limit"` // Emails per minute
	RetryAttempts int           `json:"retry_attempts"`
	Timeout       time.Duration `json:"timeout"`
}

type LoggingConfig struct {
	Level            string `json:"level"`  // debug, info, warn, error
	Format           string `json:"format"` // json, text
	Output           string `json:"output"` // stdout, file, both
	FilePath         string `json:"file_path"`
	MaxSize          int    `json:"max_size"` // MB
	MaxBackups       int    `json:"max_backups"`
	MaxAge           int    `json:"max_age"` // days
	Compress         bool   `json:"compress"`
	EnableCaller     bool   `json:"enable_caller"`
	EnableStacktrace bool   `json:"enable_stacktrace"`

	// Access Logs
	EnableAccessLog bool   `json:"enable_access_log"`
	AccessLogPath   string `json:"access_log_path"`
	AccessLogFormat string `json:"access_log_format"`

	// Audit Logs
	EnableAuditLog bool   `json:"enable_audit_log"`
	AuditLogPath   string `json:"audit_log_path"`

	// Security Logs
	EnableSecurityLog bool   `json:"enable_security_log"`
	SecurityLogPath   string `json:"security_log_path"`
}

type MetricsConfig struct {
	Enabled     bool   `json:"enabled"`
	Port        int    `json:"port"`
	Path        string `json:"path"`
	EnablePprof bool   `json:"enable_pprof"`
	PprofPort   int    `json:"pprof_port"`

	// Prometheus
	EnablePrometheus bool   `json:"enable_prometheus"`
	PrometheusPath   string `json:"prometheus_path"`

	// Custom Metrics
	CollectDBMetrics    bool `json:"collect_db_metrics"`
	CollectCacheMetrics bool `json:"collect_cache_metrics"`
	CollectAppMetrics   bool `json:"collect_app_metrics"`
}

type CacheConfig struct {
	Enabled         bool          `json:"enabled"`
	Provider        string        `json:"provider"` // redis, memory
	RedisURL        string        `json:"redis_url"`
	RedisDB         int           `json:"redis_db"`
	RedisPrefix     string        `json:"redis_prefix"`
	DefaultTTL      time.Duration `json:"default_ttl"`
	MaxMemory       int           `json:"max_memory"` // MB
	CleanupInterval time.Duration `json:"cleanup_interval"`
}

type DeploymentConfig struct {
	// Domain Configuration
	Domain           string `json:"domain"`
	APIDomain        string `json:"api_domain"`
	MonitoringDomain string `json:"monitoring_domain"`

	// SSL/TLS Configuration
	CertbotEmail string `json:"certbot_email"`

	// Monitoring Configuration
	GrafanaAdminPassword string `json:"grafana_admin_password"`

	// Additional Security
	RedisPassword string `json:"redis_password"`

	// Backup Configuration
	BackupS3Bucket    string `json:"backup_s3_bucket"`
	BackupS3AccessKey string `json:"backup_s3_access_key"`
	BackupS3SecretKey string `json:"backup_s3_secret_key"`

	// Build Information
	Environment string `json:"environment"`
	Version     string `json:"version"`
	CommitHash  string `json:"commit_hash"`
	BuildTime   string `json:"build_time"`
}

type AtipayConfig struct {
	APIKey   string `json:"api_key"`
	Terminal string `json:"terminal"`
}

type AdminConfig struct {
	Mobile string `json:"mobile"`
}

// LoadProductionConfig loads and validates configuration from environment variables
func LoadProductionConfig() (*ProductionConfig, error) {
	// Load environment variables from .env file
	if err := loadEnvFile(); err != nil {
		return nil, fmt.Errorf("failed to load .env file: %w", err)
	}

	cfg := &ProductionConfig{
		Database: DatabaseConfig{
			Host:            getEnvString("DB_HOST", "localhost"),
			Port:            getEnvInt("DB_PORT", 5432),
			Name:            getEnvString("DB_NAME", "postgres"),
			User:            getEnvString("DB_USER", "postgres"),
			Password:        getEnvString("DB_PASSWORD", ""),
			SSLMode:         getEnvString("DB_SSL_MODE", "require"),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 100),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
			ConnMaxIdleTime: getEnvDuration("DB_CONN_MAX_IDLE_TIME", 15*time.Minute),
			SlowQueryLog:    getEnvBool("DB_SLOW_QUERY_LOG", true),
			SlowQueryTime:   getEnvDuration("DB_SLOW_QUERY_TIME", 1*time.Second),
		},
		Server: ServerConfig{
			Host:              getEnvString("SERVER_HOST", "0.0.0.0"),
			Port:              getEnvInt("SERVER_PORT", 8080),
			ReadTimeout:       getEnvDuration("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout:      getEnvDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:       getEnvDuration("SERVER_IDLE_TIMEOUT", 120*time.Second),
			ShutdownTimeout:   getEnvDuration("SERVER_SHUTDOWN_TIMEOUT", 30*time.Second),
			BodyLimit:         getEnvInt("SERVER_BODY_LIMIT", 4*1024*1024), // 4MB
			EnablePprof:       getEnvBool("SERVER_ENABLE_PPROF", false),
			EnableMetrics:     getEnvBool("SERVER_ENABLE_METRICS", true),
			TrustedProxies:    getEnvStringSlice("SERVER_TRUSTED_PROXIES", []string{"127.0.0.1"}),
			ProxyHeader:       getEnvString("SERVER_PROXY_HEADER", "X-Real-IP"),
			EnableCompression: getEnvBool("SERVER_ENABLE_COMPRESSION", true),
			CompressionLevel:  getEnvInt("SERVER_COMPRESSION_LEVEL", 6),
		},
		Security: SecurityConfig{
			TLSEnabled:             getEnvBool("TLS_ENABLED", true),
			TLSCertFile:            getEnvString("TLS_CERT_FILE", "/etc/ssl/certs/yamata.crt"),
			TLSKeyFile:             getEnvString("TLS_KEY_FILE", "/etc/ssl/private/yamata.key"),
			TLSMinVersion:          getEnvString("TLS_MIN_VERSION", "1.3"),
			HSTSMaxAge:             getEnvInt("HSTS_MAX_AGE", 31536000), // 1 year
			HSTSIncludeSubDoms:     getEnvBool("HSTS_INCLUDE_SUBDOMAINS", true),
			HSTSPreload:            getEnvBool("HSTS_PRELOAD", true),
			AllowedOrigins:         getEnvStringSlice("CORS_ALLOWED_ORIGINS", []string{"https://yamata-no-orochi.com", "https://api.yamata-no-orochi.com", "https://wwww.yamata-no-orochi.com", "https://monitoring.yamata-no-orochi.com", "https://admin.yamata-no-orochi.com"}),
			AllowedMethods:         getEnvStringSlice("CORS_ALLOWED_METHODS", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
			AllowedHeaders:         getEnvStringSlice("CORS_ALLOWED_HEADERS", []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With", "X-API-Key"}),
			AllowCredentials:       getEnvBool("CORS_ALLOW_CREDENTIALS", true),
			CORSMaxAge:             getEnvInt("CORS_MAX_AGE", 86400),
			AuthRateLimit:          getEnvInt("AUTH_RATE_LIMIT", 20),
			GlobalRateLimit:        getEnvInt("GLOBAL_RATE_LIMIT", 2000),
			RateLimitWindow:        getEnvDuration("RATE_LIMIT_WINDOW", 1*time.Minute),
			RateLimitMemory:        getEnvInt("RATE_LIMIT_MEMORY", 64), // MB
			CSPPolicy:              getEnvString("CSP_POLICY", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'"),
			XFrameOptions:          getEnvString("X_FRAME_OPTIONS", "DENY"),
			XContentTypeOptions:    getEnvString("X_CONTENT_TYPE_OPTIONS", "nosniff"),
			XSSProtection:          getEnvString("XSS_PROTECTION", "1; mode=block"),
			ReferrerPolicy:         getEnvString("REFERRER_POLICY", "strict-origin-when-cross-origin"),
			RequireAPIKey:          getEnvBool("REQUIRE_API_KEY", false),
			APIKeyHeader:           getEnvString("API_KEY_HEADER", "X-API-Key"),
			AllowedAPIKeys:         getEnvStringSlice("ALLOWED_API_KEYS", []string{}),
			IPWhitelist:            getEnvStringSlice("IP_WHITELIST", []string{}),
			IPBlacklist:            getEnvStringSlice("IP_BLACKLIST", []string{}),
			PasswordMinLength:      getEnvInt("PASSWORD_MIN_LENGTH", 8),
			PasswordRequireUpper:   getEnvBool("PASSWORD_REQUIRE_UPPER", true),
			PasswordRequireLower:   getEnvBool("PASSWORD_REQUIRE_LOWER", true),
			PasswordRequireNum:     getEnvBool("PASSWORD_REQUIRE_NUMBER", true),
			PasswordRequireSymbol:  getEnvBool("PASSWORD_REQUIRE_SYMBOL", true),
			BcryptCost:             getEnvInt("BCRYPT_COST", 12),
			SessionCookieSecure:    getEnvBool("SESSION_COOKIE_SECURE", true),
			SessionCookieHTTPOnly:  getEnvBool("SESSION_COOKIE_HTTPONLY", true),
			SessionCookieSameSite:  getEnvString("SESSION_COOKIE_SAMESITE", "Strict"),
			SessionTimeout:         getEnvDuration("SESSION_TIMEOUT", 24*time.Hour),
			SessionCleanupInterval: getEnvDuration("SESSION_CLEANUP_INTERVAL", 1*time.Hour),
		},
		JWT: JWTConfig{
			SecretKey:       getEnvString("JWT_SECRET_KEY", ""),
			PrivateKey:      getEnvString("JWT_PRIVATE_KEY", ""),
			PublicKey:       getEnvString("JWT_PUBLIC_KEY", ""),
			UseRSAKeys:      getEnvBool("JWT_USE_RSA_KEYS", false),
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TOKEN_TTL", 24*time.Hour),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TOKEN_TTL", 7*24*time.Hour),
			Issuer:          getEnvString("JWT_ISSUER", "yamata-no-orochi"),
			Audience:        getEnvString("JWT_AUDIENCE", "yamata-no-orochi-api"),
			Algorithm:       getEnvString("JWT_ALGORITHM", "HS256"),
		},
		SMS: SMSConfig{
			ProviderDomain: getEnvString("SMS_PROVIDER_DOMAIN", "mock"),
			APIKey:         getEnvString("SMS_API_KEY", ""),
			SourceNumber:   getEnvString("SMS_SOURCE_NUMBER", ""),
			RetryCount:     getEnvInt("SMS_RETRY_COUNT", 3),
			ValidityPeriod: getEnvInt("SMS_VALIDITY_PERIOD", 300),
			Timeout:        getEnvDuration("SMS_TIMEOUT", 30*time.Second),
		},
		Email: EmailConfig{
			Host:          getEnvString("EMAIL_HOST", "smtp.gmail.com"),
			Port:          getEnvInt("EMAIL_PORT", 587),
			Username:      getEnvString("EMAIL_USERNAME", ""),
			Password:      getEnvString("EMAIL_PASSWORD", ""),
			FromEmail:     getEnvString("EMAIL_FROM_EMAIL", "noreply@yamata-no-orochi.com"),
			FromName:      getEnvString("EMAIL_FROM_NAME", "Yamata no Orochi"),
			UseTLS:        getEnvBool("EMAIL_USE_TLS", true),
			UseSTARTTLS:   getEnvBool("EMAIL_USE_STARTTLS", true),
			RateLimit:     getEnvInt("EMAIL_RATE_LIMIT", 100),
			RetryAttempts: getEnvInt("EMAIL_RETRY_ATTEMPTS", 3),
			Timeout:       getEnvDuration("EMAIL_TIMEOUT", 30*time.Second),
		},
		Logging: LoggingConfig{
			Level:             getEnvString("LOG_LEVEL", "info"),
			Format:            getEnvString("LOG_FORMAT", "json"),
			Output:            getEnvString("LOG_OUTPUT", "file"),
			FilePath:          getEnvString("LOG_FILE_PATH", "/var/log/yamata/app.log"),
			MaxSize:           getEnvInt("LOG_MAX_SIZE", 100),
			MaxBackups:        getEnvInt("LOG_MAX_BACKUPS", 10),
			MaxAge:            getEnvInt("LOG_MAX_AGE", 30),
			Compress:          getEnvBool("LOG_COMPRESS", true),
			EnableCaller:      getEnvBool("LOG_ENABLE_CALLER", true),
			EnableStacktrace:  getEnvBool("LOG_ENABLE_STACKTRACE", true),
			EnableAccessLog:   getEnvBool("LOG_ENABLE_ACCESS", true),
			AccessLogPath:     getEnvString("LOG_ACCESS_PATH", "/var/log/yamata/access.log"),
			AccessLogFormat:   getEnvString("LOG_ACCESS_FORMAT", "combined"),
			EnableAuditLog:    getEnvBool("LOG_ENABLE_AUDIT", true),
			AuditLogPath:      getEnvString("LOG_AUDIT_PATH", "/var/log/yamata/audit.log"),
			EnableSecurityLog: getEnvBool("LOG_ENABLE_SECURITY", true),
			SecurityLogPath:   getEnvString("LOG_SECURITY_PATH", "/var/log/yamata/security.log"),
		},
		Metrics: MetricsConfig{
			Enabled:             getEnvBool("METRICS_ENABLED", true),
			Port:                getEnvInt("METRICS_PORT", 9090),
			Path:                getEnvString("METRICS_PATH", "/metrics"),
			EnablePprof:         getEnvBool("METRICS_ENABLE_PPROF", false),
			PprofPort:           getEnvInt("METRICS_PPROF_PORT", 6060),
			EnablePrometheus:    getEnvBool("METRICS_ENABLE_PROMETHEUS", true),
			PrometheusPath:      getEnvString("METRICS_PROMETHEUS_PATH", "/prometheus"),
			CollectDBMetrics:    getEnvBool("METRICS_COLLECT_DB", true),
			CollectCacheMetrics: getEnvBool("METRICS_COLLECT_CACHE", true),
			CollectAppMetrics:   getEnvBool("METRICS_COLLECT_APP", true),
		},
		Cache: CacheConfig{
			Enabled:         getEnvBool("CACHE_ENABLED", true),
			Provider:        getEnvString("CACHE_PROVIDER", "redis"),
			RedisURL:        getEnvString("CACHE_REDIS_URL", "redis://localhost:6379"),
			RedisDB:         getEnvInt("CACHE_REDIS_DB", 0),
			RedisPrefix:     getEnvString("CACHE_REDIS_PREFIX", "yamata:"),
			DefaultTTL:      getEnvDuration("CACHE_DEFAULT_TTL", 1*time.Hour),
			MaxMemory:       getEnvInt("CACHE_MAX_MEMORY", 256),
			CleanupInterval: getEnvDuration("CACHE_CLEANUP_INTERVAL", 10*time.Minute),
		},
		Deployment: DeploymentConfig{
			Domain:               getEnvString("DOMAIN", "your-domain.com"),
			APIDomain:            getEnvString("API_DOMAIN", "api.your-domain.com"),
			MonitoringDomain:     getEnvString("MONITORING_DOMAIN", "monitoring.your-domain.com"),
			CertbotEmail:         getEnvString("CERTBOT_EMAIL", "admin@your-domain.com"),
			GrafanaAdminPassword: getEnvString("GRAFANA_ADMIN_PASSWORD", ""),
			RedisPassword:        getEnvString("REDIS_PASSWORD", ""),
			BackupS3Bucket:       getEnvString("BACKUP_S3_BUCKET", ""),
			BackupS3AccessKey:    getEnvString("BACKUP_S3_ACCESS_KEY", ""),
			BackupS3SecretKey:    getEnvString("BACKUP_S3_SECRET_KEY", ""),
			Environment:          getEnvString("APP_ENV", "production"),
			Version:              getEnvString("VERSION", "1.0.0"),
			CommitHash:           getEnvString("COMMIT_HASH", "unknown"),
			BuildTime:            getEnvString("BUILD_TIME", "unknown"),
		},
		Atipay: AtipayConfig{
			APIKey:   getEnvString("ATIPAY_API_KEY", ""),
			Terminal: getEnvString("ATIPAY_TERMINAL", ""),
		},
		Admin: AdminConfig{
			Mobile: getEnvString("ADMIN_MOBILE", ""),
		},
	}

	// Validate the loaded configuration
	if err := ValidateProductionConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadEnvFile loads environment variables from .env file if it exists
func loadEnvFile() error {
	envFile := ".env"

	// Check if .env file exists
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		// .env file doesn't exist, continue with environment variables
		return nil
	}

	// Open .env file
	file, err := os.Open(envFile)
	if err != nil {
		return fmt.Errorf("failed to open .env file: %w", err)
	}
	defer file.Close()

	// Read file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key=value pairs
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])

				// Remove quotes if present
				if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
					(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
					value = value[1 : len(value)-1]
				}

				// Set environment variable if not already set
				if os.Getenv(key) == "" {
					os.Setenv(key, value)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading .env file: %w", err)
	}

	return nil
}

// Helper functions for environment variable parsing
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvStringSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// Use standard library strings.Split and strings.TrimSpace
		var result []string
		for _, item := range strings.Split(value, ",") {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}

// ValidateProductionConfig validates the production configuration
func ValidateProductionConfig(cfg *ProductionConfig) error {
	var errors []string

	// Validate database configuration
	if cfg.Database.Host == "" {
		errors = append(errors, "DB_HOST is required")
	}
	if cfg.Database.Port <= 0 || cfg.Database.Port > 65535 {
		errors = append(errors, "DB_PORT must be between 1 and 65535")
	}
	if cfg.Database.Name == "" {
		errors = append(errors, "DB_NAME is required")
	}
	if cfg.Database.User == "" {
		errors = append(errors, "DB_USER is required")
	}
	if cfg.Database.Password == "" {
		errors = append(errors, "DB_PASSWORD is required")
	}

	// Validate JWT configuration
	if cfg.JWT.SecretKey == "" {
		errors = append(errors, "JWT_SECRET_KEY is required")
	}
	if len(cfg.JWT.SecretKey) < 32 {
		errors = append(errors, "JWT_SECRET_KEY must be at least 32 characters long")
	}
	if cfg.JWT.AccessTokenTTL <= 0 {
		errors = append(errors, "JWT_ACCESS_TOKEN_TTL must be positive")
	}
	if cfg.JWT.RefreshTokenTTL <= 0 {
		errors = append(errors, "JWT_REFRESH_TOKEN_TTL must be positive")
	}
	if cfg.JWT.Issuer == "" {
		errors = append(errors, "JWT_ISSUER is required")
	}
	if cfg.JWT.Audience == "" {
		errors = append(errors, "JWT_AUDIENCE is required")
	}

	// Validate server configuration
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		errors = append(errors, "SERVER_PORT must be between 1 and 65535")
	}
	if cfg.Server.ReadTimeout <= 0 {
		errors = append(errors, "SERVER_READ_TIMEOUT must be positive")
	}
	if cfg.Server.WriteTimeout <= 0 {
		errors = append(errors, "SERVER_WRITE_TIMEOUT must be positive")
	}
	if cfg.Server.IdleTimeout <= 0 {
		errors = append(errors, "SERVER_IDLE_TIMEOUT must be positive")
	}

	// Validate security configuration
	if cfg.Security.PasswordMinLength < 6 {
		errors = append(errors, "PASSWORD_MIN_LENGTH must be at least 6")
	}
	if cfg.Security.BcryptCost < 10 || cfg.Security.BcryptCost > 14 {
		errors = append(errors, "BCRYPT_COST must be between 10 and 14")
	}

	// Validate SMS configuration if enabled
	if cfg.SMS.ProviderDomain != "mock" {
		if cfg.SMS.APIKey == "" {
			errors = append(errors, "SMS_API_KEY is required for SMS provider")
		}
		if cfg.SMS.SourceNumber == "" {
			errors = append(errors, "SMS_SOURCE_NUMBER is required for SMS provider")
		}
	}

	// Validate email configuration if enabled
	if cfg.Email.Host != "" {
		if cfg.Email.Username == "" {
			errors = append(errors, "EMAIL_USERNAME is required for email configuration")
		}
		if cfg.Email.Password == "" {
			errors = append(errors, "EMAIL_PASSWORD is required for email configuration")
		}
		if cfg.Email.FromEmail == "" {
			errors = append(errors, "EMAIL_FROM_EMAIL is required for email configuration")
		}
	}

	// Validate TLS configuration if enabled
	if cfg.Security.TLSEnabled {
		if cfg.Security.TLSCertFile == "" {
			errors = append(errors, "TLS_CERT_FILE is required when TLS is enabled")
		}
		if cfg.Security.TLSKeyFile == "" {
			errors = append(errors, "TLS_KEY_FILE is required when TLS is enabled")
		}
	}

	// Validate logging configuration
	if cfg.Logging.Level != "" {
		validLevels := []string{"debug", "info", "warn", "error"}
		valid := false
		for _, level := range validLevels {
			if cfg.Logging.Level == level {
				valid = true
				break
			}
		}
		if !valid {
			errors = append(errors, fmt.Sprintf("LOG_LEVEL must be one of: %v", validLevels))
		}
	}

	// Validate cache configuration if enabled
	if cfg.Cache.Enabled {
		if cfg.Cache.Provider == "redis" && cfg.Cache.RedisURL == "" {
			errors = append(errors, "CACHE_REDIS_URL is required when cache is enabled with redis provider")
		}
	}

	// Return validation errors if any
	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}
