// Package tests contains test cases for models and repository packages to avoid circular imports
package tests

import (
	"context"
	"testing"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	testingutil "github.com/amirphl/Yamata-no-Orochi/testing"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountTypeRepository(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		repo := repository.NewAccountTypeRepository(testDB.DB)
		ctx := testingutil.CreateTestContext()

		t.Run("ByID", func(t *testing.T) {
			// Get existing account type (inserted by setup)
			accountType, err := repo.ByID(ctx, 1)
			require.NoError(t, err)
			assert.NotNil(t, accountType)
			assert.Equal(t, uint(1), accountType.ID)
			assert.Equal(t, models.AccountTypeIndividual, accountType.TypeName)
		})

		t.Run("ByIDNotFound", func(t *testing.T) {
			accountType, err := repo.ByID(ctx, 999)
			assert.NoError(t, err)
			assert.Nil(t, accountType)
		})

		t.Run("ByTypeName", func(t *testing.T) {
			accountType, err := repo.ByTypeName(ctx, models.AccountTypeIndividual)
			require.NoError(t, err)
			assert.NotNil(t, accountType)
			assert.Equal(t, models.AccountTypeIndividual, accountType.TypeName)
			assert.Equal(t, "Individual", accountType.DisplayName)
		})

		t.Run("ByFilter", func(t *testing.T) {
			// Test with empty filter (should return all)
			accountTypes, err := repo.ByFilter(ctx, models.AccountType{}, "", 0, 0)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(accountTypes), 3) // At least the 3 default types

			// Test with specific filter
			accountTypes, err = repo.ByFilter(ctx, models.AccountType{TypeName: models.AccountTypeIndividual}, "", 0, 0)
			require.NoError(t, err)
			assert.Len(t, accountTypes, 1)
			assert.Equal(t, models.AccountTypeIndividual, accountTypes[0].TypeName)
		})

		t.Run("Count", func(t *testing.T) {
			count, err := repo.Count(ctx, models.AccountType{})
			require.NoError(t, err)
			assert.GreaterOrEqual(t, count, int64(3))
		})

		t.Run("Exists", func(t *testing.T) {
			exists, err := repo.Exists(ctx, models.AccountType{TypeName: models.AccountTypeIndividual})
			require.NoError(t, err)
			assert.True(t, exists)
		})

		return nil
	})
	require.NoError(t, err)
}

