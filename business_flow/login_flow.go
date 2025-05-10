// Package business_flow contains the core business logic and use cases for authentication workflows
package business_flow

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"
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

type LoginFlow interface {
	Login(ctx context.Context, request *dto.LoginRequest, ipAddress, userAgent string) (*LoginResult, error)
	ForgotPassword(ctx context.Context, request *dto.ForgotPasswordRequest, ipAddress, userAgent string) (*PasswordResetResult, error)
	ResetPassword(ctx context.Context, request *dto.ResetPasswordRequest, ipAddress, userAgent string) (*LoginResult, error)
}

// LoginFlowImpl handles user authentication and password reset operations
type LoginFlowImpl struct {
	customerRepo    repository.CustomerRepository
	sessionRepo     repository.CustomerSessionRepository
	otpRepo         repository.OTPVerificationRepository
	auditRepo       repository.AuditLogRepository
	accountTypeRepo repository.AccountTypeRepository
	tokenSvc        services.TokenService
	notificationSvc services.NotificationService
	db              *gorm.DB
}

// NewLoginFlow creates a new instance of LoginFlow
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
		tokenSvc:        tokenService,
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
func (lf *LoginFlowImpl) Login(ctx context.Context, request *dto.LoginRequest, ipAddress, userAgent string) (*LoginResult, error) {
	// Start transaction for login process
	return lf.WithLoginTransaction(ctx, func(ctx context.Context) (*LoginResult, error) {
		// Find customer by email or mobile
		customer, err := lf.FindCustomerByIdentifier(ctx, request.Identifier)
		if err != nil {
			// Log failed login attempt
			errMsg := fmt.Sprintf("User not found: %s", request.Identifier)
			err := lf.LogLoginAttempt(ctx, nil, models.AuditActionLoginFailed, errMsg, false, &errMsg, ipAddress, userAgent)
			if err != nil {
				return nil, fmt.Errorf("failed to log login attempt: %w", err)
			}

			return &LoginResult{
				Success:      false,
				ErrorCode:    dto.ErrorUserNotFound,
				ErrorMessage: "User not found",
			}, nil
		}

		// Check if account is active
		if !utils.IsTrue(customer.IsActive) {
			// Log inactive account login attempt
			errMsg := fmt.Sprintf("Login attempt on inactive account: %d", customer.ID)
			err := lf.LogLoginAttempt(ctx, customer, models.AuditActionLoginFailed, errMsg, false, &errMsg, ipAddress, userAgent)
			if err != nil {
				return nil, fmt.Errorf("failed to log login attempt: %w", err)
			}

			return &LoginResult{
				Success:      false,
				ErrorCode:    dto.ErrorAccountInactive,
				ErrorMessage: "Please contact support",
			}, nil
		}

		// Verify password
		if err := bcrypt.CompareHashAndPassword([]byte(customer.PasswordHash), []byte(request.Password)); err != nil {
			// Log failed password attempt
			errMsg := fmt.Sprintf("Incorrect password for customer: %d", customer.ID)
			_ = lf.LogLoginAttempt(ctx, customer, models.AuditActionLoginFailed, errMsg, false, &errMsg, ipAddress, userAgent)

			return &LoginResult{
				Success:      false,
				ErrorCode:    dto.ErrorIncorrectPassword,
				ErrorMessage: "Incorrect password",
			}, nil
		}

		// Get account type information
		accountType, err := lf.accountTypeRepo.ByID(ctx, customer.AccountTypeID)
		if err != nil {
			return nil, fmt.Errorf("failed to get account type: %w", err)
		}
		if accountType == nil {
			return nil, fmt.Errorf("account type not found")
		}

		// Create new session
		session, err := lf.CreateSession(ctx, customer.ID, ipAddress, userAgent)
		if err != nil {
			return nil, fmt.Errorf("failed to create session: %w", err)
		}

		// Log successful login
		msg := fmt.Sprintf("User logged in successfully: %d", customer.ID)
		err = lf.LogLoginAttempt(ctx, customer, models.AuditActionLoginSuccessful, msg, true, nil, ipAddress, userAgent)
		if err != nil {
			return nil, fmt.Errorf("failed to log login attempt: %w", err)
		}

		return &LoginResult{
			Success:     true,
			Customer:    customer,
			AccountType: accountType,
			Session:     session,
		}, nil
	})
}

