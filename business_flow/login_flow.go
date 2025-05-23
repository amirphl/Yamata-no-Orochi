// Package businessflow contains the core business logic and use cases for authentication workflows
package businessflow

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/app/services"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// LoginFlow handles user authentication and password reset operations
type LoginFlow interface {
	Login(ctx context.Context, request *dto.LoginRequest, metadata *ClientMetadata) (*LoginResult, error)
	ForgotPassword(ctx context.Context, request *dto.ForgotPasswordRequest, metadata *ClientMetadata) (*PasswordResetResult, error)
	ResetPassword(ctx context.Context, request *dto.ResetPasswordRequest, metadata *ClientMetadata) (*LoginResult, error)
}

// LoginFlowImpl implements the login business flow
type LoginFlowImpl struct {
	customerRepo    repository.CustomerRepository
	sessionRepo     repository.CustomerSessionRepository
	otpRepo         repository.OTPVerificationRepository
	auditRepo       repository.AuditLogRepository
	accountTypeRepo repository.AccountTypeRepository
	tokenService    services.TokenService
	notificationSvc services.NotificationService
	db              *gorm.DB
}

// NewLoginFlow creates a new login flow instance
func NewLoginFlow(
	customerRepo repository.CustomerRepository,
	sessionRepo repository.CustomerSessionRepository,
	otpRepo repository.OTPVerificationRepository,
	auditRepo repository.AuditLogRepository,
	accountTypeRepo repository.AccountTypeRepository,
	tokenService services.TokenService,
	notificationSvc services.NotificationService,
	db *gorm.DB,
) LoginFlow {
	return &LoginFlowImpl{
		customerRepo:    customerRepo,
		sessionRepo:     sessionRepo,
		otpRepo:         otpRepo,
		auditRepo:       auditRepo,
		accountTypeRepo: accountTypeRepo,
		tokenService:    tokenService,
		notificationSvc: notificationSvc,
		db:              db,
	}
}

// LoginResult represents the result of a login attempt
type LoginResult struct {
	Success      bool
	Customer     *models.Customer
	AccountType  *models.AccountType
	Session      *models.CustomerSession
	ErrorCode    string
	ErrorMessage string
}

// PasswordResetResult represents the result of a password reset request
type PasswordResetResult struct {
	Success      bool
	CustomerID   uint
	MaskedPhone  string
	OTPExpiry    time.Time
	ErrorCode    string
	ErrorMessage string
}