func TestCustomerRepository(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		repo := repository.NewCustomerRepository(testDB.DB)
		fixtures := testingutil.NewTestFixtures(testDB)
		ctx := testingutil.CreateTestContext()

		t.Run("Save", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)
			assert.NotZero(t, customer.ID)
		})

		t.Run("ByID", func(t *testing.T) {
			originalCustomer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			customer, err := repo.ByID(ctx, originalCustomer.ID)
			require.NoError(t, err)
			assert.NotNil(t, customer)
			assert.Equal(t, originalCustomer.ID, customer.ID)
			assert.Equal(t, originalCustomer.Email, customer.Email)
		})

		t.Run("ByEmail", func(t *testing.T) {
			originalCustomer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			customer, err := repo.ByEmail(ctx, originalCustomer.Email)
			require.NoError(t, err)
			assert.NotNil(t, customer)
			assert.Equal(t, originalCustomer.ID, customer.ID)
			assert.Equal(t, originalCustomer.Email, customer.Email)
		})

		t.Run("ByEmailNotFound", func(t *testing.T) {
			customer, err := repo.ByEmail(ctx, "nonexistent@example.com")
			assert.NoError(t, err)
			assert.Nil(t, customer)
		})

		t.Run("ByMobile", func(t *testing.T) {
			originalCustomer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			customer, err := repo.ByMobile(ctx, originalCustomer.RepresentativeMobile)
			require.NoError(t, err)
			assert.NotNil(t, customer)
			assert.Equal(t, originalCustomer.ID, customer.ID)
			assert.Equal(t, originalCustomer.RepresentativeMobile, customer.RepresentativeMobile)
		})

		t.Run("ByMobileNotFound", func(t *testing.T) {
			customer, err := repo.ByMobile(ctx, "+989999999999")
			assert.NoError(t, err)
			assert.Nil(t, customer)
		})

		t.Run("ByNationalID", func(t *testing.T) {
			originalCustomer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			customer, err := repo.ByNationalID(ctx, *originalCustomer.NationalID)

			require.NoError(t, err)
			assert.NotNil(t, customer)
			assert.Equal(t, originalCustomer.ID, customer.ID)
			assert.Equal(t, *originalCustomer.NationalID, *customer.NationalID)
		})

		t.Run("ByFilter", func(t *testing.T) {
			// Create multiple test customers
			customers, err := fixtures.CreateMultipleTestCustomers()
			require.NoError(t, err)
			require.Len(t, customers, 3)

			// Test filter by email
			email := customers[0].Email
			filter := models.CustomerFilter{Email: &email}
			result, err := repo.ByFilter(ctx, filter, "", 0, 0)
			require.NoError(t, err)
			assert.Len(t, result, 1)
			assert.Equal(t, customers[0].Email, result[0].Email)

			// Test filter by IsActive
			isActive := true
			filter = models.CustomerFilter{IsActive: &isActive}
			result, err = repo.ByFilter(ctx, filter, "", 0, 0)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(result), 3)

			// Test filter by AccountTypeName
			accountTypeName := models.AccountTypeIndividual
			filter = models.CustomerFilter{AccountTypeName: &accountTypeName}
			result, err = repo.ByFilter(ctx, filter, "", 0, 0)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(result), 1)
		})

		t.Run("ListActiveCustomers", func(t *testing.T) {
			// Create multiple test customers
			_, err := fixtures.CreateMultipleTestCustomers()
			require.NoError(t, err)

			customers, err := repo.ListActiveCustomers(ctx, 10, 0)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(customers), 3)

			// All returned customers should be active
			for _, customer := range customers {
				assert.True(t, utils.IsTrue(customer.IsActive))
			}
		})

		t.Run("ListByAgency", func(t *testing.T) {
			// Create an agency customer first
			agency, err := fixtures.CreateTestCustomer(models.AccountTypeMarketingAgency)
			require.NoError(t, err)

			// Create a customer with this agency as referrer
			individual, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)
			individual.ReferrerAgencyID = &agency.ID
			individual.Email = "referred@example.com"
			individual.RepresentativeMobile = "+989123456700"
			err = testDB.DB.Save(individual).Error
			require.NoError(t, err)

			// Find customers by agency
			customers, err := repo.ListByAgency(ctx, agency.ID)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(customers), 1)

			// Check that the returned customer has the correct referrer
			found := false
			for _, customer := range customers {
				if customer.ID == individual.ID {
					assert.Equal(t, agency.ID, *customer.ReferrerAgencyID)
					found = true
					break
				}
			}
			assert.True(t, found)
		})

		t.Run("Count", func(t *testing.T) {
			count, err := repo.Count(ctx, models.CustomerFilter{})
			require.NoError(t, err)
			assert.GreaterOrEqual(t, count, int64(1))
		})

		t.Run("Exists", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			exists, err := repo.Exists(ctx, models.CustomerFilter{Email: &customer.Email})
			require.NoError(t, err)
			assert.True(t, exists)

			nonExistentEmail := "nonexistent@example.com"
			exists, err = repo.Exists(ctx, models.CustomerFilter{Email: &nonExistentEmail})
			require.NoError(t, err)
			assert.False(t, exists)
		})

		return nil
	})
	require.NoError(t, err)
}

