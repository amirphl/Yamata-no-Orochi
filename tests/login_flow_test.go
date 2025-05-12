// Package tests contains integration tests for login flow
package tests

import (
	"context"
	"testing"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/app/services"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	testingutil "github.com/amirphl/Yamata-no-Orochi/testing"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginFlow(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		fixtures := testingutil.NewTestFixtures(testDB)

		// Initialize repositories
		customerRepo := repository.NewCustomerRepository(testDB.DB)
		accountTypeRepo := repository.NewAccountTypeRepository(testDB.DB)
		otpRepo := repository.NewOTPVerificationRepository(testDB.DB)
		sessionRepo := repository.NewCustomerSessionRepository(testDB.DB)
		auditRepo := repository.NewAuditLogRepository(testDB.DB)

		// Initialize services
		tokenService, err := services.NewTokenService(1*time.Hour, 24*time.Hour, "test-issuer", "test-audience")
		require.NoError(t, err)

		notificationService := services.NewNotificationService(
			services.NewMockSMSProvider(),
			services.NewMockEmailProvider(),
		)

		// Initialize business flow
		loginFlow := businessflow.NewLoginFlow(
			customerRepo,
			sessionRepo,
			otpRepo,
			auditRepo,
			accountTypeRepo,
			tokenService,
			notificationService,
			testDB.DB,
		)

		t.Run("SuccessfulLoginWithEmail", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create login request
			loginReq := &dto.LoginRequest{
				Identifier: customer.Email,
				Password:   "TestPass123!",
			}

			// Perform login
			loginResult, err := loginFlow.Login(context.Background(), loginReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, loginResult)
			assert.True(t, loginResult.Success)
			assert.NotEmpty(t, loginResult.Customer)
			assert.NotEmpty(t, loginResult.Session)
			assert.Empty(t, loginResult.ErrorCode)
			assert.Empty(t, loginResult.ErrorMessage)

			// Verify customer data
			assert.Equal(t, customer.ID, loginResult.Customer.ID)
			assert.Equal(t, customer.Email, loginResult.Customer.Email)
			assert.Equal(t, customer.RepresentativeFirstName, loginResult.Customer.RepresentativeFirstName)
			assert.Equal(t, customer.RepresentativeLastName, loginResult.Customer.RepresentativeLastName)
			assert.Equal(t, customer.RepresentativeMobile, loginResult.Customer.RepresentativeMobile)
			assert.True(t, utils.IsTrue(loginResult.Customer.IsActive))

			// Verify session was created
			assert.NotEmpty(t, loginResult.Session.SessionToken)
			assert.NotEmpty(t, loginResult.Session.RefreshToken)
			assert.True(t, utils.IsTrue(loginResult.Session.IsActive))
			assert.True(t, loginResult.Session.ExpiresAt.After(time.Now()))
		})

		t.Run("SuccessfulLoginWithMobile", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create login request
			loginReq := &dto.LoginRequest{
				Identifier: customer.RepresentativeMobile,
				Password:   "TestPass123!",
			}

			// Perform login
			loginResult, err := loginFlow.Login(context.Background(), loginReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, loginResult)
			assert.True(t, loginResult.Success)
			assert.NotEmpty(t, loginResult.Customer)
			assert.NotEmpty(t, loginResult.Session)
		})

		t.Run("UserNotFound", func(t *testing.T) {
			// Create login request with non-existent user
			loginReq := &dto.LoginRequest{
				Identifier: "nonexistent@example.com",
				Password:   "TestPass123!",
			}

			// Perform login
			loginResult, err := loginFlow.Login(context.Background(), loginReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, loginResult)
			assert.False(t, loginResult.Success)
			assert.Nil(t, loginResult.Customer)
			assert.Nil(t, loginResult.Session)
			assert.Equal(t, "USER_NOT_FOUND", loginResult.ErrorCode)
			assert.NotEmpty(t, loginResult.ErrorMessage)
		})

		t.Run("IncorrectPassword", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create login request with wrong password
			loginReq := &dto.LoginRequest{
				Identifier: customer.Email,
				Password:   "WrongPassword123!",
			}

			// Perform login
			loginResult, err := loginFlow.Login(context.Background(), loginReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, loginResult)
			assert.False(t, loginResult.Success)
			assert.Nil(t, loginResult.Customer)
			assert.Nil(t, loginResult.Session)
			assert.Equal(t, "INCORRECT_PASSWORD", loginResult.ErrorCode)
			assert.NotEmpty(t, loginResult.ErrorMessage)
		})

		t.Run("InactiveAccount", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Deactivate the customer
			customer.IsActive = utils.ToPtr(false)
			err = testDB.DB.Save(customer).Error
			require.NoError(t, err)

			// Create login request
			loginReq := &dto.LoginRequest{
				Identifier: customer.Email,
				Password:   "TestPass123!",
			}

			// Perform login
			loginResult, err := loginFlow.Login(context.Background(), loginReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, loginResult)
			assert.False(t, loginResult.Success)
			assert.Nil(t, loginResult.Customer)
			assert.Nil(t, loginResult.Session)
			assert.Equal(t, "ACCOUNT_INACTIVE", loginResult.ErrorCode)
			assert.NotEmpty(t, loginResult.ErrorMessage)
		})

		t.Run("SessionCreationWithCorrelationID", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create login request
			loginReq := &dto.LoginRequest{
				Identifier: customer.Email,
				Password:   "TestPass123!",
			}

			// Perform login
			loginResult, err := loginFlow.Login(context.Background(), loginReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, loginResult)
			assert.True(t, loginResult.Success)

			// Verify session has correlation ID
			assert.NotEqual(t, uuid.Nil, loginResult.Session.CorrelationID)
			assert.NotEmpty(t, loginResult.Session.CorrelationID.String())

			// Get session history
			history, err := sessionRepo.GetHistoryByCorrelationID(context.Background(), loginResult.Session.CorrelationID)
			require.NoError(t, err)
			require.Len(t, history, 1)

			// Verify session in history
			assert.Equal(t, loginResult.Session.ID, history[0].ID)
			assert.Equal(t, loginResult.Session.CorrelationID, history[0].CorrelationID)
			assert.True(t, utils.IsTrue(history[0].IsActive))
		})

		t.Run("AuditLogCreation", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create login request
			loginReq := &dto.LoginRequest{
				Identifier: customer.Email,
				Password:   "TestPass123!",
			}

			// Perform login
			loginResult, err := loginFlow.Login(context.Background(), loginReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, loginResult)
			assert.True(t, loginResult.Success)

			// Check audit logs were created
			auditLogs, err := auditRepo.ByFilter(context.Background(), models.AuditLogFilter{
				CustomerID: &customer.ID,
				Action:     utils.ToPtr(models.AuditActionLoginSuccessful),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, auditLogs, 1)

			// Verify audit log
			auditLog := auditLogs[0]
			assert.Equal(t, customer.ID, *auditLog.CustomerID)
			assert.Equal(t, models.AuditActionLoginSuccessful, auditLog.Action)
			assert.True(t, utils.IsTrue(auditLog.Success))
			assert.NotNil(t, auditLog.IPAddress)
			assert.NotNil(t, auditLog.UserAgent)
		})

		t.Run("FailedLoginAuditLog", func(t *testing.T) {
			// Create login request with non-existent user
			loginReq := &dto.LoginRequest{
				Identifier: "nonexistent@example.com",
				Password:   "TestPass123!",
			}

			// Perform login
			loginResult, err := loginFlow.Login(context.Background(), loginReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, loginResult)
			assert.False(t, loginResult.Success)

			// Check audit log was created for failed login
			_, err = auditRepo.ByFilter(context.Background(), models.AuditLogFilter{
				Action: utils.ToPtr(models.AuditActionLoginFailed),
			}, "", 0, 0)
			require.NoError(t, err)
		})

		t.Run("SessionExpirationWithCorrelationID", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create login request
			loginReq := &dto.LoginRequest{
				Identifier: customer.Email,
				Password:   "TestPass123!",
			}

			// Perform login
			loginResult, err := loginFlow.Login(context.Background(), loginReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, loginResult)
			assert.True(t, loginResult.Success)

			// Expire the session
			err = sessionRepo.ExpireSession(context.Background(), loginResult.Session.ID)
			require.NoError(t, err)

			// Get session history
			history, err := sessionRepo.GetHistoryByCorrelationID(context.Background(), loginResult.Session.CorrelationID)
			require.NoError(t, err)
			require.Len(t, history, 2)

			// Both records should have same correlation ID
			assert.Equal(t, loginResult.Session.CorrelationID, history[0].CorrelationID)
			assert.Equal(t, loginResult.Session.CorrelationID, history[1].CorrelationID)

			// Latest should be inactive
			assert.False(t, utils.IsTrue(history[0].IsActive))
		})

		t.Run("MultipleSessionsForSameCustomer", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create first login
			loginReq1 := &dto.LoginRequest{
				Identifier: customer.Email,
				Password:   "TestPass123!",
			}

			loginResult1, err := loginFlow.Login(context.Background(), loginReq1, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, loginResult1)
			assert.True(t, loginResult1.Success)

			// Create second login
			loginResult2, err := loginFlow.Login(context.Background(), loginReq1, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, loginResult2)
			assert.True(t, loginResult2.Success)

			// Verify both sessions have different correlation IDs
			assert.NotEqual(t, loginResult1.Session.CorrelationID, loginResult2.Session.CorrelationID)

			// Verify both sessions are active
			assert.True(t, utils.IsTrue(loginResult1.Session.IsActive))
			assert.True(t, utils.IsTrue(loginResult2.Session.IsActive))
		})

		t.Run("TokenGeneration", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create login request
			loginReq := &dto.LoginRequest{
				Identifier: customer.Email,
				Password:   "TestPass123!",
			}

			// Perform login
			loginResult, err := loginFlow.Login(context.Background(), loginReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, loginResult)
			assert.True(t, loginResult.Success)

			// Verify tokens were generated
			assert.NotEmpty(t, loginResult.Session.SessionToken)
			assert.NotEmpty(t, loginResult.Session.RefreshToken)

			// Verify tokens are different
			assert.NotEqual(t, loginResult.Session.SessionToken, loginResult.Session.RefreshToken)
		})

		t.Run("AccountTypeRetrieval", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create login request
			loginReq := &dto.LoginRequest{
				Identifier: customer.Email,
				Password:   "TestPass123!",
			}

			// Perform login
			loginResult, err := loginFlow.Login(context.Background(), loginReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, loginResult)
			assert.True(t, loginResult.Success)

			// Verify account type was retrieved
			assert.NotNil(t, loginResult.AccountType)
			assert.Equal(t, models.AccountTypeIndividual, loginResult.AccountType.TypeName)
		})

		return nil
	})
	require.NoError(t, err)
}