// Login authenticates a user with email/mobile and password
func (lf *LoginFlowImpl) Login(ctx context.Context, request *dto.LoginRequest, metadata *ClientMetadata) (*LoginResult, error) {
	// Start transaction for login process
	return lf.WithLoginTransaction(ctx, func(ctx context.Context) (*LoginResult, error) {
		// Find customer by email or mobile
		customer, err := lf.FindCustomerByIdentifier(ctx, request.Identifier)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to search customer: %s", request.Identifier)
			_ = lf.LogLoginAttempt(ctx, nil, models.AuditActionLoginFailed, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("CUSTOMER_SEARCH_FAILED", "Failed to search customer", err)
		}
		if customer == nil {
			errMsg := fmt.Sprintf("User not found: %s", request.Identifier)
			_ = lf.LogLoginAttempt(ctx, nil, models.AuditActionLoginFailed, errMsg, false, &errMsg, metadata)

			return &LoginResult{
				Success:      false,
				ErrorCode:    dto.ErrorUserNotFound,
				ErrorMessage: "User not found",
			}, nil
		}

		// Check if account is active
		if !utils.IsTrue(customer.IsActive) {
			errMsg := fmt.Sprintf("Login attempt on inactive account: %d", customer.ID)
			_ = lf.LogLoginAttempt(ctx, customer, models.AuditActionLoginFailed, errMsg, false, &errMsg, metadata)

			return &LoginResult{
				Success:      false,
				ErrorCode:    dto.ErrorAccountInactive,
				ErrorMessage: "Please contact support",
			}, nil
		}

		// Verify password
		if err := bcrypt.CompareHashAndPassword([]byte(customer.PasswordHash), []byte(request.Password)); err != nil {
			errMsg := fmt.Sprintf("Incorrect password for customer: %d", customer.ID)
			_ = lf.LogLoginAttempt(ctx, customer, models.AuditActionLoginFailed, errMsg, false, &errMsg, metadata)

			return &LoginResult{
				Success:      false,
				ErrorCode:    dto.ErrorIncorrectPassword,
				ErrorMessage: "Incorrect password",
			}, nil
		}

		// Get account type information
		accountType, err := lf.accountTypeRepo.ByID(ctx, customer.AccountTypeID)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to get account type: %d", customer.AccountTypeID)
			_ = lf.LogLoginAttempt(ctx, customer, models.AuditActionLoginFailed, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("ACCOUNT_TYPE_FETCH_FAILED", "Failed to get account type", err)
		}
		if accountType == nil {
			errMsg := fmt.Sprintf("Account type not found: %d", customer.AccountTypeID)
			_ = lf.LogLoginAttempt(ctx, customer, models.AuditActionLoginFailed, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("ACCOUNT_TYPE_FETCH_FAILED", "Failed to get account type", ErrAccountTypeNotFound)
		}

		// Create new session
		session, err := lf.CreateSession(ctx, customer.ID, metadata)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to create session: %d", customer.ID)
			_ = lf.LogLoginAttempt(ctx, customer, models.AuditActionLoginFailed, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("SESSION_CREATION_FAILED", "Failed to create session", err)
		}

		// Log successful login
		msg := fmt.Sprintf("User logged in successfully: %d", customer.ID)
		_ = lf.LogLoginAttempt(ctx, customer, models.AuditActionLoginSuccess, msg, true, nil, metadata)

		return &LoginResult{
			Success:     true,
			Customer:    customer,
			AccountType: accountType,
			Session:     session,
		}, nil
	})
}

// ForgotPassword initiates the password reset process
func (lf *LoginFlowImpl) ForgotPassword(ctx context.Context, request *dto.ForgotPasswordRequest, metadata *ClientMetadata) (*PasswordResetResult, error) {
	// Start transaction for password reset process
	return lf.WithPasswordResetTransaction(ctx, func(ctx context.Context) (*PasswordResetResult, error) {
		// Find customer by email or mobile
		customer, err := lf.FindCustomerByIdentifier(ctx, request.Identifier)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to search customer: %s", request.Identifier)
			_ = lf.LogLoginAttempt(ctx, nil, models.AuditActionLoginFailed, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("CUSTOMER_SEARCH_FAILED", "Failed to search customer", err)
		}
		if customer == nil {
			errMsg := fmt.Sprintf("Password reset requested for non-existent user: %s", request.Identifier)
			_ = lf.LogPasswordResetAttempt(ctx, nil, models.AuditActionPasswordResetRequested, errMsg, false, &errMsg, metadata)

			return &PasswordResetResult{
				Success:      false,
				ErrorCode:    dto.ErrorUserNotFound,
				ErrorMessage: "User not found",
			}, nil
		}

		// Check if account is active
		if !utils.IsTrue(customer.IsActive) {
			errMsg := fmt.Sprintf("Password reset attempted on inactive account: %d", customer.ID)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetRequested, errMsg, false, &errMsg, metadata)

			return &PasswordResetResult{
				Success:      false,
				ErrorCode:    dto.ErrorAccountInactive,
				ErrorMessage: "Please contact support",
			}, nil
		}

		// Expire any existing password reset OTPs
		if err := lf.ExpireOldPasswordResetOTPs(ctx, customer.ID); err != nil {
			errMsg := fmt.Sprintf("Failed to expire old OTPs: %d", customer.ID)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetRequested, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("OTP_EXPIRATION_FAILED", "Failed to expire old OTPs", err)
		}

		// Generate new OTP
		otpCode, err := GenerateOTP()
		if err != nil {
			errMsg := fmt.Sprintf("Failed to generate OTP: %d", customer.ID)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetRequested, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("OTP_GENERATION_FAILED", "Failed to generate OTP", err)
		}

		ipAddress := "127.0.0.1"
		userAgent := ""
		if metadata != nil {
			ipAddress = metadata.IPAddress
			userAgent = metadata.UserAgent
		}
		// Create OTP verification record
		otp := &models.OTPVerification{
			CustomerID:    customer.ID,
			CorrelationID: uuid.New(),
			OTPCode:       otpCode,
			OTPType:       models.OTPTypePasswordReset,
			TargetValue:   customer.RepresentativeMobile,
			Status:        models.OTPStatusPending,
			AttemptsCount: 0,
			MaxAttempts:   3,
			ExpiresAt:     time.Now().Add(utils.OTPExpiry),
			IPAddress:     &ipAddress,
			UserAgent:     &userAgent,
		}

		if err := lf.otpRepo.Save(ctx, otp); err != nil {
			errMsg := fmt.Sprintf("Failed to save OTP: %d", customer.ID)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetRequested, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("OTP_SAVE_FAILED", "Failed to save OTP", err)
		}

		// Send OTP via SMS
		smsMessage := fmt.Sprintf("Your password reset code is: %s. This code will expire in 5 minutes.", otpCode)
		if err := lf.notificationSvc.SendSMS(customer.RepresentativeMobile, smsMessage); err != nil {
			// Log SMS failure but don't fail the entire process
			errMsg := fmt.Sprintf("OTP generated but SMS failed: %v", err)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetRequested, errMsg, false, &errMsg, metadata)
			// TODO: Retry sending OTP
		}

		// Log successful password reset request
		msg := fmt.Sprintf("Password reset OTP sent successfully: %d", customer.ID)
		_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetRequested, msg, true, nil, metadata)

		return &PasswordResetResult{
			Success:     true,
			CustomerID:  customer.ID,
			MaskedPhone: dto.MaskPhoneNumber(customer.RepresentativeMobile),
			OTPExpiry:   otp.ExpiresAt,
		}, nil
	})
}