func TestOTPVerificationRepository(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		repo := repository.NewOTPVerificationRepository(testDB.DB)
		fixtures := testingutil.NewTestFixtures(testDB)
		ctx := testingutil.CreateTestContext()

		t.Run("Save", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otp, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)
			assert.NotZero(t, otp.ID)
		})

		t.Run("ByID", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			originalOTP, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			otp, err := repo.ByID(ctx, originalOTP.ID)
			require.NoError(t, err)
			assert.NotNil(t, otp)
			assert.Equal(t, originalOTP.ID, otp.ID)
			assert.Equal(t, originalOTP.CustomerID, otp.CustomerID)
		})

		t.Run("ByCustomerAndType", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create multiple OTPs for this customer
			otp1, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			otp2, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeEmail, "654321")
			require.NoError(t, err)

			// Find mobile OTPs
			otps, err := repo.ByCustomerAndType(ctx, customer.ID, models.OTPTypeMobile)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(otps), 1)

			// Verify mobile OTP is included
			found := false
			for _, otp := range otps {
				if otp.ID == otp1.ID {
					found = true
					assert.Equal(t, models.OTPTypeMobile, otp.OTPType)
					break
				}
			}
			assert.True(t, found)

			// Find email OTPs
			otps, err = repo.ByCustomerAndType(ctx, customer.ID, models.OTPTypeEmail)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(otps), 1)

			// Verify email OTP is included
			found = false
			for _, otp := range otps {
				if otp.ID == otp2.ID {
					found = true
					assert.Equal(t, models.OTPTypeEmail, otp.OTPType)
					break
				}
			}
			assert.True(t, found)
		})

		t.Run("ByTargetAndType", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			originalOTP, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			otp, err := repo.ByTargetAndType(ctx, originalOTP.TargetValue, models.OTPTypeMobile)
			require.NoError(t, err)
			assert.NotNil(t, otp)
			assert.Equal(t, originalOTP.TargetValue, otp.TargetValue)
			assert.Equal(t, models.OTPTypeMobile, otp.OTPType)
		})

		t.Run("ListActiveOTPs", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create active OTP
			activeOTP, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			// Create expired OTP
			expiredOTP, err := fixtures.CreateExpiredOTP(customer.ID)
			require.NoError(t, err)

			otps, err := repo.ListActiveOTPs(ctx, customer.ID)
			require.NoError(t, err)

			// Should include active OTP but not expired one
			activeFound := false
			expiredFound := false
			for _, otp := range otps {
				if otp.ID == activeOTP.ID {
					activeFound = true
				}
				if otp.ID == expiredOTP.ID {
					expiredFound = true
				}
			}

			assert.True(t, activeFound, "Active OTP should be found")
			assert.False(t, expiredFound, "Expired OTP should not be found in active list")
		})

		t.Run("ExpireOldOTPs", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create pending OTP
			otp1, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			otp2, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "654321")
			require.NoError(t, err)

			// Expire old OTPs
			err = repo.ExpireOldOTPs(ctx, customer.ID, models.OTPTypeMobile)
			require.NoError(t, err)

			// Check that expired records were created
			otpType := models.OTPTypeMobile
			otpStatus := models.OTPStatusExpired
			filter := models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    &otpType,
				Status:     &otpStatus,
			}

			expiredOTPs, err := repo.ByFilter(ctx, filter, "", 0, 0)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(expiredOTPs), 2)

			// Original OTPs should still exist
			otp, err := repo.ByID(ctx, otp1.ID)
			require.NoError(t, err)
			assert.NotNil(t, otp)

			otp, err = repo.ByID(ctx, otp2.ID)
			require.NoError(t, err)
			assert.NotNil(t, otp)
		})

		t.Run("ByFilter", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			otp, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			// Filter by customer ID
			filter := models.OTPVerificationFilter{CustomerID: &customer.ID}
			otps, err := repo.ByFilter(ctx, filter, "", 0, 0)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(otps), 1)

			// Filter by OTP type
			otpType := models.OTPTypeMobile
			filter = models.OTPVerificationFilter{
				CustomerID: &customer.ID,
				OTPType:    &otpType,
			}
			otps, err = repo.ByFilter(ctx, filter, "", 0, 0)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(otps), 1)

			// Verify the OTP is included
			found := false
			for _, o := range otps {
				if o.ID == otp.ID {
					found = true
					break
				}
			}
			assert.True(t, found)
		})

		return nil
	})
	require.NoError(t, err)
}

