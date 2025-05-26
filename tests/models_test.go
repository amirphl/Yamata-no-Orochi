// Package tests contains test cases for models and repository packages to avoid circular imports
package tests

import (
	"testing"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	testingutil "github.com/amirphl/Yamata-no-Orochi/testing"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestAccountType(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		t.Run("AccountTypeConstants", func(t *testing.T) {
			assert.Equal(t, "individual", models.AccountTypeIndividual)
			assert.Equal(t, "independent_company", models.AccountTypeIndependentCompany)
			assert.Equal(t, "marketing_agency", models.AccountTypeMarketingAgency)
		})

		t.Run("TableName", func(t *testing.T) {
			accountType := &models.AccountType{}
			assert.Equal(t, "account_types", accountType.TableName())
		})

		return nil
	})
	require.NoError(t, err)
}

func TestCustomer(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		fixtures := testingutil.NewTestFixtures(testDB)

		t.Run("CreateIndividualCustomer", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)
			assert.NotZero(t, customer.ID)
			assert.Equal(t, "John", customer.RepresentativeFirstName)
			assert.Equal(t, "Doe", customer.RepresentativeLastName)
			assert.True(t, utils.IsTrue(customer.IsActive))
			assert.False(t, utils.IsTrue(customer.IsEmailVerified))
			assert.False(t, utils.IsTrue(customer.IsMobileVerified))
			assert.NotNil(t, customer.NationalID)
			assert.Nil(t, customer.CompanyName)
		})

		t.Run("CreateCompanyCustomer", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndependentCompany)
			require.NoError(t, err)
			assert.NotZero(t, customer.ID)
			assert.NotNil(t, customer.CompanyName)
			assert.Equal(t, "Test Company Ltd", *customer.CompanyName)
			assert.NotNil(t, customer.CompanyPhone)
			assert.NotNil(t, customer.CompanyAddress)
			assert.NotNil(t, customer.PostalCode)
		})

		t.Run("CreateMarketingAgencyCustomer", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeMarketingAgency)
			require.NoError(t, err)
			assert.NotZero(t, customer.ID)
			assert.NotNil(t, customer.CompanyName)
			assert.Equal(t, "Test Company Ltd", *customer.CompanyName)
		})

		t.Run("PasswordHashing", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Verify password hash was created
			assert.NotEmpty(t, customer.PasswordHash)

			// Verify we can validate the password
			err = bcrypt.CompareHashAndPassword([]byte(customer.PasswordHash), []byte("TestPass123!"))
			assert.NoError(t, err)

			// Verify wrong password fails
			err = bcrypt.CompareHashAndPassword([]byte(customer.PasswordHash), []byte("WrongPassword"))
			assert.Error(t, err)
		})

		t.Run("TableName", func(t *testing.T) {
			customer := &models.Customer{}
			assert.Equal(t, "customers", customer.TableName())
		})

		t.Run("UniqueConstraints", func(t *testing.T) {
			customer1, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Try to create another customer with same email
			customer2 := &models.Customer{
				AccountTypeID:           customer1.AccountTypeID,
				RepresentativeFirstName: "Jane",
				RepresentativeLastName:  "Doe",
				RepresentativeMobile:    "+989123456788", // Different mobile
				Email:                   customer1.Email, // Same email
				PasswordHash:            "hashedpassword",
				IsActive:                utils.ToPtr(true),
			}

			err = testDB.DB.Create(customer2).Error
			assert.Error(t, err) // Should fail due to unique constraint on email

			// Try to create another customer with same mobile
			customer3 := &models.Customer{
				AccountTypeID:           customer1.AccountTypeID,
				RepresentativeFirstName: "Jane",
				RepresentativeLastName:  "Doe",
				RepresentativeMobile:    customer1.RepresentativeMobile, // Same mobile
				Email:                   "jane@example.com",             // Different email
				PasswordHash:            "hashedpassword",
				IsActive:                utils.ToPtr(true),
			}

			err = testDB.DB.Create(customer3).Error
			assert.Error(t, err) // Should fail due to unique constraint on mobile
		})

		return nil
	})
	require.NoError(t, err)
}