// ResetPassword completes the password reset process with OTP verification
func (lf *LoginFlowImpl) ResetPassword(ctx context.Context, request *dto.ResetPasswordRequest, metadata *ClientMetadata) (*LoginResult, error) {
	// Start transaction for password reset completion
	return lf.WithLoginTransaction(ctx, func(ctx context.Context) (*LoginResult, error) {
		// Find the customer
		customer, err := lf.customerRepo.ByID(ctx, request.CustomerID)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to find customer: %d", request.CustomerID)
			_ = lf.LogPasswordResetAttempt(ctx, nil, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("CUSTOMER_SEARCH_FAILED", "Failed to find customer", err)
		}
		if customer == nil {
			errMsg := fmt.Sprintf("Customer not found: %d", request.CustomerID)
			_ = lf.LogPasswordResetAttempt(ctx, nil, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

			return &LoginResult{
				Success:      false,
				ErrorCode:    dto.ErrorUserNotFound,
				ErrorMessage: "Customer not found",
			}, nil
		}

		// Find and verify OTP
		otpFilter := models.OTPVerificationFilter{
			CustomerID: &customer.ID,
			OTPType:    utils.ToPtr(models.OTPTypePasswordReset),
			OTPCode:    &request.OTPCode,
			Status:     utils.ToPtr(models.OTPStatusPending),
			IsActive:   utils.ToPtr(true),
		}

		otps, err := lf.otpRepo.ByFilter(ctx, otpFilter, "", 0, 0)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to find OTP: %d", customer.ID)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("OTP_SEARCH_FAILED", "Failed to find OTP", err)
		}

		var validOTP *models.OTPVerification
		for _, otp := range otps {
			if time.Now().Before(otp.ExpiresAt) {
				validOTP = otp
				break
			}
		}

		if validOTP == nil {
			// Check if there was an OTP but it's expired or wrong
			for _, otp := range otps {
				if time.Now().After(otp.ExpiresAt) {
					errMsg := fmt.Sprintf("Expired OTP used for password reset: %d", customer.ID)
					_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

					return &LoginResult{
						Success:      false,
						ErrorCode:    dto.ErrorOTPExpired,
						ErrorMessage: "OTP has expired",
					}, nil
				}
			}

			// Invalid OTP
			errMsg := fmt.Sprintf("Invalid OTP used for password reset: %d", customer.ID)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

			return &LoginResult{
				Success:      false,
				ErrorCode:    dto.ErrorInvalidOTP,
				ErrorMessage: "Invalid OTP",
			}, nil
		}

		// Hash the new password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to hash password: %d", customer.ID)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("PASSWORD_HASH_FAILED", "Failed to hash password", err)
		}

		// Update customer password (maintain referential integrity)
		err = lf.customerRepo.UpdatePassword(ctx, customer.ID, string(hashedPassword))
		if err != nil {
			errMsg := fmt.Sprintf("Failed to update customer password: %d", customer.ID)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("PASSWORD_UPDATE_FAILED", "Failed to update customer password", err)
		}

		// Mark OTP as used
		usedOTP := *validOTP
		usedOTP.ID = 0                                 // Reset ID to create new record (immutable design for OTP)
		usedOTP.CorrelationID = validOTP.CorrelationID // Use same correlation ID
		usedOTP.Status = models.OTPStatusUsed
		usedOTP.CreatedAt = time.Now()

		if err := lf.otpRepo.Save(ctx, &usedOTP); err != nil {
			errMsg := fmt.Sprintf("Failed to mark OTP as used: %d", customer.ID)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("OTP_MARK_USED_FAILED", "Failed to mark OTP as used", err)
		}

		// Invalidate all existing sessions for this customer
		if err := lf.InvalidateAllSessions(ctx, customer.ID); err != nil {
			errMsg := fmt.Sprintf("Failed to invalidate sessions: %d", customer.ID)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("SESSION_INVALIDATION_FAILED", "Failed to invalidate sessions", err)
		}

		// Get account type information
		accountType, err := lf.accountTypeRepo.ByID(ctx, customer.AccountTypeID)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to get account type: %d", customer.AccountTypeID)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("ACCOUNT_TYPE_FETCH_FAILED", "Failed to get account type", err)
		}
		if accountType == nil {
			errMsg := fmt.Sprintf("Account type not found: %d", customer.AccountTypeID)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("ACCOUNT_TYPE_FETCH_FAILED", "Failed to get account type", ErrAccountTypeNotFound)
		}

		// Create new session for the user
		session, err := lf.CreateSession(ctx, customer.ID, metadata)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to create session: %d", customer.ID)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

			return nil, NewBusinessError("SESSION_CREATION_FAILED", "Failed to create session", err)
		}

		// Log successful password reset
		msg := fmt.Sprintf("Password reset completed successfully: %d", customer.ID)
		_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetCompleted, msg, true, nil, metadata)

		return &LoginResult{
			Success:     true,
			Customer:    customer, // Use the existing customer (password updated in place)
			AccountType: accountType,
			Session:     session,
		}, nil
	})
}