func TestOTPVerificationRepositoryCorrelationID(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		fixtures := testingutil.NewTestFixtures(testDB)
		otpRepo := repository.NewOTPVerificationRepository(testDB.DB)

		t.Run("GetLatestByCorrelationID", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create initial OTP
			otp, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			// Get latest by correlation ID
			latestOTP, err := otpRepo.GetLatestByCorrelationID(context.Background(), otp.CorrelationID)
			require.NoError(t, err)
			require.NotNil(t, latestOTP)

			// Should return the original OTP
			assert.Equal(t, otp.ID, latestOTP.ID)
			assert.Equal(t, otp.CorrelationID, latestOTP.CorrelationID)
		})

		t.Run("GetLatestByCorrelationIDNotFound", func(t *testing.T) {
			// Try to get latest with non-existent correlation ID
			nonExistentID := uuid.New()
			latestOTP, err := otpRepo.GetLatestByCorrelationID(context.Background(), nonExistentID)
			require.NoError(t, err)
			assert.Nil(t, latestOTP)
		})

		t.Run("GetHistoryByCorrelationID", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create initial OTP
			otp, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			// Create expired OTP with same correlation ID
			expiredOTP := &models.OTPVerification{
				CorrelationID: otp.CorrelationID,
				CustomerID:    otp.CustomerID,
				OTPCode:       otp.OTPCode,
				OTPType:       otp.OTPType,
				TargetValue:   otp.TargetValue,
				Status:        models.OTPStatusExpired,
				AttemptsCount: otp.AttemptsCount,
				MaxAttempts:   otp.MaxAttempts,
				CreatedAt:     otp.CreatedAt,
				ExpiresAt:     time.Now(),
			}
			err = testDB.DB.Create(expiredOTP).Error
			require.NoError(t, err)

			// Get history by correlation ID
			history, err := otpRepo.GetHistoryByCorrelationID(context.Background(), otp.CorrelationID)
			require.NoError(t, err)
			require.Len(t, history, 2)

			// Should return both records ordered by ID DESC (latest first)
			assert.Equal(t, expiredOTP.ID, history[0].ID)
			assert.Equal(t, otp.ID, history[1].ID)
			assert.Equal(t, otp.CorrelationID, history[0].CorrelationID)
			assert.Equal(t, otp.CorrelationID, history[1].CorrelationID)
		})

		t.Run("ExpireOldOTPsWithCorrelationID", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create initial OTP
			otp, err := fixtures.CreateTestOTP(customer.ID, models.OTPTypeMobile, "123456")
			require.NoError(t, err)

			// Expire old OTPs
			err = otpRepo.ExpireOldOTPs(context.Background(), customer.ID, models.OTPTypeMobile)
			require.NoError(t, err)

			// Get history to verify correlation ID is preserved
			history, err := otpRepo.GetHistoryByCorrelationID(context.Background(), otp.CorrelationID)
			require.NoError(t, err)
			require.Len(t, history, 2)

			// Both records should have the same correlation ID
			assert.Equal(t, otp.CorrelationID, history[0].CorrelationID)
			assert.Equal(t, otp.CorrelationID, history[1].CorrelationID)

			// Latest should be expired
			assert.Equal(t, models.OTPStatusExpired, history[0].Status)
		})

		return nil
	})
	require.NoError(t, err)
}

