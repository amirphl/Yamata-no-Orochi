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
	"github.com/redis/go-redis/v9"
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
	auditRepo       repository.AuditLogRepository
	accountTypeRepo repository.AccountTypeRepository
	tokenService    services.TokenService
	notificationSvc services.NotificationService
	db              *gorm.DB
	rc              *redis.Client
}

// NewLoginFlow creates a new login flow instance
func NewLoginFlow(
	customerRepo repository.CustomerRepository,
	sessionRepo repository.CustomerSessionRepository,
	auditRepo repository.AuditLogRepository,
	accountTypeRepo repository.AccountTypeRepository,
	tokenService services.TokenService,
	notificationSvc services.NotificationService,
	db *gorm.DB,
	rc *redis.Client,
) LoginFlow {
	return &LoginFlowImpl{
		customerRepo:    customerRepo,
		sessionRepo:     sessionRepo,
		auditRepo:       auditRepo,
		accountTypeRepo: accountTypeRepo,
		tokenService:    tokenService,
		notificationSvc: notificationSvc,
		db:              db,
		rc:              rc,
	}
}

// Login authenticates a user with email/mobile and password
func (lf *LoginFlowImpl) Login(ctx context.Context, req *dto.LoginRequest, metadata *ClientMetadata) (*dto.LoginResponse, error) {
	// Validate business rules
	if err := lf.validateLoginRequest(req); err != nil {
		return nil, NewBusinessError("LOGIN_VALIDATION_FAILED", "Login validation failed", err)
	}

	var customer *models.Customer
	var resp *dto.LoginResponse

	err := repository.WithTransaction(ctx, lf.db, func(txCtx context.Context) error {
		// Find customer by email or mobile
		var err error
		customer, err = lf.findCustomerByIdentifier(txCtx, req.Identifier)
		if err != nil {
			return err
		}
		if customer == nil {
			return ErrCustomerNotFound
		}
		if !utils.IsTrue(customer.IsActive) {
			return ErrAccountInactive
		}

		// Verify password
		if err := bcrypt.CompareHashAndPassword([]byte(customer.PasswordHash), []byte(req.Password)); err != nil {
			return ErrIncorrectPassword
		}

		// Get account type information
		accountType, err := lf.accountTypeRepo.ByID(txCtx, customer.AccountTypeID)
		if err != nil {
			return err
		}
		if accountType == nil {
			return ErrAccountTypeNotFound
		}

		// Create new session
		session, err := lf.createSession(txCtx, customer.ID, metadata)
		if err != nil {
			return err
		}

		resp = &dto.LoginResponse{
			Customer: ToAuthCustomerDTO(*customer),
			Session:  ToCustomerSessionDTO(*session),
		}

		return nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Login failed for identifier %s: %s", req.Identifier, err.Error())
		_ = lf.createAuditLog(ctx, customer, models.AuditActionLoginFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("LOGIN_FAILED", "Login failed", err)
	}

	msg := fmt.Sprintf("User logged in successfully for identifier %s", req.Identifier)
	_ = lf.createAuditLog(ctx, customer, models.AuditActionLoginSuccess, msg, true, nil, metadata)

	return resp, nil
}

// ForgotPassword initiates the password reset process
func (lf *LoginFlowImpl) ForgotPassword(ctx context.Context, req *dto.ForgotPasswordRequest, metadata *ClientMetadata) (*dto.ForgetPasswordResponse, error) {
	// Validate business rules
	if err := lf.validateForgotPasswordRequest(req); err != nil {
		return nil, NewBusinessError("FORGOT_PASSWORD_VALIDATION_FAILED", "Forgot password validation failed", err)
	}

	var customer *models.Customer
	var resp *dto.ForgetPasswordResponse

	err := repository.WithTransaction(ctx, lf.db, func(txCtx context.Context) error {
		var err error
		customer, err = lf.findCustomerByIdentifier(txCtx, req.Identifier)
		if err != nil {
			return err
		}
		if customer == nil {
			return ErrCustomerNotFound
		}
		if !utils.IsTrue(customer.IsActive) {
			return ErrAccountInactive
		}

		// Expire any existing password reset OTPs
		if err := lf.expireOldPasswordResetOTPs(txCtx, customer.ID); err != nil {
			return err
		}

		// Generate new OTP (Redis)
		otpCode, expiresAt, err := lf.generateAndSavePasswordResetOTP(txCtx, customer, metadata)
		if err != nil {
			return err
		}

		// Send OTP via SMS
		smsMessage := fmt.Sprintf("Your password reset code is: %s. This code will expire in %v minutes.", otpCode, utils.OTPExpiry.Minutes())
		customerID := int64(customer.ID)
		if err := lf.notificationSvc.SendSMS(txCtx, customer.RepresentativeMobile, smsMessage, &customerID); err != nil {
			// Log SMS failure but don't fail the entire process
			errMsg := fmt.Sprintf("OTP generated but SMS failed: %v", err)
			_ = lf.createAuditLog(txCtx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)
			// TODO: Retry sending OTP
		}

		resp = &dto.ForgetPasswordResponse{
			CustomerID:  customer.ID,
			MaskedPhone: dto.MaskPhoneNumber(customer.RepresentativeMobile),
			OTPExpiry:   expiresAt,
		}

		return nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Forgot password failed for identifier %s: %s", req.Identifier, err.Error())
		_ = lf.createAuditLog(ctx, customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("FORGOT_PASSWORD_FAILED", "Forgot password failed", err)
	}

	msg := fmt.Sprintf("Password reset OTP sent successfully for identifier %s", req.Identifier)
	_ = lf.createAuditLog(ctx, customer, models.AuditActionPasswordResetRequested, msg, true, nil, metadata)

	return resp, nil
}

// ResetPassword completes the password reset process with OTP verification
func (lf *LoginFlowImpl) ResetPassword(ctx context.Context, req *dto.ResetPasswordRequest, metadata *ClientMetadata) (*dto.ResetPasswordResponse, error) {
	// Validate business rules
	if err := lf.validateResetPasswordRequest(req); err != nil {
		return nil, NewBusinessError("RESET_PASSWORD_VALIDATION_FAILED", "Reset password validation failed", err)
	}

	var customer models.Customer
	var resp *dto.ResetPasswordResponse

	// Start transaction for password reset completion
	err := repository.WithTransaction(ctx, lf.db, func(txCtx context.Context) error {
		// Find the customer
		var err error
		customer, err = getCustomer(txCtx, lf.customerRepo, req.CustomerID)
		if err != nil {
			return err
		}

		// Verify OTP
		if err := lf.verifyPasswordResetOTP(txCtx, customer.ID, req.OTPCode); err != nil {
			return err
		}

		// Hash the new password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}

		// Update customer password (maintain referential integrity)
		err = lf.customerRepo.UpdatePassword(txCtx, customer.ID, string(hashedPassword))
		if err != nil {
			return err
		}

		// Invalidate all existing sessions for this customer
		if err := lf.invalidateAllSessions(txCtx, customer.ID); err != nil {
			return err
		}

		// Create new session for the user
		session, err := lf.createSession(txCtx, customer.ID, metadata)
		if err != nil {
			return err
		}

		resp = &dto.ResetPasswordResponse{
			Customer: ToAuthCustomerDTO(customer),
			Session:  ToCustomerSessionDTO(*session),
		}

		return nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Password reset failed for customer %d: %s", req.CustomerID, err.Error())
		_ = lf.createAuditLog(ctx, &customer, models.AuditActionPasswordResetFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("PASSWORD_RESET_FAILED", "Password reset failed", err)
	}

	msg := fmt.Sprintf("Password reset completed successfully for customer %d", req.CustomerID)
	_ = lf.createAuditLog(ctx, &customer, models.AuditActionPasswordResetCompleted, msg, true, nil, metadata)

	return resp, nil
}

// Private helper methods

func (lf *LoginFlowImpl) findCustomerByIdentifier(ctx context.Context, identifier string) (*models.Customer, error) {
	identifier = strings.TrimSpace(identifier)

	// Try to find by email first
	if strings.Contains(identifier, "@") {
		filter := models.CustomerFilter{
			Email:    &identifier,
			IsActive: utils.ToPtr(true),
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
			IsActive:             utils.ToPtr(true),
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

func (lf *LoginFlowImpl) createSession(ctx context.Context, customerID uint, metadata *ClientMetadata) (*models.CustomerSession, error) {
	// Generate tokens
	accessToken, refreshToken, err := lf.tokenService.GenerateTokens(customerID)
	if err != nil {
		return nil, err
	}

	ipAddress := ""
	userAgent := ""
	if metadata != nil {
		ipAddress = metadata.IPAddress
		userAgent = metadata.UserAgent
	}

	// Create session record
	session := &models.CustomerSession{
		CorrelationID: uuid.New(),
		CustomerID:    customerID,
		SessionToken:  accessToken,
		RefreshToken:  &refreshToken,
		// DeviceInfo:    json.RawMessage, // TODO: Add device info
		IPAddress:      &ipAddress,
		UserAgent:      &userAgent,
		IsActive:       utils.ToPtr(true),
		ExpiresAt:      utils.UTCNowAdd(utils.SessionTimeout),
		LastAccessedAt: utils.UTCNow(),
	}

	err = lf.sessionRepo.Save(ctx, session)
	if err != nil {
		return nil, err
	}

	return session, nil
}

func generateOTP() (string, error) {
	// Generate a secure 6-digit number using crypto/rand and math/big (consistent with signup_flow.go)
	max := big.NewInt(999999)
	min := big.NewInt(100000)

	n, err := rand.Int(rand.Reader, new(big.Int).Sub(max, min))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%06d", new(big.Int).Add(n, min).Int64()), nil
}

// generateAndSavePasswordResetOTP creates a new OTP and stores it in Redis with expiry
func (lf *LoginFlowImpl) generateAndSavePasswordResetOTP(ctx context.Context, customer *models.Customer, metadata *ClientMetadata) (string, time.Time, error) {
	if lf.rc == nil {
		return "", time.Time{}, ErrCacheNotAvailable
	}

	otpCode, err := generateOTP()
	if err != nil {
		return "", time.Time{}, err
	}

	expiresAt := utils.UTCNowAdd(utils.OTPExpiry)
	key := fmt.Sprintf("password_reset:otp:%d", customer.ID)
	if err := lf.rc.Set(ctx, key, otpCode, utils.OTPExpiry).Err(); err != nil {
		return "", time.Time{}, err
	}

	return otpCode, expiresAt, nil
}

// verifyPasswordResetOTP checks the OTP from Redis and consumes it on success
func (lf *LoginFlowImpl) verifyPasswordResetOTP(ctx context.Context, customerID uint, otpCode string) error {
	if lf.rc == nil {
		return ErrCacheNotAvailable
	}
	key := fmt.Sprintf("password_reset:otp:%d", customerID)
	val, err := lf.rc.Get(ctx, key).Result()
	if err == redis.Nil {
		return ErrNoValidOTPFound
	}
	if err != nil {
		return err
	}
	if val != otpCode {
		return ErrInvalidOTPCode
	}
	_ = lf.rc.Del(ctx, key).Err()
	return nil
}

func (lf *LoginFlowImpl) expireOldPasswordResetOTPs(ctx context.Context, customerID uint) error {
	if lf.rc != nil {
		key := fmt.Sprintf("password_reset:otp:%d", customerID)
		if err := lf.rc.Del(ctx, key).Err(); err != nil {
			return err
		}
		return nil
	}

	return nil
}

func (lf *LoginFlowImpl) invalidateAllSessions(ctx context.Context, customerID uint) error {
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
		session.IsActive = utils.ToPtr(false)
		session.ExpiresAt = utils.UTCNow()

		if err := lf.sessionRepo.Save(ctx, session); err != nil {
			return err
		}
	}

	return nil
}

func (lf *LoginFlowImpl) createAuditLog(ctx context.Context, customer *models.Customer, action string, description string, success bool, errMsg *string, metadata *ClientMetadata) error {
	var customerID *uint
	if customer != nil {
		customerID = &customer.ID
	}

	ipAddress := ""
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

	requestID := ctx.Value(utils.RequestIDKey)
	if requestID != nil {
		requestIDStr, ok := requestID.(string)
		if ok {
			audit.RequestID = &requestIDStr
		}
	}

	if err := lf.auditRepo.Save(ctx, audit); err != nil {
		return err
	}

	return nil
}

func (lf *LoginFlowImpl) validateLoginRequest(request *dto.LoginRequest) error {
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

func (lf *LoginFlowImpl) validateForgotPasswordRequest(request *dto.ForgotPasswordRequest) error {
	// Validate identifier is not empty
	if request.Identifier == "" {
		return ErrCustomerNotFound
	}

	return nil
}

func (lf *LoginFlowImpl) validateResetPasswordRequest(request *dto.ResetPasswordRequest) error {
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