// Private helper methods

func (lf *LoginFlowImpl) FindCustomerByIdentifier(ctx context.Context, identifier string) (*models.Customer, error) {
	identifier = strings.TrimSpace(identifier)

	// Try to find by email first
	if strings.Contains(identifier, "@") {
		filter := models.CustomerFilter{
			Email: &identifier,
		}
		customers, err := lf.customerRepo.ByFilter(ctx, filter, "", 0, 0)
		if err != nil {
			return nil, err
		}
		if len(customers) > 0 {
			// Return the most recent record (latest ID)
			return customers[0], nil
		}
	} else {
		// Try to find by mobile
		filter := models.CustomerFilter{
			RepresentativeMobile: &identifier,
		}
		customers, err := lf.customerRepo.ByFilter(ctx, filter, "", 0, 0)
		if err != nil {
			return nil, err
		}
		if len(customers) > 0 {
			// Return the most recent record (latest ID)
			return customers[0], nil
		}
	}

	return nil, nil
}

func (lf *LoginFlowImpl) CreateSession(ctx context.Context, customerID uint, metadata *ClientMetadata) (*models.CustomerSession, error) {
	// Generate tokens
	accessToken, refreshToken, err := lf.tokenService.GenerateTokens(customerID)
	if err != nil {
		return nil, err
	}

	// Calculate expiry time using constant
	expiresAt := time.Now().Add(utils.SessionTimeout)

	ipAddress := "127.0.0.1"
	userAgent := ""
	if metadata != nil {
		ipAddress = metadata.IPAddress
		userAgent = metadata.UserAgent
	}

	// Create session record
	session := &models.CustomerSession{
		CustomerID:    customerID,
		CorrelationID: uuid.New(),
		SessionToken:  accessToken,
		RefreshToken:  &refreshToken,
		ExpiresAt:     expiresAt,
		IsActive:      utils.ToPtr(true),
		IPAddress:     &ipAddress,
		UserAgent:     &userAgent,
	}

	err = lf.sessionRepo.Save(ctx, session)
	if err != nil {
		return nil, err
	}

	return session, nil
}

func GenerateOTP() (string, error) {
	// Generate a secure 6-digit number using crypto/rand and math/big (consistent with signup_flow.go)
	max := big.NewInt(999999)
	min := big.NewInt(100000)

	n, err := rand.Int(rand.Reader, new(big.Int).Sub(max, min))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%06d", new(big.Int).Add(n, min).Int64()), nil
}