func TestOTPVerification(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		fixtures := testingutil.NewTestFixtures(testDB)

		t.Run("CreateOTPVerification", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otp, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			assert.NotZero(t, otp.ID)
			assert.Equal(t, customer.ID, otp.CustomerID)
			assert.Equal(t, "123456", otp.OTPCode)
			assert.Equal(t, models.OTPTypeMobile, otp.OTPType)
			assert.Equal(t, models.OTPStatusPending, otp.Status)
			assert.Equal(t, 0, otp.AttemptsCount)
			assert.Equal(t, 3, otp.MaxAttempts)
			assert.True(t, otp.ExpiresAt.After(time.Now()))
		})

		t.Run("OTPConstants", func(t *testing.T) {
			assert.Equal(t, "mobile", models.OTPTypeMobile)
			assert.Equal(t, "email", models.OTPTypeEmail)
			assert.Equal(t, "password_reset", models.OTPTypePasswordReset)

			assert.Equal(t, "pending", models.OTPStatusPending)
			assert.Equal(t, "verified", models.OTPStatusVerified)
			assert.Equal(t, "expired", models.OTPStatusExpired)
			assert.Equal(t, "failed", models.OTPStatusFailed)
			assert.Equal(t, "used", models.OTPStatusUsed)
		})

		t.Run("ExpiredOTP", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			expiredOTP, err := fixtures.CreateExpiredOTP(customer.ID)
			require.NoError(t, err)

			assert.True(t, time.Now().After(expiredOTP.ExpiresAt))
		})

		t.Run("CanAttemptMethod", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otp, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			// Fresh OTP should allow attempts
			assert.True(t, otp.CanAttempt())

			// Update attempts to max
			otp.AttemptsCount = 3
			err = testDB.DB.Save(otp).Error
			require.NoError(t, err)

			// Reload from DB
			err = testDB.DB.First(otp, otp.ID).Error
			require.NoError(t, err)

			// Should not allow more attempts
			assert.False(t, otp.CanAttempt())
		})

		t.Run("TableName", func(t *testing.T) {
			otp := &models.OTPVerification{}
			assert.Equal(t, "otp_verifications", otp.TableName())
		})

		return nil
	})
	require.NoError(t, err)
}

func TestOTPVerificationCorrelationID(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		fixtures := testingutil.NewTestFixtures(testDB)

		t.Run("CorrelationIDGeneration", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otp, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			// Verify correlation ID is generated and not zero
			assert.NotEqual(t, uuid.Nil, otp.CorrelationID)
			assert.NotEmpty(t, otp.CorrelationID.String())
		})

		t.Run("CorrelationIDUniqueness", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otp1, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			otp2, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeEmail, "654321")
			require.NoError(t, err)

			// Verify different OTPs have different correlation IDs
			assert.NotEqual(t, otp1.CorrelationID, otp2.CorrelationID)
		})

		t.Run("CorrelationIDPersistence", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otp, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			// Reload from database
			var reloadedOTP models.OTPVerification
			err = testDB.DB.First(&reloadedOTP, otp.ID).Error
			require.NoError(t, err)

			// Verify correlation ID persists
			assert.Equal(t, otp.CorrelationID, reloadedOTP.CorrelationID)
		})

		return nil
	})
	require.NoError(t, err)
}

func TestCustomerSession(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		fixtures := testingutil.NewTestFixtures(testDB)

		t.Run("CreateCustomerSession", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			session, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			assert.NotZero(t, session.ID)
			assert.Equal(t, customer.ID, session.CustomerID)
			assert.NotNil(t, session.RefreshToken)
			assert.True(t, utils.IsTrue(session.IsActive))
			assert.True(t, session.ExpiresAt.After(time.Now()))
			assert.NotNil(t, session.IPAddress)
			assert.NotNil(t, session.UserAgent)
		})

		t.Run("SessionValidation", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			session, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			// Active, non-expired session should be valid
			assert.True(t, session.IsValid())

			// Inactive session should be invalid
			session.IsActive = utils.ToPtr(false)
			assert.False(t, session.IsValid())

			// Reset to active
			session.IsActive = utils.ToPtr(true)
			assert.True(t, session.IsValid())

			// Expired session should be invalid
			session.ExpiresAt = time.Now().Add(-1 * time.Hour)
			assert.False(t, session.IsValid())
		})

		t.Run("TableName", func(t *testing.T) {
			session := &models.CustomerSession{}
			assert.Equal(t, "customer_sessions", session.TableName())
		})

		return nil
	})
	require.NoError(t, err)
}

func TestCustomerSessionCorrelationID(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		fixtures := testingutil.NewTestFixtures(testDB)

		t.Run("CorrelationIDGeneration", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			session, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			// Verify correlation ID is generated and not zero
			assert.NotEqual(t, uuid.Nil, session.CorrelationID)
			assert.NotEmpty(t, session.CorrelationID.String())
		})

		t.Run("CorrelationIDUniqueness", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			session1, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			session2, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			// Verify different sessions have different correlation IDs
			assert.NotEqual(t, session1.CorrelationID, session2.CorrelationID)
		})

		t.Run("CorrelationIDPersistence", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			session, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			// Reload from database
			var reloadedSession models.CustomerSession
			err = testDB.DB.First(&reloadedSession, session.ID).Error
			require.NoError(t, err)

			// Verify correlation ID persists
			assert.Equal(t, session.CorrelationID, reloadedSession.CorrelationID)
		})

		return nil
	})
	require.NoError(t, err)
}