func TestLoginFlowHelperFunctions(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		fixtures := testingutil.NewTestFixtures(testDB)

		// Initialize repositories
		customerRepo := repository.NewCustomerRepository(testDB.DB)
		accountTypeRepo := repository.NewAccountTypeRepository(testDB.DB)
		otpRepo := repository.NewOTPVerificationRepository(testDB.DB)
		sessionRepo := repository.NewCustomerSessionRepository(testDB.DB)
		auditRepo := repository.NewAuditLogRepository(testDB.DB)

		// Initialize services
		tokenService, err := services.NewTokenService(1*time.Hour, 24*time.Hour, "test-issuer", "test-audience")
		require.NoError(t, err)

		notificationService := services.NewNotificationService(
			services.NewMockSMSProvider(),
			services.NewMockEmailProvider(),
		)

		// Initialize business flow
		loginFlow := businessflow.NewLoginFlow(
			customerRepo,
			sessionRepo,
			otpRepo,
			auditRepo,
			accountTypeRepo,
			tokenService,
			notificationService,
			testDB.DB,
		)

		t.Run("FindCustomerByIdentifier", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Test finding by email
			foundCustomer, err := loginFlow.(*businessflow.LoginFlowImpl).FindCustomerByIdentifier(context.Background(), customer.Email)
			require.NoError(t, err)
			require.NotNil(t, foundCustomer)
			assert.Equal(t, customer.ID, foundCustomer.ID)
			assert.Equal(t, customer.Email, foundCustomer.Email)

			// Test finding by mobile
			foundCustomer, err = loginFlow.(*businessflow.LoginFlowImpl).FindCustomerByIdentifier(context.Background(), customer.RepresentativeMobile)
			require.NoError(t, err)
			require.NotNil(t, foundCustomer)
			assert.Equal(t, customer.ID, foundCustomer.ID)
			assert.Equal(t, customer.RepresentativeMobile, foundCustomer.RepresentativeMobile)

			// Test non-existent identifier
			foundCustomer, err = loginFlow.(*businessflow.LoginFlowImpl).FindCustomerByIdentifier(context.Background(), "nonexistent@example.com")
			require.Error(t, err)
			assert.Nil(t, foundCustomer)
		})

		t.Run("CreateSession", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create session
			session, err := loginFlow.(*businessflow.LoginFlowImpl).CreateSession(context.Background(), customer.ID, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, session)

			// Verify session properties
			assert.Equal(t, customer.ID, session.CustomerID)
			assert.NotEmpty(t, session.SessionToken)
			assert.NotEmpty(t, session.RefreshToken)
			assert.True(t, utils.IsTrue(session.IsActive))
			assert.True(t, session.ExpiresAt.After(time.Now()))
			assert.NotNil(t, session.IPAddress)
			assert.NotNil(t, session.UserAgent)
			assert.NotEqual(t, uuid.Nil, session.CorrelationID)
		})

		t.Run("LogLoginAttempt", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Log successful login attempt
			err = loginFlow.(*businessflow.LoginFlowImpl).LogLoginAttempt(context.Background(), customer, models.AuditActionLoginSuccessful, "test description", true, nil, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)

			// Check audit log was created
			auditLogs, err := auditRepo.ByFilter(context.Background(), models.AuditLogFilter{
				CustomerID: &customer.ID,
				Action:     utils.ToPtr(models.AuditActionLoginSuccessful),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, auditLogs, 1)

			// Verify audit log
			auditLog := auditLogs[0]
			assert.Equal(t, customer.ID, *auditLog.CustomerID)
			assert.Equal(t, models.AuditActionLoginSuccessful, auditLog.Action)
			assert.True(t, utils.IsTrue(auditLog.Success))
			assert.NotNil(t, auditLog.IPAddress)
			assert.NotNil(t, auditLog.UserAgent)
		})

		t.Run("InvalidateAllSessions", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create multiple sessions
			session1, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			session2, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			// Invalidate all sessions
			err = loginFlow.(*businessflow.LoginFlowImpl).InvalidateAllSessions(context.Background(), customer.ID)
			require.NoError(t, err)

			// Verify both sessions have expired records with same correlation IDs
			history1, err := sessionRepo.GetHistoryByCorrelationID(context.Background(), session1.CorrelationID)
			require.NoError(t, err)
			require.Len(t, history1, 2)
			assert.Equal(t, session1.CorrelationID, history1[0].CorrelationID)
			assert.False(t, utils.IsTrue(history1[0].IsActive))

			history2, err := sessionRepo.GetHistoryByCorrelationID(context.Background(), session2.CorrelationID)
			require.NoError(t, err)
			require.Len(t, history2, 2)
			assert.Equal(t, session2.CorrelationID, history2[0].CorrelationID)
			assert.False(t, utils.IsTrue(history2[0].IsActive))
		})

		return nil
	})
	require.NoError(t, err)
}

