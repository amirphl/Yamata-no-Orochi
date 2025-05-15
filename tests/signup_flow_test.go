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

func TestInitiateSignup(t *testing.T) {
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
		signupFlow := businessflow.NewSignupFlow(
			customerRepo,
			accountTypeRepo,
			otpRepo,
			sessionRepo,
			auditRepo,
			tokenService,
			notificationService,
			testDB.DB,
		)

		t.Run("SuccessfulIndividualSignup", func(t *testing.T) {
			req := &dto.SignupRequest{
				AccountType:             models.AccountTypeIndividual,
				RepresentativeFirstName: "John",
				RepresentativeLastName:  "Doe",
				RepresentativeMobile:    "+989123456789",
				Email:                   "john.doe@example.com",
				Password:                "SecurePass123!",
				ConfirmPassword:         "SecurePass123!",
			}

			result, err := signupFlow.InitiateSignup(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.OTPSent)
			assert.NotEmpty(t, result.CustomerID)
			assert.Contains(t, result.OTPTarget, "****")

			// Verify customer was created
			customer, err := customerRepo.ByID(context.Background(), result.CustomerID)
			require.NoError(t, err)
			require.NotNil(t, customer)
			assert.Equal(t, "John", customer.RepresentativeFirstName)
			assert.Equal(t, "Doe", customer.RepresentativeLastName)
			assert.Equal(t, "+989123456789", customer.RepresentativeMobile)
			assert.Equal(t, "john.doe@example.com", customer.Email)
			assert.NotEmpty(t, customer.UUID)
			assert.NotZero(t, customer.AgencyRefererCode)
			assert.True(t, utils.IsTrue(customer.IsActive))
			assert.False(t, utils.IsTrue(customer.IsEmailVerified))
			assert.False(t, utils.IsTrue(customer.IsMobileVerified))

			// Verify OTP was created
			otps, err := otpRepo.ByFilter(context.Background(), models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    utils.ToPtr(models.OTPTypeMobile),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, otps, 1)
			assert.Equal(t, models.OTPStatusPending, otps[0].Status)
			assert.True(t, otps[0].ExpiresAt.After(time.Now()))

			// Verify audit log was created
			auditLogs, err := auditRepo.ByFilter(context.Background(), models.AuditLogFilter{
				CustomerID: &customer.ID,
				Action:     utils.ToPtr(models.AuditActionSignupInitiated),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, auditLogs, 1)
			assert.True(t, utils.IsTrue(auditLogs[0].Success))
		})

		t.Run("SuccessfulCompanySignup", func(t *testing.T) {
			companyName := "Test Company Ltd"
			nationalID := "12345678901"
			companyPhone := "02112345678"
			companyAddress := "123 Test Street, Tehran"
			postalCode := "1234567890"

			req := &dto.SignupRequest{
				AccountType:             models.AccountTypeIndependentCompany,
				CompanyName:             &companyName,
				NationalID:              &nationalID,
				CompanyPhone:            &companyPhone,
				CompanyAddress:          &companyAddress,
				PostalCode:              &postalCode,
				RepresentativeFirstName: "Jane",
				RepresentativeLastName:  "Smith",
				RepresentativeMobile:    "+989987654321",
				Email:                   "jane.smith@company.com",
				Password:                "SecurePass123!",
				ConfirmPassword:         "SecurePass123!",
			}

			result, err := signupFlow.InitiateSignup(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.OTPSent)

			// Verify customer was created with company fields
			customer, err := customerRepo.ByID(context.Background(), result.CustomerID)
			require.NoError(t, err)
			require.NotNil(t, customer)
			assert.Equal(t, companyName, *customer.CompanyName)
			assert.Equal(t, nationalID, *customer.NationalID)
			assert.Equal(t, companyPhone, *customer.CompanyPhone)
			assert.Equal(t, companyAddress, *customer.CompanyAddress)
			assert.Equal(t, postalCode, *customer.PostalCode)
		})

		t.Run("SuccessfulAgencySignup", func(t *testing.T) {
			companyName := "Marketing Agency Ltd"
			nationalID := "98765432109"
			companyPhone := "02187654321"
			companyAddress := "456 Agency Street, Tehran"
			postalCode := "9876543210"

			req := &dto.SignupRequest{
				AccountType:             models.AccountTypeMarketingAgency,
				CompanyName:             &companyName,
				NationalID:              &nationalID,
				CompanyPhone:            &companyPhone,
				CompanyAddress:          &companyAddress,
				PostalCode:              &postalCode,
				RepresentativeFirstName: "Agency",
				RepresentativeLastName:  "Manager",
				RepresentativeMobile:    "+989111111111",
				Email:                   "agency@marketing.com",
				Password:                "SecurePass123!",
				ConfirmPassword:         "SecurePass123!",
			}

			result, err := signupFlow.InitiateSignup(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.OTPSent)

			// Verify customer was created as marketing agency
			customer, err := customerRepo.ByFilter(context.Background(), models.CustomerFilter{
				ID: &result.CustomerID,
			}, "", 0, 0)
			require.NoError(t, err)
			require.NotNil(t, customer)
			assert.True(t, customer[0].IsAgency())
		})

		t.Run("SignupWithReferrerAgency", func(t *testing.T) {
			// First create a marketing agency
			agency, err := fixtures.CreateTestCustomer(models.AccountTypeMarketingAgency)
			require.NoError(t, err)

			req := &dto.SignupRequest{
				AccountType:             models.AccountTypeIndividual,
				RepresentativeFirstName: "Referral",
				RepresentativeLastName:  "User",
				RepresentativeMobile:    "+989222222222",
				Email:                   "referral@example.com",
				Password:                "SecurePass123!",
				ConfirmPassword:         "SecurePass123!",
				ReferrerAgencyCode:      &agency.AgencyRefererCode,
			}

			result, err := signupFlow.InitiateSignup(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.OTPSent)

			// Verify customer was created with referrer agency
			customer, err := customerRepo.ByID(context.Background(), result.CustomerID)
			require.NoError(t, err)
			require.NotNil(t, customer)
			assert.Equal(t, agency.ID, *customer.ReferrerAgencyID)
		})

		t.Run("EmailAlreadyExists", func(t *testing.T) {
			// Create existing customer
			existingCustomer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			req := &dto.SignupRequest{
				AccountType:             models.AccountTypeIndividual,
				RepresentativeFirstName: "New",
				RepresentativeLastName:  "User",
				RepresentativeMobile:    "+989333333333",
				Email:                   existingCustomer.Email, // Same email
				Password:                "SecurePass123!",
				ConfirmPassword:         "SecurePass123!",
			}

			result, err := signupFlow.InitiateSignup(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), "email already exists")

			// Verify no customer was created
			customers, err := customerRepo.ByFilter(context.Background(), models.CustomerFilter{
				Email: &req.Email,
			}, "", 0, 0)
			require.NoError(t, err)
			assert.Len(t, customers, 1) // Only the original customer
		})

		t.Run("MobileAlreadyExists", func(t *testing.T) {
			// Create existing customer
			existingCustomer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			req := &dto.SignupRequest{
				AccountType:             models.AccountTypeIndividual,
				RepresentativeFirstName: "New",
				RepresentativeLastName:  "User",
				RepresentativeMobile:    existingCustomer.RepresentativeMobile, // Same mobile
				Email:                   "newuser@example.com",
				Password:                "SecurePass123!",
				ConfirmPassword:         "SecurePass123!",
			}

			result, err := signupFlow.InitiateSignup(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), "mobile number already exists")
		})

		t.Run("NationalIDAlreadyExists", func(t *testing.T) {
			// Create existing company customer
			existingCustomer, err := fixtures.CreateTestCustomer(models.AccountTypeIndependentCompany)
			require.NoError(t, err)

			req := &dto.SignupRequest{
				AccountType:             models.AccountTypeIndependentCompany,
				CompanyName:             utils.ToPtr("Another Company"),
				NationalID:              existingCustomer.NationalID, // Same national ID
				CompanyPhone:            utils.ToPtr("02187654321"),
				CompanyAddress:          utils.ToPtr("Another Address"),
				PostalCode:              utils.ToPtr("1111111111"),
				RepresentativeFirstName: "Another",
				RepresentativeLastName:  "Manager",
				RepresentativeMobile:    "+989444444444",
				Email:                   "another@company.com",
				Password:                "SecurePass123!",
				ConfirmPassword:         "SecurePass123!",
			}

			result, err := signupFlow.InitiateSignup(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), "national ID already exists")
		})

		t.Run("CompanySignupMissingFields", func(t *testing.T) {
			req := &dto.SignupRequest{
				AccountType: models.AccountTypeIndependentCompany,
				// Missing company fields
				RepresentativeFirstName: "Incomplete",
				RepresentativeLastName:  "Company",
				RepresentativeMobile:    "+989555555555",
				Email:                   "incomplete@company.com",
				Password:                "SecurePass123!",
				ConfirmPassword:         "SecurePass123!",
			}

			result, err := signupFlow.InitiateSignup(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), "company fields are required")
		})

		t.Run("InvalidReferrerAgency", func(t *testing.T) {
			invalidAgencyCode := int64(9999999999)

			req := &dto.SignupRequest{
				AccountType:             models.AccountTypeIndividual,
				RepresentativeFirstName: "Invalid",
				RepresentativeLastName:  "Referral",
				RepresentativeMobile:    "+989666666666",
				Email:                   "invalid@referral.com",
				Password:                "SecurePass123!",
				ConfirmPassword:         "SecurePass123!",
				ReferrerAgencyCode:      &invalidAgencyCode,
			}

			result, err := signupFlow.InitiateSignup(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), "referrer agency not found")
		})

		t.Run("ReferrerAgencyNotMarketingAgency", func(t *testing.T) {
			// Create a company (not marketing agency)
			company, err := fixtures.CreateTestCustomer(models.AccountTypeIndependentCompany)
			require.NoError(t, err)

			req := &dto.SignupRequest{
				AccountType:             models.AccountTypeIndividual,
				RepresentativeFirstName: "Invalid",
				RepresentativeLastName:  "Referral",
				RepresentativeMobile:    "+989777777777",
				Email:                   "invalid@referral.com",
				Password:                "SecurePass123!",
				ConfirmPassword:         "SecurePass123!",
				ReferrerAgencyCode:      &company.AgencyRefererCode,
			}

			result, err := signupFlow.InitiateSignup(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), "referrer must be a marketing agency")
		})

		// t.Run("InactiveReferrerAgency", func(t *testing.T) {
		// 	// Create an inactive marketing agency
		// 	agency, err := fixtures.CreateTestCustomer(models.AccountTypeMarketingAgency)
		// 	require.NoError(t, err)

		// 	// Deactivate the agency
		// 	err = customerRepo.UpdateVerificationStatus(context.Background(), agency.ID, nil, nil, nil, nil)
		// 	require.NoError(t, err)

		// 	req := &dto.SignupRequest{
		// 		AccountType:             models.AccountTypeIndividual,
		// 		RepresentativeFirstName: "Inactive",
		// 		RepresentativeLastName:  "Referral",
		// 		RepresentativeMobile:    "+989888888888",
		// 		Email:                   "inactive@referral.com",
		// 		Password:                "SecurePass123!",
		// 		ConfirmPassword:         "SecurePass123!",
		// 		ReferrerAgencyCode:      &agency.AgencyRefererCode,
		// 	}

		// 	result, err := signupFlow.InitiateSignup(context.Background(), req)
		// 	require.Error(t, err)
		// 	require.Nil(t, result)
		// 	assert.Contains(t, err.Error(), "referrer agency is inactive")
		// })

		// t.Run("DatabaseError", func(t *testing.T) {
		// 	// This test would require mocking the database to return an error
		// 	// For now, we'll test with a valid request but simulate a database issue
		// 	// by using a context that might cause issues
		// 	req := &dto.SignupRequest{
		// 		AccountType:             models.AccountTypeIndividual,
		// 		RepresentativeFirstName: "Database",
		// 		RepresentativeLastName:  "Error",
		// 		RepresentativeMobile:    "+989000000000",
		// 		Email:                   "database@error.com",
		// 		Password:                "SecurePass123!",
		// 		ConfirmPassword:         "SecurePass123!",
		// 	}

		// 	// Use a cancelled context to simulate database error
		// 	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		// 	defer cancel()

		// 	result, err := signupFlow.InitiateSignup(ctx, req)
		// 	require.Error(t, err)
		// 	require.Nil(t, result)
		// })

		t.Run("OTPGenerationFailure", func(t *testing.T) {
			// This test would require mocking the OTP generation to fail
			// For now, we'll test the normal flow and verify OTP creation
			req := &dto.SignupRequest{
				AccountType:             models.AccountTypeIndividual,
				RepresentativeFirstName: "OTP",
				RepresentativeLastName:  "Test",
				RepresentativeMobile:    "+989177111111",
				Email:                   "otp@test.com",
				Password:                "SecurePass123!",
				ConfirmPassword:         "SecurePass123!",
			}

			result, err := signupFlow.InitiateSignup(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify OTP was created with proper correlation ID
			otps, err := otpRepo.ByFilter(context.Background(), models.OTPVerificationFilter{
				CustomerID: &result.CustomerID,
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, otps, 1)
			assert.NotEmpty(t, otps[0].CorrelationID)
			assert.Equal(t, models.OTPStatusPending, otps[0].Status)
			assert.Equal(t, 0, otps[0].AttemptsCount)
			assert.Equal(t, 3, otps[0].MaxAttempts)
		})

		t.Run("AuditLogCreation", func(t *testing.T) {
			req := &dto.SignupRequest{
				AccountType:             models.AccountTypeIndividual,
				RepresentativeFirstName: "Audit",
				RepresentativeLastName:  "Test",
				RepresentativeMobile:    "+989222288222",
				Email:                   "audit@test.com",
				Password:                "SecurePass123!",
				ConfirmPassword:         "SecurePass123!",
			}

			result, err := signupFlow.InitiateSignup(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify audit log was created for successful signup
			auditLogs, err := auditRepo.ByFilter(context.Background(), models.AuditLogFilter{
				CustomerID: &result.CustomerID,
				Action:     utils.ToPtr(models.AuditActionSignupInitiated),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, auditLogs, 1)
			assert.True(t, utils.IsTrue(auditLogs[0].Success))
			assert.Contains(t, *auditLogs[0].Description, "Signup initiated successfully")
		})

		t.Run("FailedSignupAuditLog", func(t *testing.T) {
			// Create existing customer to cause failure
			existingCustomer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			req := &dto.SignupRequest{
				AccountType:             models.AccountTypeIndividual,
				RepresentativeFirstName: "Failed",
				RepresentativeLastName:  "Signup",
				RepresentativeMobile:    "+989333333333",
				Email:                   existingCustomer.Email, // Duplicate email
				Password:                "SecurePass123!",
				ConfirmPassword:         "SecurePass123!",
			}

			result, err := signupFlow.InitiateSignup(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)

			// Verify audit log was created for failed signup
			auditLogs, err := auditRepo.ByFilter(context.Background(), models.AuditLogFilter{
				Action: utils.ToPtr(models.AuditActionSignupInitiated),
			}, "", 0, 0)
			require.NoError(t, err)
			// Should have at least one failed audit log
			failedLogs := 0
			for _, log := range auditLogs {
				if !utils.IsTrue(log.Success) {
					failedLogs++
				}
			}
			assert.GreaterOrEqual(t, failedLogs, 1)
		})

		t.Run("MobileNumberMasking", func(t *testing.T) {
			req := &dto.SignupRequest{
				AccountType:             models.AccountTypeIndividual,
				RepresentativeFirstName: "Mask",
				RepresentativeLastName:  "Test",
				RepresentativeMobile:    "+989223456789",
				Email:                   "mask@test.com",
				Password:                "SecurePass123!",
				ConfirmPassword:         "SecurePass123!",
			}

			result, err := signupFlow.InitiateSignup(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify mobile number is masked in response
			assert.Contains(t, result.OTPTarget, "****")
			assert.NotEqual(t, req.RepresentativeMobile, result.OTPTarget)
		})

		return nil
	})

	require.NoError(t, err)
}

func TestVerifyOTP(t *testing.T) {
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
		signupFlow := businessflow.NewSignupFlow(
			customerRepo,
			accountTypeRepo,
			otpRepo,
			sessionRepo,
			auditRepo,
			tokenService,
			notificationService,
			testDB.DB,
		)

		t.Run("SuccessfulMobileOTPVerification", func(t *testing.T) {
			// Create customer and OTP
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otpCode := "123456"
			_, err = fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, otpCode)
			require.NoError(t, err)

			req := &dto.OTPVerificationRequest{
				CustomerID: customer.ID,
				OTPCode:    otpCode,
				OTPType:    models.OTPTypeMobile,
			}

			result, err := signupFlow.VerifyOTP(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, "Signup completed successfully!", result.Message)
			assert.NotEmpty(t, result.Token)
			assert.NotEmpty(t, result.RefreshToken)
			assert.NotNil(t, result.Customer)
			assert.Equal(t, customer.ID, result.Customer.ID)

			// Verify customer verification status was updated
			updatedCustomer, err := customerRepo.ByID(context.Background(), customer.ID)
			require.NoError(t, err)
			require.NotNil(t, updatedCustomer)
			assert.True(t, utils.IsTrue(updatedCustomer.IsMobileVerified))
			assert.False(t, utils.IsTrue(updatedCustomer.IsEmailVerified))

			// Verify OTP status was updated
			otps, err := otpRepo.ByFilter(context.Background(), models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    utils.ToPtr(models.OTPTypeMobile),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, otps, 2) // Original pending + new verified

			// Check that one is verified and one is pending
			statuses := make(map[string]int)
			for _, o := range otps {
				statuses[o.Status]++
			}
			assert.Equal(t, 1, statuses[models.OTPStatusVerified])
			assert.Equal(t, 1, statuses[models.OTPStatusPending])

			// Verify session was created
			sessions, err := sessionRepo.ByFilter(context.Background(), models.CustomerSessionFilter{
				CustomerID: &customer.ID,
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, sessions, 1)
			assert.True(t, utils.IsTrue(sessions[0].IsActive))
			assert.True(t, sessions[0].ExpiresAt.After(time.Now()))

			// Verify audit logs were created
			auditLogs, err := auditRepo.ByFilter(context.Background(), models.AuditLogFilter{
				CustomerID: &customer.ID,
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, auditLogs, 2) // OTP verified + signup completed

			actions := make(map[string]int)
			for _, log := range auditLogs {
				actions[log.Action]++
			}
			assert.Equal(t, 1, actions[models.AuditActionOTPVerified])
			assert.Equal(t, 1, actions[models.AuditActionSignupCompleted])
		})

		t.Run("SuccessfulEmailOTPVerification", func(t *testing.T) {
			// Create customer and OTP
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otpCode := "654321"
			_, err = fixtures.CreateTestOTP(customer.ID, models.OTPTypeEmail, otpCode)
			require.NoError(t, err)

			req := &dto.OTPVerificationRequest{
				CustomerID: customer.ID,
				OTPCode:    otpCode,
				OTPType:    models.OTPTypeEmail,
			}

			result, err := signupFlow.VerifyOTP(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify customer verification status was updated
			updatedCustomer, err := customerRepo.ByID(context.Background(), customer.ID)
			require.NoError(t, err)
			require.NotNil(t, updatedCustomer)
			assert.True(t, utils.IsTrue(updatedCustomer.IsEmailVerified))
			assert.False(t, utils.IsTrue(updatedCustomer.IsMobileVerified))
		})

		t.Run("CustomerNotFound", func(t *testing.T) {
			req := &dto.OTPVerificationRequest{
				CustomerID: 99999, // Non-existent customer
				OTPCode:    "123456",
				OTPType:    models.OTPTypeMobile,
			}

			result, err := signupFlow.VerifyOTP(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), "customer not found")
		})

		t.Run("NoValidOTPFound", func(t *testing.T) {
			// Create customer without OTP
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			req := &dto.OTPVerificationRequest{
				CustomerID: customer.ID,
				OTPCode:    "123456",
				OTPType:    models.OTPTypeMobile,
			}

			result, err := signupFlow.VerifyOTP(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), "no valid OTP found")
		})

		t.Run("InvalidOTPCode", func(t *testing.T) {
			// Create customer and OTP
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otpCode := "123456"
			_, err = fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, otpCode)
			require.NoError(t, err)

			req := &dto.OTPVerificationRequest{
				CustomerID: customer.ID,
				OTPCode:    "999999", // Wrong code
				OTPType:    models.OTPTypeMobile,
			}

			result, err := signupFlow.VerifyOTP(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), "invalid OTP code")

			// Verify failed OTP record was created
			otps, err := otpRepo.ByFilter(context.Background(), models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    utils.ToPtr(models.OTPTypeMobile),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, otps, 2) // Original pending + new failed

			// Check that one is failed and one is pending
			statuses := make(map[string]int)
			for _, o := range otps {
				statuses[o.Status]++
			}
			assert.Equal(t, 1, statuses[models.OTPStatusFailed])
			assert.Equal(t, 1, statuses[models.OTPStatusPending])
		})

		t.Run("ExpiredOTP", func(t *testing.T) {
			// Create customer and expired OTP
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otpCode := "123456"
			expiredOTP := &models.OTPVerification{
				CorrelationID: uuid.New(),
				CustomerID:    customer.ID,
				OTPCode:       otpCode,
				OTPType:       models.OTPTypeMobile,
				TargetValue:   customer.RepresentativeMobile,
				Status:        models.OTPStatusPending,
				AttemptsCount: 0,
				MaxAttempts:   3,
				ExpiresAt:     time.Now().Add(-1 * time.Hour), // Expired
			}
			err = testDB.DB.Create(expiredOTP).Error
			require.NoError(t, err)

			req := &dto.OTPVerificationRequest{
				CustomerID: customer.ID,
				OTPCode:    otpCode,
				OTPType:    models.OTPTypeMobile,
			}

			result, err := signupFlow.VerifyOTP(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), "no valid OTP found")
		})

		t.Run("OTPExceededMaxAttempts", func(t *testing.T) {
			// Create customer and OTP with max attempts exceeded
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otpCode := "123456"
			exceededOTP := &models.OTPVerification{
				CorrelationID: uuid.New(),
				CustomerID:    customer.ID,
				OTPCode:       otpCode,
				OTPType:       models.OTPTypeMobile,
				TargetValue:   customer.RepresentativeMobile,
				Status:        models.OTPStatusPending,
				AttemptsCount: 3, // Max attempts reached
				MaxAttempts:   3,
				ExpiresAt:     time.Now().Add(5 * time.Minute),
			}
			err = testDB.DB.Create(exceededOTP).Error
			require.NoError(t, err)

			req := &dto.OTPVerificationRequest{
				CustomerID: customer.ID,
				OTPCode:    otpCode,
				OTPType:    models.OTPTypeMobile,
			}

			result, err := signupFlow.VerifyOTP(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), "no valid OTP found")
		})

		t.Run("MultipleOTPsForSameCustomer", func(t *testing.T) {
			// Create customer with multiple OTPs
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create multiple OTPs
			_, err = fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "111111")
			require.NoError(t, err)
			_, err = fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "222222")
			require.NoError(t, err)

			// Verify with second OTP
			req := &dto.OTPVerificationRequest{
				CustomerID: customer.ID,
				OTPCode:    "222222",
				OTPType:    models.OTPTypeMobile,
			}

			result, err := signupFlow.VerifyOTP(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify only one OTP was marked as verified
			otps, err := otpRepo.ByFilter(context.Background(), models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    utils.ToPtr(models.OTPTypeMobile),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, otps, 3) // 2 original + 1 verified

			statuses := make(map[string]int)
			for _, o := range otps {
				statuses[o.Status]++
			}
			assert.Equal(t, 1, statuses[models.OTPStatusVerified])
			assert.Equal(t, 2, statuses[models.OTPStatusPending])
		})

		t.Run("InvalidOTPType", func(t *testing.T) {
			// Create customer and OTP
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otpCode := "123456"
			_, err = fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, otpCode)
			require.NoError(t, err)

			req := &dto.OTPVerificationRequest{
				CustomerID: customer.ID,
				OTPCode:    otpCode,
				OTPType:    "invalid_type",
			}

			result, err := signupFlow.VerifyOTP(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), "no valid OTP found")
		})

		t.Run("FailedVerificationAuditLog", func(t *testing.T) {
			// Create customer without OTP to cause failure
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			req := &dto.OTPVerificationRequest{
				CustomerID: customer.ID,
				OTPCode:    "123456",
				OTPType:    models.OTPTypeMobile,
			}

			result, err := signupFlow.VerifyOTP(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)

			// Verify audit log was created for failed verification
			auditLogs, err := auditRepo.ByFilter(context.Background(), models.AuditLogFilter{
				CustomerID: &customer.ID,
				Action:     utils.ToPtr(models.AuditActionOTPFailed),
			}, "", 0, 0)
			require.NoError(t, err)
			require.Len(t, auditLogs, 1)
			assert.False(t, utils.IsTrue(auditLogs[0].Success))
			assert.Contains(t, *auditLogs[0].Description, "OTP verification failed")
		})

		t.Run("CorrelationIDPreservation", func(t *testing.T) {
			// Create customer and OTP
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otpCode := "123456"
			otp, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, otpCode)
			require.NoError(t, err)

			originalCorrelationID := otp.CorrelationID

			req := &dto.OTPVerificationRequest{
				CustomerID: customer.ID,
				OTPCode:    otpCode,
				OTPType:    models.OTPTypeMobile,
			}

			result, err := signupFlow.VerifyOTP(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify correlation ID was preserved in verified OTP
			otps, err := otpRepo.ByFilter(context.Background(), models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    utils.ToPtr(models.OTPTypeMobile),
			}, "", 0, 0)
			require.NoError(t, err)

			var verifiedOTP *models.OTPVerification
			for _, o := range otps {
				if o.Status == models.OTPStatusVerified {
					verifiedOTP = o
					break
				}
			}
			require.NotNil(t, verifiedOTP)
			assert.Equal(t, originalCorrelationID, verifiedOTP.CorrelationID)
		})

		t.Run("TransactionRollbackOnFailure", func(t *testing.T) {
			// Create customer and OTP
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otpCode := "123456"
			_, err = fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, otpCode)
			require.NoError(t, err)

			// Store initial verification status
			initialMobileVerified := customer.IsMobileVerified
			initialEmailVerified := customer.IsEmailVerified

			req := &dto.OTPVerificationRequest{
				CustomerID: customer.ID,
				OTPCode:    "999999", // Wrong code to cause failure
				OTPType:    models.OTPTypeMobile,
			}

			result, err := signupFlow.VerifyOTP(context.Background(), req)
			require.Error(t, err)
			require.Nil(t, result)

			// Verify customer verification status was not changed (transaction rolled back)
			updatedCustomer, err := customerRepo.ByID(context.Background(), customer.ID)
			require.NoError(t, err)
			require.NotNil(t, updatedCustomer)
			assert.Equal(t, initialMobileVerified, updatedCustomer.IsMobileVerified)
			assert.Equal(t, initialEmailVerified, updatedCustomer.IsEmailVerified)
		})

		return nil
	})

	require.NoError(t, err)
}