// ForgotPassword initiates the password reset process
func (lf *LoginFlowImpl) ForgotPassword(ctx context.Context, request *dto.ForgotPasswordRequest, ipAddress, userAgent string) (*PasswordResetResult, error) {
	// Start transaction for password reset process
	return lf.WithPasswordResetTransaction(ctx, func(ctx context.Context) (*PasswordResetResult, error) {
		// Find customer by email or mobile
		customer, err := lf.FindCustomerByIdentifier(ctx, request.Identifier)
		if err != nil {
			// Log failed password reset attempt
			errMsg := fmt.Sprintf("Password reset requested for non-existent user: %s", request.Identifier)
			err := lf.LogPasswordResetAttempt(ctx, nil, models.AuditActionPasswordResetRequested, errMsg, false, &errMsg, ipAddress, userAgent)
			if err != nil {
				return nil, fmt.Errorf("failed to log password reset attempt: %w", err)
			}

			return &PasswordResetResult{
				Success:      false,
				ErrorCode:    dto.ErrorUserNotFound,
				ErrorMessage: "User not found",
			}, nil
		}

		// Check if account is active
		if !utils.IsTrue(customer.IsActive) {
			errMsg := fmt.Sprintf("Password reset attempted on inactive account: %d", customer.ID)
			err := lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetRequested, errMsg, false, &errMsg, ipAddress, userAgent)
			if err != nil {
				return nil, fmt.Errorf("failed to log password reset attempt: %w", err)
			}

			return &PasswordResetResult{
				Success:      false,
				ErrorCode:    dto.ErrorAccountInactive,
				ErrorMessage: "Please contact support",
			}, nil
		}

		// Expire any existing password reset OTPs
		if err := lf.ExpireOldPasswordResetOTPs(ctx, customer.ID); err != nil {
			return nil, fmt.Errorf("failed to expire old OTPs: %w", err)
		}

		// Generate new OTP
		otpCode, err := lf.GenerateOTP()
		if err != nil {
			return nil, fmt.Errorf("failed to generate OTP: %w", err)
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
			ExpiresAt:     time.Now().Add(5 * time.Minute), // 5 minutes expiry
			IPAddress:     &ipAddress,
			UserAgent:     &userAgent,
		}

		if err := lf.otpRepo.Save(ctx, otp); err != nil {
			return nil, fmt.Errorf("failed to save OTP: %w", err)
		}

		// Send OTP via SMS
		smsMessage := fmt.Sprintf("Your password reset code is: %s. This code will expire in 5 minutes.", otpCode)
		if err := lf.notificationSvc.SendSMS(customer.RepresentativeMobile, smsMessage); err != nil {
			// Log SMS failure but don't fail the entire process
			errMsg := fmt.Sprintf("OTP generated but SMS failed: %v", err)
			err := lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetRequested, errMsg, false, &errMsg, ipAddress, userAgent)
			if err != nil {
				return nil, fmt.Errorf("failed to log password reset attempt: %w", err)
			}
		}

		// Log successful password reset request
		msg := fmt.Sprintf("Password reset OTP sent successfully: %d", customer.ID)
		err = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetRequested, msg, true, nil, ipAddress, userAgent)
		if err != nil {
			return nil, fmt.Errorf("failed to log password reset attempt: %w", err)
		}

		return &PasswordResetResult{
			Success:     true,
			CustomerID:  customer.ID,
			MaskedPhone: dto.MaskPhoneNumber(customer.RepresentativeMobile),
			OTPExpiry:   otp.ExpiresAt,
		}, nil
	})
}

// ResetPassword completes the password reset process with OTP verification
func (lf *LoginFlowImpl) ResetPassword(ctx context.Context, request *dto.ResetPasswordRequest, ipAddress, userAgent string) (*LoginResult, error) {
	// Start transaction for password reset completion
	return lf.WithLoginTransaction(ctx, func(ctx context.Context) (*LoginResult, error) {
		// Find the customer
		customer, err := lf.customerRepo.ByID(ctx, request.CustomerID)
		if err != nil {
			return &LoginResult{
				Success:      false,
				ErrorCode:    dto.ErrorUserNotFound,
				ErrorMessage: "Customer not found",
			}, nil
		}
		if customer == nil {
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
		}

		otps, err := lf.otpRepo.ByFilter(ctx, otpFilter, "", 0, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to find OTP: %w", err)
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
					err := lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, ipAddress, userAgent)
					if err != nil {
						return nil, fmt.Errorf("failed to log password reset attempt: %w", err)
					}

					return &LoginResult{
						Success:      false,
						ErrorCode:    dto.ErrorOTPExpired,
						ErrorMessage: "OTP has expired",
					}, nil
				}
			}

			// Invalid OTP
			errMsg := fmt.Sprintf("Invalid OTP used for password reset: %d", customer.ID)
			err := lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, ipAddress, userAgent)
			if err != nil {
				return nil, fmt.Errorf("failed to log password reset attempt: %w", err)
			}

			return &LoginResult{
				Success:      false,
				ErrorCode:    dto.ErrorInvalidOTP,
				ErrorMessage: "Invalid OTP",
			}, nil
		}

		// Hash the new password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}

		// Update customer password (maintain referential integrity)
		if err := lf.customerRepo.UpdatePassword(ctx, customer.ID, string(hashedPassword)); err != nil {
			return nil, fmt.Errorf("failed to update customer password: %w", err)
		}

		// Mark OTP as used
		usedOTP := *validOTP
		usedOTP.ID = 0                                 // Reset ID to create new record (immutable design for OTP)
		usedOTP.CorrelationID = validOTP.CorrelationID // Use same correlation ID
		usedOTP.Status = models.OTPStatusUsed
		usedOTP.CreatedAt = time.Now()

		if err := lf.otpRepo.Save(ctx, &usedOTP); err != nil {
			return nil, fmt.Errorf("failed to mark OTP as used: %w", err)
		}

		// Invalidate all existing sessions for this customer
		if err := lf.InvalidateAllSessions(ctx, customer.ID); err != nil {
			return nil, fmt.Errorf("failed to invalidate sessions: %w", err)
		}

		// Get account type information
		accountType, err := lf.accountTypeRepo.ByID(ctx, customer.AccountTypeID)
		if err != nil {
			return nil, fmt.Errorf("failed to get account type: %w", err)
		}
		if accountType == nil {
			return nil, fmt.Errorf("account type not found")
		}

		// Create new session for the user
		session, err := lf.CreateSession(ctx, customer.ID, ipAddress, userAgent)
		if err != nil {
			return nil, fmt.Errorf("failed to create session: %w", err)
		}

		// Log successful password reset
		msg := fmt.Sprintf("Password reset completed successfully: %d", customer.ID)
		err = lf.LogPasswordResetAttempt(ctx, customer, models.AuditActionPasswordResetCompleted, msg, true, nil, ipAddress, userAgent)
		if err != nil {
			return nil, fmt.Errorf("failed to log password reset attempt: %w", err)
		}

		return &LoginResult{
			Success:     true,
			Customer:    customer, // Use the existing customer (password updated in place)
			AccountType: accountType,
			Session:     session,
		}, nil
	})
}