func TestForgotPasswordFlow(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		fixtures := testingutil.NewTestFixtures(testDB)

		// Initialize repositories
		customerRepo := repository.NewCustomerRepository(testDB.DB)
		accountTypeRepo := repository.NewAccountTypeRepository(testDB.DB)
		otpRepo := repository.NewOTPVerificationRepository(testDB.DB)
		sessionRepo := repository.NewCustomerSessionRepository(testDB.DB)
		auditRepo := repository.NewAuditLogRepository(testDB.DB)

		// Initialize services
		tokenService, err := services.NewTokenService(1*time.Hour, 24*time.Hour, "test-issuer", "test-audience")
		require.NoError(t, err)

		notificationService := services.NewNotificationService(
			services.NewMockSMSProvider(),
			services.NewMockEmailProvider(),
		)

		// Initialize business flow
		loginFlow := businessflow.NewLoginFlow(
			customerRepo,
			sessionRepo,
			otpRepo,
			auditRepo,
			accountTypeRepo,
			tokenService,
			notificationService,
			testDB.DB,
		)

		t.Run("SuccessfulForgotPasswordWithEmail", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create forgot password request
			forgotReq := &dto.ForgotPasswordRequest{
				Identifier: customer.Email,
			}

			// Perform forgot password
			result, err := loginFlow.ForgotPassword(context.Background(), forgotReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)
			assert.Equal(t, customer.ID, result.CustomerID)
			assert.NotEmpty(t, result.MaskedPhone)
			assert.True(t, result.OTPExpiry.After(time.Now()))
			assert.Empty(t, result.ErrorCode)
			assert.Empty(t, result.ErrorMessage)

			// Verify OTP was created
			otps, err := otpRepo.ByFilter(context.Background(), models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    utils.ToPtr(models.OTPTypePasswordReset),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, otps, 1)

			otp := otps[0]
			assert.Equal(t, models.OTPStatusPending, otp.Status)
			assert.Equal(t, customer.RepresentativeMobile, otp.TargetValue)
			assert.Equal(t, 0, otp.AttemptsCount)
			assert.Equal(t, 3, otp.MaxAttempts)
			assert.True(t, otp.ExpiresAt.After(time.Now()))
			assert.NotEqual(t, uuid.Nil, otp.CorrelationID)
		})

		t.Run("SuccessfulForgotPasswordWithMobile", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create forgot password request
			forgotReq := &dto.ForgotPasswordRequest{
				Identifier: customer.RepresentativeMobile,
			}

			// Perform forgot password
			result, err := loginFlow.ForgotPassword(context.Background(), forgotReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)
			assert.Equal(t, customer.ID, result.CustomerID)
			assert.NotEmpty(t, result.MaskedPhone)
		})

		t.Run("UserNotFound", func(t *testing.T) {
			// Create forgot password request with non-existent user
			forgotReq := &dto.ForgotPasswordRequest{
				Identifier: "nonexistent@example.com",
			}

			// Perform forgot password
			result, err := loginFlow.ForgotPassword(context.Background(), forgotReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.False(t, result.Success)
			assert.Equal(t, uint(0), result.CustomerID)
			assert.Empty(t, result.MaskedPhone)
			assert.Equal(t, dto.ErrorUserNotFound, result.ErrorCode)
			assert.Equal(t, "User not found", result.ErrorMessage)
		})

		t.Run("InactiveAccount", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Deactivate the customer
			customer.IsActive = utils.ToPtr(false)
			err = testDB.DB.Save(customer).Error
			require.NoError(t, err)

			// Create forgot password request
			forgotReq := &dto.ForgotPasswordRequest{
				Identifier: customer.Email,
			}

			// Perform forgot password
			result, err := loginFlow.ForgotPassword(context.Background(), forgotReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.False(t, result.Success)
			assert.Equal(t, uint(0), result.CustomerID)
			assert.Empty(t, result.MaskedPhone)
			assert.Equal(t, dto.ErrorAccountInactive, result.ErrorCode)
			assert.Equal(t, "Please contact support", result.ErrorMessage)
		})

		t.Run("ExpireOldPasswordResetOTPs", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create an old password reset OTP
			oldOTP := &models.OTPVerification{
				CorrelationID: uuid.New(),
				CustomerID:    customer.ID,
				OTPCode:       "123456",
				OTPType:       models.OTPTypePasswordReset,
				TargetValue:   customer.RepresentativeMobile,
				Status:        models.OTPStatusPending,
				AttemptsCount: 0,
				MaxAttempts:   3,
				ExpiresAt:     time.Now().Add(-1 * time.Hour), // Expired
				IPAddress:     utils.ToPtr("127.0.0.1"),
				UserAgent:     utils.ToPtr("Test User Agent"),
			}
			err = testDB.DB.Create(oldOTP).Error
			require.NoError(t, err)

			// Create forgot password request
			forgotReq := &dto.ForgotPasswordRequest{
				Identifier: customer.Email,
			}

			// Perform forgot password
			result, err := loginFlow.ForgotPassword(context.Background(), forgotReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)

			// Verify old OTP was expired and new one was created
			otps, err := otpRepo.ByFilter(context.Background(), models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    utils.ToPtr(models.OTPTypePasswordReset),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, otps, 2+1)

			// Check that one is expired and one is pending
			statuses := make(map[string]int)
			for _, otp := range otps {
				statuses[otp.Status]++
			}
			assert.Equal(t, 1, statuses[models.OTPStatusExpired])
			assert.Equal(t, 2, statuses[models.OTPStatusPending])
		})

		t.Run("OTPGenerationWithCorrelationID", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create forgot password request
			forgotReq := &dto.ForgotPasswordRequest{
				Identifier: customer.Email,
			}

			// Perform forgot password
			result, err := loginFlow.ForgotPassword(context.Background(), forgotReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)

			// Get the created OTP
			otps, err := otpRepo.ByFilter(context.Background(), models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    utils.ToPtr(models.OTPTypePasswordReset),
				Status:     utils.ToPtr(models.OTPStatusPending),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, otps, 1)

			otp := otps[0]
			assert.NotEqual(t, uuid.Nil, otp.CorrelationID)
			assert.NotEmpty(t, otp.CorrelationID.String())

			// Get OTP history by correlation ID
			history, err := otpRepo.GetHistoryByCorrelationID(context.Background(), otp.CorrelationID)
			require.NoError(t, err)
			require.Len(t, history, 1)

			// Verify correlation ID is preserved
			assert.Equal(t, otp.CorrelationID, history[0].CorrelationID)
		})

		t.Run("AuditLogCreation", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create forgot password request
			forgotReq := &dto.ForgotPasswordRequest{
				Identifier: customer.Email,
			}

			// Perform forgot password
			result, err := loginFlow.ForgotPassword(context.Background(), forgotReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)

			// Check audit logs were created
			auditLogs, err := auditRepo.ByFilter(context.Background(), models.AuditLogFilter{
				CustomerID: &customer.ID,
				Action:     utils.ToPtr(models.AuditActionPasswordResetRequested),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, auditLogs, 1)

			// Verify audit log
			auditLog := auditLogs[0]
			assert.Equal(t, customer.ID, *auditLog.CustomerID)
			assert.Equal(t, models.AuditActionPasswordResetRequested, auditLog.Action)
			assert.True(t, utils.IsTrue(auditLog.Success))
			assert.NotNil(t, auditLog.IPAddress)
			assert.NotNil(t, auditLog.UserAgent)
		})

		t.Run("FailedForgotPasswordAuditLog", func(t *testing.T) {
			// Create forgot password request with non-existent user
			forgotReq := &dto.ForgotPasswordRequest{
				Identifier: "nonexistent@example.com",
			}

			// Perform forgot password
			result, err := loginFlow.ForgotPassword(context.Background(), forgotReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.False(t, result.Success)

			// Check audit log was created for failed request
			_, err = auditRepo.ByFilter(context.Background(), models.AuditLogFilter{
				Action: utils.ToPtr(models.AuditActionPasswordResetRequested),
			}, "", 0, 0)
			require.NoError(t, err)
		})

		t.Run("MultipleForgotPasswordRequests", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create first forgot password request
			forgotReq := &dto.ForgotPasswordRequest{
				Identifier: customer.Email,
			}

			result1, err := loginFlow.ForgotPassword(context.Background(), forgotReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result1)
			assert.True(t, result1.Success)

			// Create second forgot password request
			result2, err := loginFlow.ForgotPassword(context.Background(), forgotReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result2)
			assert.True(t, result2.Success)

			// Verify both OTPs were created with different correlation IDs
			otps, err := otpRepo.ByFilter(context.Background(), models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    utils.ToPtr(models.OTPTypePasswordReset),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, otps, 3)

			// Verify different correlation IDs
			assert.NotEqual(t, otps[0].OTPCode, otps[1].OTPCode)
			assert.Equal(t, otps[1].OTPCode, otps[2].OTPCode)
		})

		t.Run("PhoneNumberMasking", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create forgot password request
			forgotReq := &dto.ForgotPasswordRequest{
				Identifier: customer.Email,
			}

			// Perform forgot password
			result, err := loginFlow.ForgotPassword(context.Background(), forgotReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)

			// Verify phone number is masked
			assert.Contains(t, result.MaskedPhone, "*****")
			assert.NotEqual(t, customer.RepresentativeMobile, result.MaskedPhone)
			assert.True(t, len(result.MaskedPhone) < len(customer.RepresentativeMobile))
		})

		t.Run("OTPExpiryTime", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create forgot password request
			forgotReq := &dto.ForgotPasswordRequest{
				Identifier: customer.Email,
			}

			// Perform forgot password
			result, err := loginFlow.ForgotPassword(context.Background(), forgotReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)

			// Verify OTP expiry time is reasonable (5 minutes from now)
			expectedExpiry := time.Now().Add(5 * time.Minute)
			assert.True(t, result.OTPExpiry.After(time.Now()))
			assert.True(t, result.OTPExpiry.Before(expectedExpiry.Add(1*time.Minute)))
			assert.True(t, result.OTPExpiry.After(expectedExpiry.Add(-1*time.Minute)))
		})

		t.Run("SMSNotificationFailure", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create forgot password request
			forgotReq := &dto.ForgotPasswordRequest{
				Identifier: customer.Email,
			}

			// Perform forgot password (SMS might fail in test environment, but OTP should still be created)
			result, err := loginFlow.ForgotPassword(context.Background(), forgotReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)

			// Verify OTP was still created even if SMS failed
			otps, err := otpRepo.ByFilter(context.Background(), models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    utils.ToPtr(models.OTPTypePasswordReset),
				Status:     utils.ToPtr(models.OTPStatusPending),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, otps, 1)
		})

		return nil
	})
	require.NoError(t, err)
}

