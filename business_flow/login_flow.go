// Package businessflow contains the core business logic and use cases for authentication workflows
package businessflow

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

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
	Login(ctx context.Context, request *dto.LoginRequest, metadata *ClientMetadata) (*dto.LoginResponse, error)
	ForgotPassword(ctx context.Context, request *dto.ForgotPasswordRequest, metadata *ClientMetadata) (*dto.ForgetPasswordResponse, error)
	ResetPassword(ctx context.Context, request *dto.ResetPasswordRequest, metadata *ClientMetadata) (*dto.ResetPasswordResponse, error)
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

// Login authenticates a user with email/mobile and password
func (lf *LoginFlowImpl) Login(ctx context.Context, request *dto.LoginRequest, metadata *ClientMetadata) (*dto.LoginResponse, error) {
	// Validate business rules
	if err := lf.validateLoginRequest(ctx, request); err != nil {
		return nil, NewBusinessError("LOGIN_VALIDATION_FAILED", "Login validation failed", err)
	}

	var customer *models.Customer

	// Start transaction for login process
	resp, err := lf.WithLoginTransaction(ctx, func(ctx context.Context) (*dto.LoginResponse, error) {
		// Find customer by email or mobile
		var err error
		customer, err = lf.FindCustomerByIdentifier(ctx, request.Identifier)
		if err != nil {
			return nil, err
		}
		if customer == nil {
			return nil, ErrCustomerNotFound
		}

		// Check if account is active
		if !utils.IsTrue(customer.IsActive) {
			return nil, ErrAccountInactive
		}

		// Verify password
		if err := bcrypt.CompareHashAndPassword([]byte(customer.PasswordHash), []byte(request.Password)); err != nil {
			return nil, ErrIncorrectPassword
		}

		// Get account type information
		accountType, err := lf.accountTypeRepo.ByID(ctx, customer.AccountTypeID)
		if err != nil {
			return nil, err
		}
		if accountType == nil {
			return nil, ErrAccountTypeNotFound
		}

		// Create new session
		session, err := lf.CreateSession(ctx, customer.ID, metadata)
		if err != nil {
			return nil, err
		}

		return &dto.LoginResponse{
			Customer: ToAuthCustomerDTO(*customer),
			Session:  ToCustomerSessionDTO(*session),
		}, nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Login failed: %s", err.Error())
		_ = lf.LogLoginAttempt(ctx, customer, models.AuditActionLoginFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("LOGIN_FAILED", "Login failed", err)
	} else {
		msg := fmt.Sprintf("User logged in successfully: %d", resp.Customer.ID)
		_ = lf.LogLoginAttempt(ctx, customer, models.AuditActionLoginSuccess, msg, true, nil, metadata)
	}

	return resp, nil
}

// ForgotPassword initiates the password reset process
func (lf *LoginFlowImpl) ForgotPassword(ctx context.Context, request *dto.ForgotPasswordRequest, metadata *ClientMetadata) (*dto.ForgetPasswordResponse, error) {
	// Validate business rules
	if err := lf.validateForgotPasswordRequest(ctx, request); err != nil {
		return nil, NewBusinessError("FORGOT_PASSWORD_VALIDATION_FAILED", "Forgot password validation failed", err)
	}

	var customer *models.Customer

	// Start transaction for password reset process
	resp, err := lf.WithForgotPasswordTransaction(ctx, func(ctx context.Context) (*dto.ForgetPasswordResponse, error) {
		// Find customer by email or mobile
		var err error
		customer, err = lf.FindCustomerByIdentifier(ctx, request.Identifier)
		if err != nil {
			return nil, err
		}
		if customer == nil {
			return nil, ErrCustomerNotFound
		}

		// Check if account is active
		if !utils.IsTrue(customer.IsActive) {
			return nil, ErrAccountInactive
		}

		// Expire any existing password reset OTPs
		if err := lf.ExpireOldPasswordResetOTPs(ctx, customer.ID); err != nil {
			return nil, err
		}

		// Generate new OTP
		otpCode, err := GenerateOTP()
		if err != nil {
			return nil, err
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
			ExpiresAt:     utils.UTCNowAdd(utils.OTPExpiry),
			IPAddress:     &ipAddress,
			UserAgent:     &userAgent,
		}

		if err := lf.otpRepo.Save(ctx, otp); err != nil {
			return nil, err
		}

		// Send OTP via SMS
		smsMessage := fmt.Sprintf("Your password reset code is: %s. This code will expire in 5 minutes.", otpCode)
		if err := lf.notificationSvc.SendSMS(customer.RepresentativeMobile, smsMessage); err != nil {
			// Log SMS failure but don't fail the entire process
			errMsg := fmt.Sprintf("OTP generated but SMS failed: %v", err)
			_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)
			// TODO: Retry sending OTP
		}

		return &dto.ForgetPasswordResponse{
			CustomerID:  customer.ID,
			MaskedPhone: dto.MaskPhoneNumber(customer.RepresentativeMobile),
			OTPExpiry:   otp.ExpiresAt,
		}, nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Forgot password failed: %s", err.Error())
		_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("FORGOT_PASSWORD_FAILED", "Forgot password failed", err)
	} else {
		msg := fmt.Sprintf("Password reset OTP sent successfully: %d", resp.CustomerID)
		_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetRequested, msg, true, nil, metadata)
	}

	return resp, nil
}

// ResetPassword completes the password reset process with OTP verification
func (lf *LoginFlowImpl) ResetPassword(ctx context.Context, request *dto.ResetPasswordRequest, metadata *ClientMetadata) (*dto.ResetPasswordResponse, error) {
	// Validate business rules
	if err := lf.validateResetPasswordRequest(ctx, request); err != nil {
		return nil, NewBusinessError("RESET_PASSWORD_VALIDATION_FAILED", "Reset password validation failed", err)
	}

	var customer *models.Customer

	// Start transaction for password reset completion
	resp, err := lf.WithResetPasswordTransaction(ctx, func(ctx context.Context) (*dto.ResetPasswordResponse, error) {
		// Find the customer
		var err error
		customer, err = lf.customerRepo.ByID(ctx, request.CustomerID)
		if err != nil {
			return nil, err
		}
		if customer == nil {
			return nil, ErrCustomerNotFound
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
			return nil, err
		}

		var validOTP *models.OTPVerification
		for _, otp := range otps {
			if utils.UTCNow().Before(otp.ExpiresAt) {
				validOTP = otp
				break
			}
		}

		if validOTP == nil {
			// Check if there was an OTP but it's expired or wrong
			for _, otp := range otps {
				if utils.UTCNow().After(otp.ExpiresAt) {
					return nil, ErrOTPExpired
				}
			}

			// Invalid OTP
			return nil, ErrInvalidOTPCode
		}

		// Hash the new password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}

		// Update customer password (maintain referential integrity)
		err = lf.customerRepo.UpdatePassword(ctx, customer.ID, string(hashedPassword))
		if err != nil {
			return nil, err
		}

		// Mark OTP as used
		usedOTP := *validOTP
		usedOTP.ID = 0                                 // Reset ID to create new record (immutable design for OTP)
		usedOTP.CorrelationID = validOTP.CorrelationID // Use same correlation ID
		usedOTP.Status = models.OTPStatusUsed
		usedOTP.CreatedAt = utils.UTCNow()

		if err := lf.otpRepo.Save(ctx, &usedOTP); err != nil {
			return nil, err
		}

		// Invalidate all existing sessions for this customer
		if err := lf.InvalidateAllSessions(ctx, customer.ID); err != nil {
			return nil, err
		}

		// Get account type information
		accountType, err := lf.accountTypeRepo.ByID(ctx, customer.AccountTypeID)
		if err != nil {
			return nil, err
		}
		if accountType == nil {
			return nil, ErrAccountTypeNotFound
		}

		// Create new session for the user
		session, err := lf.CreateSession(ctx, customer.ID, metadata)
		if err != nil {
			return nil, err
		}

		return &dto.ResetPasswordResponse{
			Customer: ToAuthCustomerDTO(*customer),
			Session:  ToCustomerSessionDTO(*session),
		}, nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Password reset failed: %s", err.Error())
		_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("PASSWORD_RESET_FAILED", "Password reset failed", err)
	} else {
		msg := fmt.Sprintf("Password reset completed successfully: %d", resp.Customer.ID)
		_ = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetCompleted, msg, true, nil, metadata)
	}

	return resp, nil
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
	expiresAt := utils.UTCNowAdd(utils.SessionTimeout)

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
		expiredOTP.CreatedAt = utils.UTCNow()

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
		inactiveSession.CreatedAt = utils.UTCNow()

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

func (lf *LoginFlowImpl) WithLoginTransaction(ctx context.Context, fn func(context.Context) (*dto.LoginResponse, error)) (*dto.LoginResponse, error) {
	var result *dto.LoginResponse
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

func (lf *LoginFlowImpl) WithForgotPasswordTransaction(ctx context.Context, fn func(context.Context) (*dto.ForgetPasswordResponse, error)) (*dto.ForgetPasswordResponse, error) {
	var result *dto.ForgetPasswordResponse
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

func (lf *LoginFlowImpl) WithResetPasswordTransaction(ctx context.Context, fn func(context.Context) (*dto.ResetPasswordResponse, error)) (*dto.ResetPasswordResponse, error) {
	var result *dto.ResetPasswordResponse
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

func (lf *LoginFlowImpl) validateLoginRequest(ctx context.Context, request *dto.LoginRequest) error {
	// Validate identifier is not empty
	if request.Identifier == "" {
		return ErrCustomerNotFound
	}

	// Validate password is not empty
	if request.Password == "" {
		return ErrIncorrectPassword
	}

	return nil
}

func (lf *LoginFlowImpl) validateForgotPasswordRequest(ctx context.Context, request *dto.ForgotPasswordRequest) error {
	// Validate identifier is not empty
	if request.Identifier == "" {
		return ErrCustomerNotFound
	}

	return nil
}

func (lf *LoginFlowImpl) validateResetPasswordRequest(ctx context.Context, request *dto.ResetPasswordRequest) error {
	// Validate customer exists
	customer, err := lf.customerRepo.ByID(ctx, request.CustomerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return ErrCustomerNotFound
	}

	// Validate OTP code format (6 digits)
	if len(request.OTPCode) != 6 {
		return ErrInvalidOTPCode
	}

	// Validate password is not empty
	if request.NewPassword == "" {
		return ErrIncorrectPassword
	}

	// Validate password confirmation matches
	if request.NewPassword != request.ConfirmPassword {
		return ErrIncorrectPassword
	}

	return nil
}