// Helper methods

func (lf *LoginFlowImpl) FindCustomerByIdentifier(ctx context.Context, identifier string) (*models.Customer, error) {
	identifier = strings.TrimSpace(identifier)

	// Try to find by email first
	if strings.Contains(identifier, "@") {
		filter := models.CustomerFilter{
			Email: &identifier,
		}
		customers, err := lf.customerRepo.ByFilter(ctx, filter, "", 0, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to search by email: %w", err)
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
			return nil, fmt.Errorf("failed to search by mobile: %w", err)
		}
		if len(customers) > 0 {
			// Return the most recent record (latest ID)
			return customers[0], nil
		}
	}

	return nil, errors.New("user not found")
}

func (lf *LoginFlowImpl) CreateSession(ctx context.Context, customerID uint, ipAddress, userAgent string) (*models.CustomerSession, error) {
	// Generate tokens
	accessToken, refreshToken, err := lf.tokenSvc.GenerateTokens(customerID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	// Calculate expiry time (24 hours from now)
	expiresAt := time.Now().Add(24 * time.Hour)

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
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

func (lf *LoginFlowImpl) GenerateOTP() (string, error) {
	// Generate a 6-digit OTP using crypto/rand for security
	bytes := make([]byte, 3) // 3 bytes = 24 bits, enough for 6 digits
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random OTP: %w", err)
	}

	// Convert to 6-digit number (100000 to 999999)
	otpNum := (int(bytes[0])<<16|int(bytes[1])<<8|int(bytes[2]))%900000 + 100000
	return strconv.Itoa(otpNum), nil
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
		return fmt.Errorf("failed to find old password reset OTPs: %w", err)
	}

	// Mark all as expired (create new expired records)
	for _, otp := range otps {
		expiredOTP := *otp
		expiredOTP.ID = 0
		expiredOTP.CorrelationID = otp.CorrelationID // Use same correlation ID
		expiredOTP.Status = models.OTPStatusExpired
		expiredOTP.CreatedAt = time.Now()

		if err := lf.otpRepo.Save(ctx, &expiredOTP); err != nil {
			return fmt.Errorf("failed to expire OTP: %w", err)
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
		return fmt.Errorf("failed to find customer sessions: %w", err)
	}

	// Mark all as inactive (create new inactive records)
	for _, session := range sessions {
		inactiveSession := *session
		inactiveSession.ID = 0
		inactiveSession.CorrelationID = session.CorrelationID // Use same correlation ID
		inactiveSession.IsActive = utils.ToPtr(false)
		inactiveSession.CreatedAt = time.Now()

		if err := lf.sessionRepo.Save(ctx, &inactiveSession); err != nil {
			return fmt.Errorf("failed to invalidate session: %w", err)
		}
	}

	return nil
}

func (lf *LoginFlowImpl) LogLoginAttempt(ctx context.Context, customer *models.Customer, action string, description string, success bool, errMsg *string, ipAddress, userAgent string) error {
	var customerID *uint
	if customer != nil {
		customerID = &customer.ID
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

	traceID := ctx.Value(TraceIDKey)
	if traceID != nil {
		traceIDStr, ok := traceID.(string)
		if ok {
			audit.RequestID = &traceIDStr
		}
	}

	return lf.auditRepo.Save(ctx, audit)
}

func (lf *LoginFlowImpl) LogPasswordResetAttempt(ctx context.Context, customer *models.Customer, action string, description string, success bool, errMsg *string, ipAddress, userAgent string) error {
	var customerID *uint
	if customer != nil {
		customerID = &customer.ID
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

	traceID := ctx.Value(TraceIDKey)
	if traceID != nil {
		traceIDStr, ok := traceID.(string)
		if ok {
			audit.RequestID = &traceIDStr
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