func TestCustomerSessionRepository(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		repo := repository.NewCustomerSessionRepository(testDB.DB)
		fixtures := testingutil.NewTestFixtures(testDB)
		ctx := testingutil.CreateTestContext()

		t.Run("Save", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			session, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)
			assert.NotZero(t, session.ID)
		})

		t.Run("ByID", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			originalSession, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			session, err := repo.ByID(ctx, originalSession.ID)
			require.NoError(t, err)
			assert.NotNil(t, session)
			assert.Equal(t, originalSession.ID, session.ID)
			assert.Equal(t, originalSession.CustomerID, session.CustomerID)
		})

		t.Run("BySessionToken", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			originalSession, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			session, err := repo.BySessionToken(ctx, originalSession.SessionToken)
			require.NoError(t, err)
			assert.NotNil(t, session)
			assert.Equal(t, originalSession.ID, session.ID)
			assert.Equal(t, originalSession.SessionToken, session.SessionToken)
		})

		t.Run("BySessionTokenNotFound", func(t *testing.T) {
			session, err := repo.BySessionToken(ctx, "nonexistent_token")
			assert.NoError(t, err)
			assert.Nil(t, session)
		})

		t.Run("ByRefreshToken", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			originalSession, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			session, err := repo.ByRefreshToken(ctx, *originalSession.RefreshToken)
			require.NoError(t, err)
			assert.NotNil(t, session)
			assert.Equal(t, originalSession.ID, session.ID)
			assert.Equal(t, *originalSession.RefreshToken, *session.RefreshToken)
		})

		t.Run("ByRefreshTokenNotFound", func(t *testing.T) {
			session, err := repo.ByRefreshToken(ctx, "nonexistent_refresh_token")
			assert.NoError(t, err)
			assert.Nil(t, session)
		})

		t.Run("ListActiveSessionsByCustomer", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create multiple sessions
			session1, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			session2, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)
			session2.SessionToken, err = testingutil.GenerateSecureToken(32)
			require.NoError(t, err)

			refreshToken, err := testingutil.GenerateSecureToken(32)
			require.NoError(t, err)
			session2.RefreshToken = stringPtr(refreshToken)
			err = testDB.DB.Save(session2).Error
			require.NoError(t, err)

			// Create inactive session
			session3, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)
			session3.SessionToken, err = testingutil.GenerateSecureToken(32)
			require.NoError(t, err)

			refreshToken, err = testingutil.GenerateSecureToken(32)
			require.NoError(t, err)
			session3.RefreshToken = stringPtr(refreshToken)
			session3.IsActive = utils.ToPtr(false)
			err = testDB.DB.Save(session3).Error
			require.NoError(t, err)

			sessions, err := repo.ListActiveSessionsByCustomer(ctx, customer.ID)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(sessions), 2)

			// All returned sessions should be active
			for _, session := range sessions {
				assert.True(t, utils.IsTrue(session.IsActive))
				assert.Equal(t, customer.ID, session.CustomerID)
			}

			// Verify specific sessions are included
			sessionIDs := make([]uint, len(sessions))
			for i, session := range sessions {
				sessionIDs[i] = session.ID
			}
			assert.Contains(t, sessionIDs, session1.ID)
			assert.Contains(t, sessionIDs, session2.ID)
			assert.NotContains(t, sessionIDs, session3.ID) // Inactive session should not be included
		})

		t.Run("ByFilter", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			session, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			// Filter by customer ID
			filter := models.CustomerSessionFilter{CustomerID: &customer.ID}
			sessions, err := repo.ByFilter(ctx, filter, "", 0, 0)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(sessions), 1)

			// Filter by active status
			isActive := true
			filter = models.CustomerSessionFilter{
				CustomerID: &customer.ID,
				IsActive:   &isActive,
			}
			sessions, err = repo.ByFilter(ctx, filter, "", 0, 0)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(sessions), 1)

			// Verify the session is included
			found := false
			for _, s := range sessions {
				if s.ID == session.ID {
					found = true
					break
				}
			}
			assert.True(t, found)
		})

		return nil
	})
	require.NoError(t, err)
}

