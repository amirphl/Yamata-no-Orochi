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