func (lf *LoginFlowImpl) ExpireOldPasswordResetOTPs(ctx context.Context, customerID uint) error {
	// Find all pending password reset OTPs for this customer
	filter := models.OTPVerificationFilter{
		CustomerID: &customerID,
		OTPType:    utils.ToPtr(models.OTPTypePasswordReset),
		Status:     utils.ToPtr(models.OTPStatusPending),
	}

	otps, err := lf.otpRepo.ByFilter(ctx, filter, "", 0, 0)
	if err != nil {
		return err
	}

	// Mark all as expired (create new expired records)
	for _, otp := range otps {
		expiredOTP := *otp
		expiredOTP.ID = 0
		expiredOTP.CorrelationID = otp.CorrelationID // Use same correlation ID
		expiredOTP.Status = models.OTPStatusExpired
		expiredOTP.CreatedAt = time.Now()

		if err := lf.otpRepo.Save(ctx, &expiredOTP); err != nil {
			return err
		}
	}

	return nil
}

func (lf *LoginFlowImpl) InvalidateAllSessions(ctx context.Context, customerID uint) error {
	// Find all active sessions for this customer
	filter := models.CustomerSessionFilter{
		CustomerID: &customerID,
		IsActive:   utils.ToPtr(true),
	}

	sessions, err := lf.sessionRepo.ByFilter(ctx, filter, "", 0, 0)
	if err != nil {
		return err
	}

	// Mark all as inactive (create new inactive records)
	for _, session := range sessions {
		inactiveSession := *session
		inactiveSession.ID = 0
		inactiveSession.CorrelationID = session.CorrelationID // Use same correlation ID
		inactiveSession.IsActive = utils.ToPtr(false)
		inactiveSession.CreatedAt = time.Now()

		if err := lf.sessionRepo.Save(ctx, &inactiveSession); err != nil {
			return err
		}
	}

	return nil
}

func (lf *LoginFlowImpl) LogLoginAttempt(ctx context.Context, customer *models.Customer, action string, description string, success bool, errMsg *string, metadata *ClientMetadata) error {
	var customerID *uint
	if customer != nil {
		customerID = &customer.ID
	}

	ipAddress := "127.0.0.1"
	userAgent := ""
	if metadata != nil {
		ipAddress = metadata.IPAddress
		userAgent = metadata.UserAgent
	}

	audit := &models.AuditLog{
		CustomerID:   customerID,
		Action:       action,
		Description:  &description,
		Success:      utils.ToPtr(success),
		IPAddress:    &ipAddress,
		UserAgent:    &userAgent,
		ErrorMessage: errMsg,
	}

	requestID := ctx.Value(RequestIDKey)
	if requestID != nil {
		requestIDStr, ok := requestID.(string)
		if ok {
			audit.RequestID = &requestIDStr
		}
	}

	return lf.auditRepo.Save(ctx, audit)
}

func (lf *LoginFlowImpl) LogPasswordResetAttempt(ctx context.Context, customer *models.Customer, action string, description string, success bool, errMsg *string, metadata *ClientMetadata) error {
	var customerID *uint
	if customer != nil {
		customerID = &customer.ID
	}

	ipAddress := "127.0.0.1"
	userAgent := ""
	if metadata != nil {
		ipAddress = metadata.IPAddress
		userAgent = metadata.UserAgent
	}

	audit := &models.AuditLog{
		CustomerID:   customerID,
		Action:       action,
		Description:  &description,
		Success:      utils.ToPtr(success),
		IPAddress:    &ipAddress,
		UserAgent:    &userAgent,
		ErrorMessage: errMsg,
	}

	requestID := ctx.Value(RequestIDKey)
	if requestID != nil {
		requestIDStr, ok := requestID.(string)
		if ok {
			audit.RequestID = &requestIDStr
		}
	}

	return lf.auditRepo.Save(ctx, audit)
}

func (lf *LoginFlowImpl) WithLoginTransaction(ctx context.Context, fn func(context.Context) (*LoginResult, error)) (*LoginResult, error) {
	var result *LoginResult
	var fnErr error

	err := repository.WithTransaction(ctx, lf.db, func(ctx context.Context) error {
		result, fnErr = fn(ctx)
		return fnErr
	})

	if err != nil {
		return nil, err
	}
	return result, fnErr
}

func (lf *LoginFlowImpl) WithPasswordResetTransaction(ctx context.Context, fn func(context.Context) (*PasswordResetResult, error)) (*PasswordResetResult, error) {
	var result *PasswordResetResult
	var fnErr error

	err := repository.WithTransaction(ctx, lf.db, func(ctx context.Context) error {
		result, fnErr = fn(ctx)
		return fnErr
	})

	if err != nil {
		return nil, err
	}
	return result, fnErr
}