func TestCustomerSessionRepositoryCorrelationID(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		fixtures := testingutil.NewTestFixtures(testDB)
		sessionRepo := repository.NewCustomerSessionRepository(testDB.DB)

		t.Run("GetLatestByCorrelationID", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create initial session
			session, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			// Get latest by correlation ID
			latestSession, err := sessionRepo.GetLatestByCorrelationID(context.Background(), session.CorrelationID)
			require.NoError(t, err)
			require.NotNil(t, latestSession)

			// Should return the original session
			assert.Equal(t, session.ID, latestSession.ID)
			assert.Equal(t, session.CorrelationID, latestSession.CorrelationID)
		})

		t.Run("GetLatestByCorrelationIDNotFound", func(t *testing.T) {
			// Try to get latest with non-existent correlation ID
			nonExistentID := uuid.New()
			latestSession, err := sessionRepo.GetLatestByCorrelationID(context.Background(), nonExistentID)
			require.NoError(t, err)
			assert.Nil(t, latestSession)
		})

		t.Run("GetHistoryByCorrelationID", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create initial session
			session, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			// Create expired session with same correlation ID
			expiredSession := &models.CustomerSession{
				CorrelationID:  session.CorrelationID,
				CustomerID:     session.CustomerID,
				SessionToken:   session.SessionToken + "_expired",
				RefreshToken:   nil,
				DeviceInfo:     session.DeviceInfo,
				IPAddress:      session.IPAddress,
				UserAgent:      session.UserAgent,
				IsActive:       utils.ToPtr(false),
				CreatedAt:      session.CreatedAt,
				LastAccessedAt: time.Now(),
				ExpiresAt:      time.Now(),
			}
			err = testDB.DB.Create(expiredSession).Error
			require.NoError(t, err)

			// Get history by correlation ID
			history, err := sessionRepo.GetHistoryByCorrelationID(context.Background(), session.CorrelationID)
			require.NoError(t, err)
			require.Len(t, history, 2)

			// Should return both records ordered by ID DESC (latest first)
			assert.Equal(t, expiredSession.ID, history[0].ID)
			assert.Equal(t, session.ID, history[1].ID)
			assert.Equal(t, session.CorrelationID, history[0].CorrelationID)
			assert.Equal(t, session.CorrelationID, history[1].CorrelationID)
		})

		t.Run("ExpireSessionWithCorrelationID", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create initial session
			session, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			// Expire session
			err = sessionRepo.ExpireSession(context.Background(), session.ID)
			require.NoError(t, err)

			// Get history to verify correlation ID is preserved
			history, err := sessionRepo.GetHistoryByCorrelationID(context.Background(), session.CorrelationID)
			require.NoError(t, err)
			require.Len(t, history, 2)

			// Both records should have the same correlation ID
			assert.Equal(t, session.CorrelationID, history[0].CorrelationID)
			assert.Equal(t, session.CorrelationID, history[1].CorrelationID)

			// Latest should be inactive
			assert.False(t, utils.IsTrue(history[0].IsActive))
		})

		t.Run("ExpireAllCustomerSessionsWithCorrelationID", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			// Create multiple sessions
			session1, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			session2, err := fixtures.CreateTestSession(customer.ID)
			require.NoError(t, err)

			// Expire all sessions
			err = sessionRepo.ExpireAllCustomerSessions(context.Background(), customer.ID)
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

func TestAuditLogRepository(t *testing.T) {
	err := testingutil.TestWithDB(func(testDB *testingutil.TestDB) error {
		repo := repository.NewAuditLogRepository(testDB.DB)
		fixtures := testingutil.NewTestFixtures(testDB)
		ctx := testingutil.CreateTestContext()

		t.Run("Save", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			audit, err := fixtures.CreateTestAuditLog(&customer.ID, models.AuditActionLoginSuccess, true)
			require.NoError(t, err)
			assert.NotZero(t, audit.ID)
		})

		t.Run("ByID", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			originalAudit, err := fixtures.CreateTestAuditLog(&customer.ID, models.AuditActionLoginSuccess, true)
			require.NoError(t, err)

			audit, err := repo.ByID(ctx, originalAudit.ID)
			require.NoError(t, err)
			assert.NotNil(t, audit)
			assert.Equal(t, originalAudit.ID, audit.ID)
			assert.Equal(t, originalAudit.Action, audit.Action)
		})

		t.Run("ByFilter", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			audit1, err := fixtures.CreateTestAuditLog(&customer.ID, models.AuditActionLoginSuccess, true)
			require.NoError(t, err)

			audit2, err := fixtures.CreateTestAuditLog(&customer.ID, models.AuditActionLoginFailed, false)
			require.NoError(t, err)

			// Filter by customer ID
			filter := models.AuditLogFilter{CustomerID: &customer.ID}
			audits, err := repo.ByFilter(ctx, filter, "", 0, 0)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(audits), 2)

			// Filter by success status
			success := true
			filter = models.AuditLogFilter{
				CustomerID: &customer.ID,
				Success:    &success,
			}
			audits, err = repo.ByFilter(ctx, filter, "", 0, 0)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(audits), 1)

			// Verify successful audit is included
			found := false
			for _, a := range audits {
				if a.ID == audit1.ID {
					found = true
					assert.True(t, *a.Success)
					break
				}
			}
			assert.True(t, found)

			// Filter by action
			action := models.AuditActionLoginFailed
			filter = models.AuditLogFilter{
				CustomerID: &customer.ID,
				Action:     &action,
			}
			audits, err = repo.ByFilter(ctx, filter, "", 0, 0)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(audits), 1)

			// Verify failed audit is included
			found = false
			for _, a := range audits {
				if a.ID == audit2.ID {
					found = true
					assert.Equal(t, models.AuditActionLoginFailed, a.Action)
					break
				}
			}
			assert.True(t, found)
		})

		t.Run("SaveBatch", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			description1 := "Test audit 1"
			description2 := "Test audit 2"
			ipAddress := "127.0.0.1"
			userAgent := "Test User Agent"

			boolPtr := func(b bool) *bool {
				return &b
			}

			audits := []*models.AuditLog{
				{
					CustomerID:  &customer.ID,
					Action:      models.AuditActionLoginSuccess,
					Description: &description1,
					Success:     boolPtr(true),
					IPAddress:   &ipAddress,
					UserAgent:   &userAgent,
				},
				{
					CustomerID:   &customer.ID,
					Action:       models.AuditActionLoginFailed,
					Description:  &description2,
					Success:      boolPtr(false),
					IPAddress:    &ipAddress,
					UserAgent:    &userAgent,
					ErrorMessage: stringPtr("Test error message"),
				},
			}

			err = repo.SaveBatch(ctx, audits)
			require.NoError(t, err)

			// Verify both audits were saved
			for _, audit := range audits {
				assert.NotZero(t, audit.ID)
			}

			// Verify they can be retrieved
			saved, err := repo.ByID(ctx, audits[0].ID)
			require.NoError(t, err)
			assert.Equal(t, models.AuditActionLoginSuccess, saved.Action)

			saved, err = repo.ByID(ctx, audits[1].ID)
			require.NoError(t, err)
			assert.Equal(t, models.AuditActionLoginFailed, saved.Action)
		})

		t.Run("Count", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			_, err = fixtures.CreateTestAuditLog(&customer.ID, models.AuditActionLoginSuccess, true)
			require.NoError(t, err)

			count, err := repo.Count(ctx, models.AuditLogFilter{CustomerID: &customer.ID})
			require.NoError(t, err)
			assert.GreaterOrEqual(t, count, int64(1))
		})

		t.Run("Exists", func(t *testing.T) {
			customer, err := fixtures.CreateTestCustomer(models.AccountTypeIndividual)
			require.NoError(t, err)

			_, err = fixtures.CreateTestAuditLog(&customer.ID, models.AuditActionLoginSuccess, true)
			require.NoError(t, err)

			exists, err := repo.Exists(ctx, models.AuditLogFilter{CustomerID: &customer.ID})
			require.NoError(t, err)
			assert.True(t, exists)

			nonExistentCustomerID := uint(999999)
			exists, err = repo.Exists(ctx, models.AuditLogFilter{CustomerID: &nonExistentCustomerID})
			require.NoError(t, err)
			assert.False(t, exists)
		})

		return nil
	})
	require.NoError(t, err)
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
