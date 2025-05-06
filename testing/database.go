// Package testing provides test utilities and database setup for testing the authentication system
package testing

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	_ "github.com/lib/pq" // PostgreSQL driver for database/sql
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestDBConfig holds configuration for test database connections
type TestDBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	SSLMode  string
}

// GetTestDBConfig loads test database configuration from environment variables
func GetTestDBConfig() *TestDBConfig {
	config := &TestDBConfig{
		Host:     getEnv("TEST_DB_HOST", "localhost"),
		Port:     getEnvAsInt("TEST_DB_PORT", 5432),
		User:     getEnv("TEST_DB_USER", "postgres"),
		Password: getEnv("TEST_DB_PASSWORD", "postgres"),
		SSLMode:  getEnv("TEST_DB_SSL_MODE", "disable"),
	}
	return config
}

// TestDB represents a test database instance
type TestDB struct {
	DB     *gorm.DB
	Name   string
	config *TestDBConfig
}

// SetupTestDB creates a new test database with a unique name and runs migrations
func SetupTestDB() (*TestDB, error) {
	config := GetTestDBConfig()

	// Generate unique database name using timestamp and random number
	dbName := fmt.Sprintf("yamata_test_%d_%d", time.Now().Unix(), rand.Intn(10000))

	// Connect to PostgreSQL server (without specific database)
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.SSLMode)

	adminDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// Create test database
	err = adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName)).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create test database %s: %w", dbName, err)
	}

	// Close admin connection
	sqlDB, _ := adminDB.DB()
	sqlDB.Close()

	// Connect to the new test database
	testDSN := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, dbName, config.SSLMode)

	testDB, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to test database %s: %w", dbName, err)
	}

	// Run migrations using golang-migrate
	if err := runTestMigrations(testDSN, dbName); err != nil {
		// Clean up on migration failure
		testDB.Exec("DROP DATABASE IF EXISTS " + dbName)
		return nil, fmt.Errorf("failed to run migrations on test database %s: %w", dbName, err)
	}

	return &TestDB{
		DB:     testDB,
		Name:   dbName,
		config: config,
	}, nil
}

// TeardownTestDB drops the test database and closes connections
func (tdb *TestDB) TeardownTestDB() error {
	if tdb.DB == nil {
		return nil
	}

	// Close test database connection
	sqlDB, err := tdb.DB.DB()
	if err == nil {
		sqlDB.Close()
	}

	// Connect to PostgreSQL server to drop the test database
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s sslmode=%s",
		tdb.config.Host, tdb.config.Port, tdb.config.User, tdb.config.Password, tdb.config.SSLMode)

	adminDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Printf("Warning: failed to connect to PostgreSQL for cleanup: %v", err)
		return err
	}
	defer func() {
		sqlDB, _ := adminDB.DB()
		sqlDB.Close()
	}()

	// Force disconnect all connections to the test database
	err = adminDB.Exec(fmt.Sprintf(
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid()",
		tdb.Name)).Error
	if err != nil {
		log.Printf("Warning: failed to terminate connections to test database %s: %v", tdb.Name, err)
	}

	// Drop the test database
	err = adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", tdb.Name)).Error
	if err != nil {
		log.Printf("Warning: failed to drop test database %s: %v", tdb.Name, err)
		return err
	}

	return nil
}

// ClearAllTables removes all data from tables while preserving structure
func (tdb *TestDB) ClearAllTables() error {
	// Order matters due to foreign key constraints
	tables := []string{
		"audit_log",
		"customer_sessions",
		"otp_verifications",
		"customers",
		"account_types",
	}

	for _, table := range tables {
		if err := tdb.DB.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table)).Error; err != nil {
			return fmt.Errorf("failed to truncate table %s: %w", table, err)
		}
	}

	return nil
}

// InsertTestAccountTypes inserts default account types for testing
func (tdb *TestDB) InsertTestAccountTypes() error {
	desc1 := "Personal individual account"
	desc2 := "Independent business company account"
	desc3 := "Marketing agency account that can manage other companies"

	accountTypes := []*models.AccountType{
		{
			TypeName:    models.AccountTypeIndividual,
			DisplayName: "Individual",
			Description: &desc1,
		},
		{
			TypeName:    models.AccountTypeIndependentCompany,
			DisplayName: "Independent Company",
			Description: &desc2,
		},
		{
			TypeName:    models.AccountTypeMarketingAgency,
			DisplayName: "Marketing Agency",
			Description: &desc3,
		},
	}

	for _, accountType := range accountTypes {
		if err := tdb.DB.Create(accountType).Error; err != nil {
			return fmt.Errorf("failed to insert account type %s: %w", accountType.TypeName, err)
		}
	}

	return nil
}

// runTestMigrations runs all database migrations by executing SQL files directly
func runTestMigrations(databaseURL, dbName string) error {
	// Get the absolute path to migrations directory
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// If we're in the tests directory, go up one level to find migrations
	if filepath.Base(wd) == "tests" {
		wd = filepath.Dir(wd)
	}

	migrationsPath := filepath.Join(wd, "migrations")

	// Check if migrations directory exists
	if _, err := os.Stat(migrationsPath); os.IsNotExist(err) {
		return fmt.Errorf("migrations directory not found at %s", migrationsPath)
	}

	// Create a database connection
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	defer db.Close()

	// Test the connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Define migration files in order (excluding down migrations and utility files)
	migrationFiles := []string{
		"0001_create_account_types.sql",
		"0002_create_customers.sql",
		"0003_create_otp_verifications.sql",
		"0004_create_customer_sessions.sql",
		"0005_create_audit_log.sql",
		"0006_update_customer_fields.sql",
	}

	// Execute each migration file
	for _, filename := range migrationFiles {
		migrationPath := filepath.Join(migrationsPath, filename)

		// Check if file exists
		if _, err := os.Stat(migrationPath); os.IsNotExist(err) {
			log.Printf("Warning: migration file %s not found, skipping", filename)
			continue
		}

		// Read migration file
		content, err := os.ReadFile(migrationPath)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", filename, err)
		}

		// Execute migration
		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", filename, err)
		}

		log.Printf("Applied migration: %s", filename)
	}

	log.Printf("Successfully applied %d migrations to test database %s", len(migrationFiles), dbName)
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// TestWithDB is a helper function that sets up a test database, runs the test function, and cleans up
func TestWithDB(testFunc func(*TestDB) error) error {
	testDB, err := SetupTestDB()
	if err != nil {
		return fmt.Errorf("failed to setup test database: %w", err)
	}
	defer func() {
		if cleanupErr := testDB.TeardownTestDB(); cleanupErr != nil {
			log.Printf("Warning: failed to cleanup test database: %v", cleanupErr)
		}
	}()

	// Account types are already inserted by migrations, no need to insert them again

	return testFunc(testDB)
}

// CreateTestContext creates a context for testing
func CreateTestContext() context.Context {
	return context.Background()
}