func TestResetPasswordFlow(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		fixtures := testingutil.NewTestFixtures(testDB)

		// Initialize repositories
		customerRepo := repository.NewCustomerRepository(testDB.DB)
		accountTypeRepo := repository.NewAccountTypeRepository(testDB.DB)
		otpRepo := repository.NewOTPVerificationRepository(testDB.DB)
		sessionRepo := repository.NewCustomerSessionRepository(testDB.DB)
		auditRepo := repository.NewAuditLogRepository(testDB.DB)

		// Initialize services
		tokenService, err := services.NewTokenService(1*time.Hour, 24*time.Hour, "test-issuer", "test-audience")
		require.NoError(t, err)

		notificationService := services.NewNotificationService(
			services.NewMockSMSProvider(),
			services.NewMockEmailProvider(),
		)

		// Initialize business flow
		loginFlow := businessflow.NewLoginFlow(
			customerRepo,
			sessionRepo,
			otpRepo,
			auditRepo,
			accountTypeRepo,
			tokenService,
			notificationService,
			testDB.DB,
		)

		t.Run("SuccessfulPasswordReset", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create a valid password reset OTP
			otp := &models.OTPVerification{
				CorrelationID: uuid.New(),
				CustomerID:    customer.ID,
				OTPCode:       "123456",
				OTPType:       models.OTPTypePasswordReset,
				TargetValue:   customer.RepresentativeMobile,
				Status:        models.OTPStatusPending,
				AttemptsCount: 0,
				MaxAttempts:   3,
				ExpiresAt:     time.Now().Add(5 * time.Minute),
				IPAddress:     utils.ToPtr("127.0.0.1"),
				UserAgent:     utils.ToPtr("Test User Agent"),
			}
			err = testDB.DB.Create(otp).Error
			require.NoError(t, err)

			// Create reset password request
			resetReq := &dto.ResetPasswordRequest{
				CustomerID:      customer.ID,
				OTPCode:         "123456",
				NewPassword:     "NewSecurePass123!",
				ConfirmPassword: "NewSecurePass123!",
			}

			// Perform password reset
			result, err := loginFlow.ResetPassword(context.Background(), resetReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)
			assert.NotNil(t, result.Customer)
			assert.NotNil(t, result.AccountType)
			assert.NotNil(t, result.Session)
			assert.Empty(t, result.ErrorCode)
			assert.Empty(t, result.ErrorMessage)

			// Verify customer data
			assert.Equal(t, customer.ID, result.Customer.ID)
			assert.Equal(t, customer.Email, result.Customer.Email)

			// Verify session was created
			assert.NotEmpty(t, result.Session.SessionToken)
			assert.NotEmpty(t, result.Session.RefreshToken)
			assert.True(t, utils.IsTrue(result.Session.IsActive))
			assert.True(t, result.Session.ExpiresAt.After(time.Now()))

			// Verify OTP was marked as used
			otps, err := otpRepo.ByFilter(context.Background(), models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    utils.ToPtr(models.OTPTypePasswordReset),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, otps, 2) // Original pending + new used

			// Check that one is used and one is pending
			statuses := make(map[string]int)
			for _, o := range otps {
				statuses[o.Status]++
			}
			assert.Equal(t, 1, statuses[models.OTPStatusUsed])
			assert.Equal(t, 1, statuses[models.OTPStatusPending])
		})

		t.Run("CustomerNotFound", func(t *testing.T) {
			// Create reset password request with non-existent customer
			resetReq := &dto.ResetPasswordRequest{
				CustomerID:      999999,
				OTPCode:         "123456",
				NewPassword:     "NewSecurePass123!",
				ConfirmPassword: "NewSecurePass123!",
			}

			// Perform password reset
			result, err := loginFlow.ResetPassword(context.Background(), resetReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.False(t, result.Success)
			assert.Nil(t, result.Customer)
			assert.Nil(t, result.AccountType)
			assert.Nil(t, result.Session)
			assert.Equal(t, dto.ErrorUserNotFound, result.ErrorCode)
			assert.Equal(t, "Customer not found", result.ErrorMessage)
		})

		t.Run("InvalidOTP", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create a valid password reset OTP
			otp := &models.OTPVerification{
				CorrelationID: uuid.New(),
				CustomerID:    customer.ID,
				OTPCode:       "123456",
				OTPType:       models.OTPTypePasswordReset,
				TargetValue:   customer.RepresentativeMobile,
				Status:        models.OTPStatusPending,
				AttemptsCount: 0,
				MaxAttempts:   3,
				ExpiresAt:     time.Now().Add(5 * time.Minute),
				IPAddress:     utils.ToPtr("127.0.0.1"),
				UserAgent:     utils.ToPtr("Test User Agent"),
			}
			err = testDB.DB.Create(otp).Error
			require.NoError(t, err)

			// Create reset password request with wrong OTP
			resetReq := &dto.ResetPasswordRequest{
				CustomerID:      customer.ID,
				OTPCode:         "000000", // Wrong OTP
				NewPassword:     "NewSecurePass123!",
				ConfirmPassword: "NewSecurePass123!",
			}

			// Perform password reset
			result, err := loginFlow.ResetPassword(context.Background(), resetReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.False(t, result.Success)
			assert.Nil(t, result.Customer)
			assert.Nil(t, result.AccountType)
			assert.Nil(t, result.Session)
			assert.Equal(t, dto.ErrorInvalidOTP, result.ErrorCode)
			assert.Equal(t, "Invalid OTP", result.ErrorMessage)
		})

		t.Run("ExpiredOTP", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create an expired password reset OTP
			otp := &models.OTPVerification{
				CorrelationID: uuid.New(),
				CustomerID:    customer.ID,
				OTPCode:       "123456",
				OTPType:       models.OTPTypePasswordReset,
				TargetValue:   customer.RepresentativeMobile,
				Status:        models.OTPStatusPending,
				AttemptsCount: 0,
				MaxAttempts:   3,
				ExpiresAt:     time.Now().Add(-1 * time.Hour), // Expired
				IPAddress:     utils.ToPtr("127.0.0.1"),
				UserAgent:     utils.ToPtr("Test User Agent"),
			}
			err = testDB.DB.Create(otp).Error
			require.NoError(t, err)

			// Create reset password request
			resetReq := &dto.ResetPasswordRequest{
				CustomerID:      customer.ID,
				OTPCode:         "123456",
				NewPassword:     "NewSecurePass123!",
				ConfirmPassword: "NewSecurePass123!",
			}

			// Perform password reset
			result, err := loginFlow.ResetPassword(context.Background(), resetReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.False(t, result.Success)
			assert.Nil(t, result.Customer)
			assert.Nil(t, result.AccountType)
			assert.Nil(t, result.Session)
			assert.Equal(t, dto.ErrorOTPExpired, result.ErrorCode)
			assert.Equal(t, "OTP has expired", result.ErrorMessage)
		})

		t.Run("PasswordUpdate", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Store original password hash
			originalPasswordHash := customer.PasswordHash

			// Create a valid password reset OTP
			otp := &models.OTPVerification{
				CorrelationID: uuid.New(),
				CustomerID:    customer.ID,
				OTPCode:       "123456",
				OTPType:       models.OTPTypePasswordReset,
				TargetValue:   customer.RepresentativeMobile,
				Status:        models.OTPStatusPending,
				AttemptsCount: 0,
				MaxAttempts:   3,
				ExpiresAt:     time.Now().Add(5 * time.Minute),
				IPAddress:     utils.ToPtr("127.0.0.1"),
				UserAgent:     utils.ToPtr("Test User Agent"),
			}
			err = testDB.DB.Create(otp).Error
			require.NoError(t, err)

			// Create reset password request
			resetReq := &dto.ResetPasswordRequest{
				CustomerID:      customer.ID,
				OTPCode:         "123456",
				NewPassword:     "NewSecurePass123!",
				ConfirmPassword: "NewSecurePass123!",
			}

			// Perform password reset
			result, err := loginFlow.ResetPassword(context.Background(), resetReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)

			// Verify password was updated
			updatedCustomer, err := customerRepo.ByID(context.Background(), customer.ID)
			require.NoError(t, err)
			assert.NotEqual(t, originalPasswordHash, updatedCustomer.PasswordHash)

			// Verify new password can be used for login
			loginReq := &dto.LoginRequest{
				Identifier: customer.Email,
				Password:   "NewSecurePass123!",
			}

			loginResult, err := loginFlow.Login(context.Background(), loginReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, loginResult)
			assert.True(t, loginResult.Success)
		})

		t.Run("SessionInvalidation", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create existing sessions for the customer
			session1, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			session2, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			// Create a valid password reset OTP
			otp := &models.OTPVerification{
				CorrelationID: uuid.New(),
				CustomerID:    customer.ID,
				OTPCode:       "123456",
				OTPType:       models.OTPTypePasswordReset,
				TargetValue:   customer.RepresentativeMobile,
				Status:        models.OTPStatusPending,
				AttemptsCount: 0,
				MaxAttempts:   3,
				ExpiresAt:     time.Now().Add(5 * time.Minute),
				IPAddress:     utils.ToPtr("127.0.0.1"),
				UserAgent:     utils.ToPtr("Test User Agent"),
			}
			err = testDB.DB.Create(otp).Error
			require.NoError(t, err)

			// Create reset password request
			resetReq := &dto.ResetPasswordRequest{
				CustomerID:      customer.ID,
				OTPCode:         "123456",
				NewPassword:     "NewSecurePass123!",
				ConfirmPassword: "NewSecurePass123!",
			}

			// Perform password reset
			result, err := loginFlow.ResetPassword(context.Background(), resetReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)

			// Verify old sessions were invalidated
			history1, err := sessionRepo.GetHistoryByCorrelationID(context.Background(), session1.CorrelationID)
			require.NoError(t, err)
			require.Len(t, history1, 2)
			assert.False(t, utils.IsTrue(history1[0].IsActive)) // Latest should be inactive

			history2, err := sessionRepo.GetHistoryByCorrelationID(context.Background(), session2.CorrelationID)
			require.NoError(t, err)
			require.Len(t, history2, 2)
			assert.False(t, utils.IsTrue(history2[0].IsActive)) // Latest should be inactive
		})

		t.Run("OTPUsedStatus", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create a valid password reset OTP
			otp := &models.OTPVerification{
				CorrelationID: uuid.New(),
				CustomerID:    customer.ID,
				OTPCode:       "123456",
				OTPType:       models.OTPTypePasswordReset,
				TargetValue:   customer.RepresentativeMobile,
				Status:        models.OTPStatusPending,
				AttemptsCount: 0,
				MaxAttempts:   3,
				ExpiresAt:     time.Now().Add(5 * time.Minute),
				IPAddress:     utils.ToPtr("127.0.0.1"),
				UserAgent:     utils.ToPtr("Test User Agent"),
			}
			err = testDB.DB.Create(otp).Error
			require.NoError(t, err)

			// Create reset password request
			resetReq := &dto.ResetPasswordRequest{
				CustomerID:      customer.ID,
				OTPCode:         "123456",
				NewPassword:     "NewSecurePass123!",
				ConfirmPassword: "NewSecurePass123!",
			}

			// Perform password reset
			result, err := loginFlow.ResetPassword(context.Background(), resetReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)

			// Verify OTP was marked as used with same correlation ID
			otps, err := otpRepo.ByFilter(context.Background(), models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    utils.ToPtr(models.OTPTypePasswordReset),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, otps, 2)

			// Find the used OTP
			var usedOTP *models.OTPVerification
			for _, o := range otps {
				if o.Status == models.OTPStatusUsed {
					usedOTP = o
					break
				}
			}
			require.NotNil(t, usedOTP)

			// Verify correlation ID is preserved
			assert.Equal(t, otp.CorrelationID, usedOTP.CorrelationID)
			assert.Equal(t, otp.OTPCode, usedOTP.OTPCode)
			assert.Equal(t, models.OTPStatusUsed, usedOTP.Status)
		})

		t.Run("AuditLogCreation", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create a valid password reset OTP
			otp := &models.OTPVerification{
				CorrelationID: uuid.New(),
				CustomerID:    customer.ID,
				OTPCode:       "123456",
				OTPType:       models.OTPTypePasswordReset,
				TargetValue:   customer.RepresentativeMobile,
				Status:        models.OTPStatusPending,
				AttemptsCount: 0,
				MaxAttempts:   3,
				ExpiresAt:     time.Now().Add(5 * time.Minute),
				IPAddress:     utils.ToPtr("127.0.0.1"),
				UserAgent:     utils.ToPtr("Test User Agent"),
			}
			err = testDB.DB.Create(otp).Error
			require.NoError(t, err)

			// Create reset password request
			resetReq := &dto.ResetPasswordRequest{
				CustomerID:      customer.ID,
				OTPCode:         "123456",
				NewPassword:     "NewSecurePass123!",
				ConfirmPassword: "NewSecurePass123!",
			}

			// Perform password reset
			result, err := loginFlow.ResetPassword(context.Background(), resetReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)

			// Check audit logs were created
			auditLogs, err := auditRepo.ByFilter(context.Background(), models.AuditLogFilter{
				CustomerID: &customer.ID,
				Action:     utils.ToPtr(models.AuditActionPasswordResetCompleted),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, auditLogs, 1)

			// Verify audit log
			auditLog := auditLogs[0]
			assert.Equal(t, customer.ID, *auditLog.CustomerID)
			assert.Equal(t, models.AuditActionPasswordResetCompleted, auditLog.Action)
			assert.True(t, utils.IsTrue(auditLog.Success))
			assert.NotNil(t, auditLog.IPAddress)
			assert.NotNil(t, auditLog.UserAgent)
		})

		t.Run("FailedResetAuditLog", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create reset password request with invalid OTP
			resetReq := &dto.ResetPasswordRequest{
				CustomerID:      customer.ID,
				OTPCode:         "000000", // Invalid OTP
				NewPassword:     "NewSecurePass123!",
				ConfirmPassword: "NewSecurePass123!",
			}

			// Perform password reset
			result, err := loginFlow.ResetPassword(context.Background(), resetReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.False(t, result.Success)

			// Check audit log was created for failed reset
			auditLogs, err := auditRepo.ByFilter(context.Background(), models.AuditLogFilter{
				CustomerID: &customer.ID,
				Action:     utils.ToPtr(models.AuditActionPasswordResetFailed),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, auditLogs, 1)

			// Verify audit log
			auditLog := auditLogs[0]
			assert.Equal(t, customer.ID, *auditLog.CustomerID)
			assert.Equal(t, models.AuditActionPasswordResetFailed, auditLog.Action)
			assert.False(t, utils.IsTrue(auditLog.Success))
			assert.NotNil(t, auditLog.IPAddress)
			assert.NotNil(t, auditLog.UserAgent)
		})

		t.Run("AccountTypeRetrieval", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create a valid password reset OTP
			otp := &models.OTPVerification{
				CorrelationID: uuid.New(),
				CustomerID:    customer.ID,
				OTPCode:       "123456",
				OTPType:       models.OTPTypePasswordReset,
				TargetValue:   customer.RepresentativeMobile,
				Status:        models.OTPStatusPending,
				AttemptsCount: 0,
				MaxAttempts:   3,
				ExpiresAt:     time.Now().Add(5 * time.Minute),
				IPAddress:     utils.ToPtr("127.0.0.1"),
				UserAgent:     utils.ToPtr("Test User Agent"),
			}
			err = testDB.DB.Create(otp).Error
			require.NoError(t, err)

			// Create reset password request
			resetReq := &dto.ResetPasswordRequest{
				CustomerID:      customer.ID,
				OTPCode:         "123456",
				NewPassword:     "NewSecurePass123!",
				ConfirmPassword: "NewSecurePass123!",
			}

			// Perform password reset
			result, err := loginFlow.ResetPassword(context.Background(), resetReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)

			// Verify account type was retrieved
			assert.NotNil(t, result.AccountType)
			assert.Equal(t, models.AccountTypeIndividual, result.AccountType.TypeName)
		})

		t.Run("NewSessionCreation", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create a valid password reset OTP
			otp := &models.OTPVerification{
				CorrelationID: uuid.New(),
				CustomerID:    customer.ID,
				OTPCode:       "123456",
				OTPType:       models.OTPTypePasswordReset,
				TargetValue:   customer.RepresentativeMobile,
				Status:        models.OTPStatusPending,
				AttemptsCount: 0,
				MaxAttempts:   3,
				ExpiresAt:     time.Now().Add(5 * time.Minute),
				IPAddress:     utils.ToPtr("127.0.0.1"),
				UserAgent:     utils.ToPtr("Test User Agent"),
			}
			err = testDB.DB.Create(otp).Error
			require.NoError(t, err)

			// Create reset password request
			resetReq := &dto.ResetPasswordRequest{
				CustomerID:      customer.ID,
				OTPCode:         "123456",
				NewPassword:     "NewSecurePass123!",
				ConfirmPassword: "NewSecurePass123!",
			}

			// Perform password reset
			result, err := loginFlow.ResetPassword(context.Background(), resetReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)

			// Verify new session was created
			assert.NotNil(t, result.Session)
			assert.NotEmpty(t, result.Session.SessionToken)
			assert.NotEmpty(t, result.Session.RefreshToken)
			assert.True(t, utils.IsTrue(result.Session.IsActive))
			assert.True(t, result.Session.ExpiresAt.After(time.Now()))
			assert.NotEqual(t, uuid.Nil, result.Session.CorrelationID)
		})

		t.Run("MultipleOTPsSameCustomer", func(t *testing.T) {
			// Create test customer
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create multiple OTPs for the same customer
			otp1 := &models.OTPVerification{
				CorrelationID: uuid.New(),
				CustomerID:    customer.ID,
				OTPCode:       "123456",
				OTPType:       models.OTPTypePasswordReset,
				TargetValue:   customer.RepresentativeMobile,
				Status:        models.OTPStatusPending,
				AttemptsCount: 0,
				MaxAttempts:   3,
				ExpiresAt:     time.Now().Add(5 * time.Minute),
				IPAddress:     utils.ToPtr("127.0.0.1"),
				UserAgent:     utils.ToPtr("Test User Agent"),
			}
			err = testDB.DB.Create(otp1).Error
			require.NoError(t, err)

			otp2 := &models.OTPVerification{
				CorrelationID: uuid.New(),
				CustomerID:    customer.ID,
				OTPCode:       "654321",
				OTPType:       models.OTPTypePasswordReset,
				TargetValue:   customer.RepresentativeMobile,
				Status:        models.OTPStatusPending,
				AttemptsCount: 0,
				MaxAttempts:   3,
				ExpiresAt:     time.Now().Add(5 * time.Minute),
				IPAddress:     utils.ToPtr("127.0.0.1"),
				UserAgent:     utils.ToPtr("Test User Agent"),
			}
			err = testDB.DB.Create(otp2).Error
			require.NoError(t, err)

			// Create reset password request with first OTP
			resetReq := &dto.ResetPasswordRequest{
				CustomerID:      customer.ID,
				OTPCode:         "123456",
				NewPassword:     "NewSecurePass123!",
				ConfirmPassword: "NewSecurePass123!",
			}

			// Perform password reset
			result, err := loginFlow.ResetPassword(context.Background(), resetReq, "127.0.0.1", "Test User Agent")
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)

			// Verify only the used OTP was marked as used, others remain pending
			otps, err := otpRepo.ByFilter(context.Background(), models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    utils.ToPtr(models.OTPTypePasswordReset),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, otps, 3) // 2 original + 1 used

			// Check statuses
			statuses := make(map[string]int)
			for _, o := range otps {
				statuses[o.Status]++
			}
			assert.Equal(t, 1, statuses[models.OTPStatusUsed])
			assert.Equal(t, 2, statuses[models.OTPStatusPending])
		})

		return nil
	})
	require.NoError(t, err)
}
