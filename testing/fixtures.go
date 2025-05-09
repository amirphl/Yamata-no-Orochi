// Package testing provides test utilities and database setup for testing the authentication system
package testing

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// TestFixtures provides helper methods for creating test data
type TestFixtures struct {
	DB *TestDB
}

// NewTestFixtures creates a new test fixtures instance
func NewTestFixtures(db *TestDB) *TestFixtures {
	return &TestFixtures{DB: db}
}

// CreateTestCustomer creates a test customer with the specified account type
func (tf *TestFixtures) CreateTestCustomer(accountTypeName string) (*models.Customer, error) {
	// Get account type
	var accountType models.AccountType
	err := tf.DB.DB.Where("type_name = ?", accountTypeName).Last(&accountType).Error
	if err != nil {
		return nil, fmt.Errorf("failed to find account type %s: %w", accountTypeName, err)
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("TestPass123!"), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// create random number containin exactly 9 digits
	randomDigits := fmt.Sprintf("%09d", rand.Intn(900000000)+100000000)

	customer := &models.Customer{
		AccountTypeID:           accountType.ID,
		RepresentativeFirstName: "John",
		RepresentativeLastName:  "Doe",
		RepresentativeMobile:    fmt.Sprintf("+989%s", randomDigits),
		Email:                   fmt.Sprintf("john.doe.%d.%s@example.com", accountType.ID, randomDigits),
		PasswordHash:            string(hashedPassword),
		IsActive:                utils.ToPtr(true),
		IsEmailVerified:         utils.ToPtr(false),
		IsMobileVerified:        utils.ToPtr(false),
	}

	// Set account-specific fields
	switch accountTypeName {
	case models.AccountTypeIndividual:
		nationalID := fmt.Sprintf("%010d", rand.Intn(9000000000)+1000000000)
		customer.NationalID = &nationalID
	case models.AccountTypeIndependentCompany, models.AccountTypeMarketingAgency:
		companyName := "Test Company Ltd"
		nationalID := fmt.Sprintf("%010d", rand.Intn(9000000000)+1000000000)
		companyPhone := "02112345678"
		companyAddress := "123 Test Street, Tehran, Iran"
		postalCode := "1234567890"

		customer.CompanyName = &companyName
		customer.NationalID = &nationalID
		customer.CompanyPhone = &companyPhone
		customer.CompanyAddress = &companyAddress
		customer.PostalCode = &postalCode
	}

	err = tf.DB.DB.Create(customer).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create test customer: %w", err)
	}

	return customer, nil
}

// CreateTestOTP creates a test OTP verification record
func (tf *TestFixtures) CreateTestOTP(customerID uint, otpType, otpCode string) (*models.OTPVerification, error) {
	otp := &models.OTPVerification{
		CorrelationID: uuid.New(), // Generate new UUID for correlation
		CustomerID:    customerID,
		OTPCode:       otpCode,
		OTPType:       otpType,
		TargetValue:   "+989123456789",
		Status:        models.OTPStatusPending,
		AttemptsCount: 0,
		MaxAttempts:   3,
		ExpiresAt:     time.Now().Add(5 * time.Minute),
	}

	err := tf.DB.DB.Create(otp).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create test OTP: %w", err)
	}

	return otp, nil
}

func GenerateSecureToken(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// CreateTestSession creates a test customer session
func (tf *TestFixtures) CreateTestSession(customerID uint) (*models.CustomerSession, error) {
	sessionToken, err := GenerateSecureToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate secure session token: %w", err)
	}

	refreshToken, err := GenerateSecureToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate secure refresh token: %w", err)
	}

	ipAddress := "127.0.0.1"
	userAgent := "Test User Agent"

	session := &models.CustomerSession{
		CorrelationID: uuid.New(), // Generate new UUID for correlation
		CustomerID:    customerID,
		SessionToken:  sessionToken,
		RefreshToken:  &refreshToken,
		ExpiresAt:     time.Now().Add(24 * time.Hour),
		IsActive:      utils.ToPtr(true),
		IPAddress:     &ipAddress,
		UserAgent:     &userAgent,
	}

	err = tf.DB.DB.Create(session).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create test session: %w", err)
	}

	return session, nil
}

// CreateTestAuditLog creates a test audit log entry
func (tf *TestFixtures) CreateTestAuditLog(customerID *uint, action string, success bool) (*models.AuditLog, error) {
	description := fmt.Sprintf("Test %s action", action)
	ipAddress := "127.0.0.1"
	userAgent := "Test User Agent"

	audit := &models.AuditLog{
		CustomerID:  customerID,
		Action:      action,
		Description: &description,
		Success:     &success,
		IPAddress:   &ipAddress,
		UserAgent:   &userAgent,
	}

	if !success {
		errorMessage := "Test failed action"
		audit.ErrorMessage = &errorMessage
	}

	err := tf.DB.DB.Create(audit).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create test audit log: %w", err)
	}

	return audit, nil
}

// CreateMultipleTestCustomers creates multiple test customers with different account types
func (tf *TestFixtures) CreateMultipleTestCustomers() ([]*models.Customer, error) {
	accountTypes := []string{
		models.AccountTypeIndividual,
		models.AccountTypeIndependentCompany,
		models.AccountTypeMarketingAgency,
	}

	var customers []*models.Customer
	for i, accountType := range accountTypes {
		customer, err := tf.CreateTestCustomer(accountType)
		if err != nil {
			return nil, fmt.Errorf("failed to create customer %d: %w", i, err)
		}

		// Make each customer unique
		customer.Email = fmt.Sprintf("user%d.%d@example.com", i+1, rand.Intn(10000000))
		customer.RepresentativeMobile = fmt.Sprintf("+989%s", fmt.Sprintf("%09d", rand.Intn(900000000)+100000000))

		err = tf.DB.DB.Save(customer).Error
		if err != nil {
			return nil, fmt.Errorf("failed to update customer %d: %w", i, err)
		}

		customers = append(customers, customer)
	}

	return customers, nil
}

// CreateExpiredOTP creates an expired OTP for testing
func (tf *TestFixtures) CreateExpiredOTP(customerID uint) (*models.OTPVerification, error) {
	otp := &models.OTPVerification{
		CustomerID:    customerID,
		OTPCode:       "123456",
		OTPType:       models.OTPTypeMobile,
		TargetValue:   "+989123456789",
		Status:        models.OTPStatusPending,
		AttemptsCount: 0,
		MaxAttempts:   3,
		ExpiresAt:     time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
	}

	err := tf.DB.DB.Create(otp).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create expired OTP: %w", err)
	}

	return otp, nil
}