func TestAuditLog(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		fixtures := testingutil.NewTestFixtures(testDB)

		t.Run("CreateAuditLog", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			audit, err := fixtures.CreateTestAuditLog(&customer.ID, models.AuditActionLoginSuccess, true)
			require.NoError(t, err)

			assert.NotZero(t, audit.ID)
			assert.Equal(t, customer.ID, *audit.CustomerID)
			assert.Equal(t, models.AuditActionLoginSuccess, audit.Action)
			assert.True(t, *audit.Success)
			assert.NotNil(t, audit.Description)
			assert.NotNil(t, audit.IPAddress)
			assert.NotNil(t, audit.UserAgent)
		})

		t.Run("AuditLogWithoutCustomer", func(t *testing.T) {
			audit, err := fixtures.CreateTestAuditLog(nil, models.AuditActionLoginFailed, false)
			require.NoError(t, err)

			assert.NotZero(t, audit.ID)
			assert.Nil(t, audit.CustomerID)
			assert.Equal(t, models.AuditActionLoginFailed, audit.Action)
			assert.False(t, *audit.Success)
		})

		t.Run("AuditActionConstants", func(t *testing.T) {
			assert.Equal(t, "signup_initiated", models.AuditActionSignupInitiated)
			assert.Equal(t, "signup_completed", models.AuditActionSignupCompleted)
			assert.Equal(t, "login_success", models.AuditActionLoginSuccess)
			assert.Equal(t, "login_failed", models.AuditActionLoginFailed)
			assert.Equal(t, "password_reset_requested", models.AuditActionPasswordResetRequested)
			assert.Equal(t, "password_reset_completed", models.AuditActionPasswordResetCompleted)
			assert.Equal(t, "otp_verification_failed", models.AuditActionOTPVerificationFailed)
		})

		t.Run("TableName", func(t *testing.T) {
			audit := &models.AuditLog{}
			assert.Equal(t, "audit_log", audit.TableName())
		})

		return nil
	})
	require.NoError(t, err)
}

func TestModelRelationships(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		fixtures := testingutil.NewTestFixtures(testDB)

		t.Run("CustomerAccountTypeRelation", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Load customer with account type
			var customerWithType models.Customer
			err = testDB.DB.Preload("AccountType").First(&customerWithType, customer.ID).Error
			require.NoError(t, err)

			assert.Equal(t, models.AccountTypeIndividual, customerWithType.AccountType.TypeName)
			assert.Equal(t, "Individual", customerWithType.AccountType.DisplayName)
		})

		t.Run("CustomerOTPRelation", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otp1, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			otp2, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeEmail, "654321")
			require.NoError(t, err)

			// Load customer with OTPs
			var customerWithOTPs models.Customer
			err = testDB.DB.Preload("OTPVerifications").First(&customerWithOTPs, customer.ID).Error
			require.NoError(t, err)

			assert.Len(t, customerWithOTPs.OTPVerifications, 2)

			otpIDs := make([]uint, len(customerWithOTPs.OTPVerifications))
			for i, otp := range customerWithOTPs.OTPVerifications {
				otpIDs[i] = otp.ID
			}
			assert.Contains(t, otpIDs, otp1.ID)
			assert.Contains(t, otpIDs, otp2.ID)
		})

		t.Run("CustomerSessionRelation", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			session, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			// Load customer with sessions
			var customerWithSessions models.Customer
			err = testDB.DB.Preload("Sessions").First(&customerWithSessions, customer.ID).Error
			require.NoError(t, err)

			assert.Len(t, customerWithSessions.Sessions, 1)
			assert.Equal(t, session.ID, customerWithSessions.Sessions[0].ID)
		})

		t.Run("CustomerAuditLogRelation", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			audit, err := fixtures.CreateTestAuditLog(&customer.ID, models.AuditActionLoginSuccess, true)
			require.NoError(t, err)

			// Load customer with audit logs
			var customerWithAudits models.Customer
			err = testDB.DB.Preload("AuditLogs").First(&customerWithAudits, customer.ID).Error
			require.NoError(t, err)

			assert.Len(t, customerWithAudits.AuditLogs, 1)
			assert.Equal(t, audit.ID, customerWithAudits.AuditLogs[0].ID)
		})

		return nil
	})
	require.NoError(t, err)
}
