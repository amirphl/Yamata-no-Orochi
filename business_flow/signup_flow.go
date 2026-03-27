// Package businessflow contains the core business logic and use cases for authentication workflows
package businessflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/app/services"
	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// SignupFlow handles the complete signup business logic
type SignupFlow interface {
	Signup(ctx context.Context, req *dto.SignupRequest, metadata *ClientMetadata) (*dto.SignupResponse, error)
	VerifyOTP(ctx context.Context, req *dto.OTPVerificationRequest, metadata *ClientMetadata) (*dto.OTPVerificationResponse, error)
	ResendOTP(ctx context.Context, req *dto.OTPResendRequest, metadata *ClientMetadata) (*dto.OTPResendResponse, error)
}

// SignupFlowImpl implements the signup business flow
type SignupFlowImpl struct {
	customerRepo       repository.CustomerRepository
	accountTypeRepo    repository.AccountTypeRepository
	sessionRepo        repository.CustomerSessionRepository
	auditRepo          repository.AuditLogRepository
	agencyDiscountRepo repository.AgencyDiscountRepository
	walletRepo         repository.WalletRepository
	tokenService       services.TokenService
	notificationSvc    services.NotificationService
	adminConfig        config.AdminConfig
	messageConfig      config.MessageConfig
	db                 *gorm.DB
	rc                 *redis.Client
}

// NewSignupFlow creates a new signup flow instance
func NewSignupFlow(
	customerRepo repository.CustomerRepository,
	accountTypeRepo repository.AccountTypeRepository,
	sessionRepo repository.CustomerSessionRepository,
	auditRepo repository.AuditLogRepository,
	agencyDiscountRepo repository.AgencyDiscountRepository,
	walletRepo repository.WalletRepository,
	tokenService services.TokenService,
	notificationSvc services.NotificationService,
	adminConfig config.AdminConfig,
	messageConfig config.MessageConfig,
	db *gorm.DB,
	rc *redis.Client,
) SignupFlow {
	return &SignupFlowImpl{
		customerRepo:       customerRepo,
		accountTypeRepo:    accountTypeRepo,
		sessionRepo:        sessionRepo,
		auditRepo:          auditRepo,
		agencyDiscountRepo: agencyDiscountRepo,
		walletRepo:         walletRepo,
		tokenService:       tokenService,
		notificationSvc:    notificationSvc,
		adminConfig:        adminConfig,
		messageConfig:      messageConfig,
		db:                 db,
		rc:                 rc,
	}
}

// Signup handles the complete signup process
func (s *SignupFlowImpl) Signup(ctx context.Context, req *dto.SignupRequest, metadata *ClientMetadata) (*dto.SignupResponse, error) {
	// Validate business rules
	if err := s.validateSignupRequest(ctx, req); err != nil {
		return nil, NewBusinessError("SIGNUP_VALIDATION_FAILED", "Signup validation failed", err)
	}

	// Default referrer code if not provided
	if req.ReferrerAgencyCode == nil || len(strings.TrimSpace(*req.ReferrerAgencyCode)) == 0 {
		defaultCode := utils.DefaultReferrerAgencyCode
		req.ReferrerAgencyCode = &defaultCode
	}

	// Use transaction for atomicity
	var customer *models.Customer
	var otpCode string

	err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		// Create customer
		var err error
		customer, err = s.createCustomer(txCtx, req)
		if err != nil {
			return err
		}

		// Generate and save OTP
		otpCode, err = s.generateAndSaveOTP(txCtx, customer.ID, customer.RepresentativeMobile, models.OTPTypeMobile)
		if err != nil {
			return err
		}

		if err := s.createDefaultDiscount(txCtx, customer); err != nil {
			return err
		}

		//create wallet
		if _, err := createWalletWithInitialSnapshot(txCtx, s.walletRepo, customer.ID, "signup"); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Signup initiation failed: %s", err.Error())
		_ = s.createAuditLog(ctx, customer, models.AuditActionSignupFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("SIGNUP_FAILED", "Signup failed", err)
	}

	msg := fmt.Sprintf("Signup initiated successfully for customer %d", customer.ID)
	_ = s.createAuditLog(ctx, customer, models.AuditActionSignupInitiated, msg, true, nil, metadata)

	// Send OTP via SMS (outside transaction to avoid rollback on SMS failure)
	go func() {
		customerID := int64(customer.ID)
		message := fmt.Sprintf(s.messageConfig.SignupVerificationCodeTemplate, otpCode)
		err := s.notificationSvc.SendSMS(ctx, customer.RepresentativeMobile, message, &customerID)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to send SMS: %v", err)
			_ = s.createAuditLog(context.Background(), customer, models.AuditActionOTPSMSFailed, errMsg, false, &errMsg, metadata)
		}
	}()

	return &dto.SignupResponse{
		Message:    "Signup initiated successfully. OTP sent to your mobile number.",
		CustomerID: customer.ID,
		OTPSent:    true,
		OTPTarget:  s.maskMobileNumber(customer.RepresentativeMobile),
	}, nil
}

// VerifyOTP handles OTP verification and completes signup
func (s *SignupFlowImpl) VerifyOTP(ctx context.Context, req *dto.OTPVerificationRequest, metadata *ClientMetadata) (*dto.OTPVerificationResponse, error) {
	// Validate business rules
	if err := s.validateOTPVerificationRequest(ctx, req); err != nil {
		return nil, NewBusinessError("OTP_VERIFICATION_VALIDATION_FAILED", "OTP verification validation failed", err)
	}

	var customer models.Customer
	var tokens struct {
		access  string
		refresh string
	}

	err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		// Find customer
		var err error
		customer, err = getCustomer(txCtx, s.customerRepo, req.CustomerID)
		if err != nil {
			return err
		}

		// Verify OTP
		if err := s.verifyOTPCode(txCtx, req.CustomerID, req.OTPCode, req.OTPType); err != nil {
			return err
		}

		// Mark customer as verified and complete signup
		if err := s.completeSignup(txCtx, &customer, req.OTPType); err != nil {
			return err
		}

		// Generate tokens
		tokens.access, tokens.refresh, err = s.tokenService.GenerateTokens(customer.ID)
		if err != nil {
			return err
		}

		// Create session
		if err := s.createSession(txCtx, customer.ID, tokens.access, tokens.refresh, metadata); err != nil {
			return err
		}

		// Get customer again to get the updated customer
		customer, err = getCustomer(txCtx, s.customerRepo, customer.ID)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("OTP verification failed for customer %d: %s", req.CustomerID, err.Error())
		_ = s.createAuditLog(ctx, &customer, models.AuditActionOTPVerificationFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("OTP_VERIFICATION_FAILED", "OTP verification failed", err)
	}

	msg := fmt.Sprintf("Signup completed successfully for customer %d", customer.ID)
	_ = s.createAuditLog(ctx, &customer, models.AuditActionSignupCompleted, msg, true, nil, metadata)

	// Notify admins
	if s.notificationSvc != nil {
		adminMsg := fmt.Sprintf("New user verified: %s %s", customer.RepresentativeFirstName, customer.RepresentativeLastName)
		for _, mobile := range s.adminConfig.ActiveMobiles() {
			_ = s.notificationSvc.SendSMS(ctx, mobile, adminMsg, utils.ToPtr(int64(customer.ID)))
		}
	}

	return &dto.OTPVerificationResponse{
		Message:      "Signup completed successfully!",
		Token:        tokens.access,
		RefreshToken: tokens.refresh,
		Customer:     ToAuthCustomerDTO(customer),
	}, nil
}

// ResendOTP generates and sends a new OTP
func (s *SignupFlowImpl) ResendOTP(ctx context.Context, req *dto.OTPResendRequest, metadata *ClientMetadata) (*dto.OTPResendResponse, error) {
	// Validate business rules
	if err := s.validateOTPResendRequest(ctx, req); err != nil {
		return nil, NewBusinessError("OTP_RESEND_VALIDATION_FAILED", "OTP resend validation failed", err)
	}

	var customer models.Customer

	err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		// Find customer
		var err error
		customer, err = getCustomer(txCtx, s.customerRepo, req.CustomerID)
		if err != nil {
			return err
		}

		// TODO: Expire old OTPs
		// Expire old OTPs
		// if err := s.otpRepo.ExpireOldOTPs(txCtx, req.CustomerID, req.OTPType); err != nil {
		// 	return err
		// }

		// Generate new OTP
		var target string
		target = customer.RepresentativeMobile
		if req.OTPType == models.OTPTypeEmail {
			target = customer.Email
		}

		otpCode, err := s.generateAndSaveOTP(txCtx, req.CustomerID, target, req.OTPType)
		if err != nil {
			return err
		}

		// Send notification
		message := fmt.Sprintf(s.messageConfig.OTPResendVerificationCodeTemplate, otpCode, utils.OTPExpiry.Minutes())
		if req.OTPType == models.OTPTypeMobile {
			customerID := int64(req.CustomerID)
			return s.notificationSvc.SendSMS(ctx, target, message, &customerID)
		} else {
			return s.notificationSvc.SendEmail(target, "Verification Code", message)
		}
	})

	if err != nil {
		errMsg := fmt.Sprintf("Resend OTP failed for customer %d: %s", req.CustomerID, err.Error())
		_ = s.createAuditLog(ctx, &customer, models.AuditActionOTPResendFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("RESEND_OTP_FAILED", "Resend OTP failed", err)
	}

	msg := fmt.Sprintf("OTP resent successfully for customer %d", req.CustomerID)
	_ = s.createAuditLog(ctx, &customer, models.AuditActionOTPResent, msg, true, nil, metadata)

	return &dto.OTPResendResponse{
		Message:         "OTP resent successfully",
		OTPSent:         true,
		MaskedOTPTarget: s.maskMobileNumber(customer.RepresentativeMobile),
	}, nil
}

func (s *SignupFlowImpl) validateSignupRequest(ctx context.Context, req *dto.SignupRequest) error {
	// Check if email already exists
	existingCustomer, err := s.customerRepo.ByEmail(ctx, req.Email)
	if err != nil {
		return err
	}
	if existingCustomer != nil {
		return ErrEmailAlreadyExists
	}

	// Check if mobile already exists
	existingCustomer, err = s.customerRepo.ByMobile(ctx, req.RepresentativeMobile)
	if err != nil {
		return err
	}
	if existingCustomer != nil {
		return ErrMobileAlreadyExists
	}

	// Validate company fields for business accounts
	if req.AccountType == models.AccountTypeIndependentCompany || req.AccountType == models.AccountTypeMarketingAgency {
		if req.CompanyName == nil || req.NationalID == nil || req.CompanyPhone == nil ||
			req.CompanyAddress == nil || req.PostalCode == nil {
			return ErrCompanyFieldsRequired
		}

		// Check if national ID already exists
		if existingCustomer, err := s.customerRepo.ByNationalID(ctx, *req.NationalID); err != nil {
			return err
		} else if existingCustomer != nil {
			return ErrNationalIDAlreadyExists
		}
	}

	// National ID requirement for individual accounts
	if req.AccountType == models.AccountTypeIndividual {
		if req.NationalID == nil || *req.NationalID == "" {
			return ErrNationalIDRequired
		}

		if existingCustomer, err := s.customerRepo.ByNationalID(ctx, *req.NationalID); err != nil {
			return err
		} else if existingCustomer != nil {
			return ErrNationalIDAlreadyExists
		}
	}

	// Sheba requirement for marketing_agency
	if req.AccountType == models.AccountTypeMarketingAgency {
		shebaNumber, err := ValidateShebaNumber(req.ShebaNumber)
		if err != nil {
			return err
		}
		req.ShebaNumber = &shebaNumber
	}

	// Check if agency exists (for marketing_agency account type)
	if req.ReferrerAgencyCode != nil {
		agencyFilter := models.CustomerFilter{
			AgencyRefererCode: req.ReferrerAgencyCode,
			IsActive:          utils.ToPtr(true),
			IsMobileVerified:  utils.ToPtr(true),
		}
		agencies, err := s.customerRepo.ByFilter(ctx, agencyFilter, "", 0, 0)
		if err != nil {
			return err
		}
		if len(agencies) == 0 {
			return ErrReferrerAgencyNotFound
		}
	}

	// Require job and category for non-agency customers
	if req.AccountType != models.AccountTypeMarketingAgency {
		if req.Job == nil || *req.Job == "" || req.Category == nil || *req.Category == "" {
			return ErrJobCategoryRequired
		}
	}

	return nil
}

func (s *SignupFlowImpl) createCustomer(ctx context.Context, req *dto.SignupRequest) (*models.Customer, error) {
	// Get account type
	accountType, err := s.accountTypeRepo.ByTypeName(ctx, req.AccountType)
	if err != nil {
		return nil, err
	}
	if accountType == nil {
		return nil, ErrAccountTypeNotFound
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Find agency if provided
	var agencyID *uint
	if req.ReferrerAgencyCode != nil {
		// Find agency by agency_referer_code
		agency, err := s.customerRepo.ByAgencyRefererCode(ctx, *req.ReferrerAgencyCode)
		if err != nil {
			return nil, err
		}
		if agency == nil {
			return nil, ErrReferrerAgencyNotFound
		}
		if agency.AccountType.TypeName != models.AccountTypeMarketingAgency {
			return nil, ErrReferrerMustBeAgency
		}
		if !utils.IsTrue(agency.IsActive) {
			return nil, ErrReferrerAgencyInactive
		}
		agencyID = &agency.ID
	}

	// Create customer
	customer := &models.Customer{
		UUID:                    uuid.New(),
		AgencyRefererCode:       utils.GenerateRandomAgencyRefererCode(),
		AccountTypeID:           accountType.ID,
		CompanyName:             req.CompanyName,
		NationalID:              req.NationalID,
		CompanyPhone:            req.CompanyPhone,
		CompanyAddress:          req.CompanyAddress,
		PostalCode:              req.PostalCode,
		ShebaNumber:             req.ShebaNumber,
		Job:                     req.Job,
		Category:                req.Category,
		RepresentativeFirstName: req.RepresentativeFirstName,
		RepresentativeLastName:  req.RepresentativeLastName,
		RepresentativeMobile:    req.RepresentativeMobile,
		Email:                   req.Email,
		PasswordHash:            string(hashedPassword),
		ReferrerAgencyID:        agencyID,
		IsEmailVerified:         utils.ToPtr(false),
		IsMobileVerified:        utils.ToPtr(false),
		IsActive:                utils.ToPtr(true),
	}

	err = s.customerRepo.Save(ctx, customer)
	if err != nil {
		return nil, err
	}

	return customer, nil
}

func (s *SignupFlowImpl) generateAndSaveOTP(ctx context.Context, customerID uint, target, otpType string) (string, error) {
	// Generate 6-digit OTP
	otpCode, err := generateOTP()
	if err != nil {
		return "", err
	}

	if s.rc != nil {
		key := fmt.Sprintf("signup:otp:%s:%d", otpType, customerID)
		if err := s.rc.Set(ctx, key, otpCode, utils.OTPExpiry).Err(); err != nil {
			return "", err
		}
		return otpCode, nil
	}

	return "", ErrCacheNotAvailable
}

func (s *SignupFlowImpl) verifyOTPCode(ctx context.Context, customerID uint, code, otpType string) error {
	if s.rc != nil {
		key := fmt.Sprintf("signup:otp:%s:%d", otpType, customerID)
		val, err := s.rc.Get(ctx, key).Result()
		if err == redis.Nil {
			return ErrNoValidOTPFound
		}
		if err != nil {
			return err
		}
		if val != code {
			return ErrInvalidOTPCode
		}
		// consume OTP
		_ = s.rc.Del(ctx, key).Err()
		return nil
	}

	return ErrCacheNotAvailable
}

func (s *SignupFlowImpl) completeSignup(ctx context.Context, customer *models.Customer, otpType string) error {
	// Update verification status for existing customer (maintain referential integrity)
	var isMobileVerified, isEmailVerified *bool
	var mobileVerifiedAt, emailVerifiedAt *time.Time

	switch otpType {
	case models.OTPTypeMobile:
		isMobileVerified = utils.ToPtr(true)
		mobileVerifiedAt = utils.UTCNowPtr()
	case models.OTPTypeEmail:
		isEmailVerified = utils.ToPtr(true)
		emailVerifiedAt = utils.UTCNowPtr()
	default:
		return ErrInvalidOTPType
	}

	return s.customerRepo.UpdateVerificationStatus(ctx, customer.ID, isMobileVerified, isEmailVerified, mobileVerifiedAt, emailVerifiedAt)
}

func (s *SignupFlowImpl) createSession(ctx context.Context, customerID uint, accessToken, refreshToken string, metadata *ClientMetadata) error {
	ipAddress := ""
	userAgent := ""
	if metadata != nil {
		ipAddress = metadata.IPAddress
		userAgent = metadata.UserAgent
	}

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

	if err := s.sessionRepo.Save(ctx, session); err != nil {
		return err
	}

	return nil
}

func (s *SignupFlowImpl) createDefaultDiscount(ctx context.Context, customer *models.Customer) error {
	rate := 0.0
	if customer.AccountType.TypeName == models.AccountTypeMarketingAgency {
		rate = 0.5
	}

	// create agency discount for customer
	if err := s.agencyDiscountRepo.Save(ctx, &models.AgencyDiscount{
		UUID:         uuid.New(),
		AgencyID:     *customer.ReferrerAgencyID,
		CustomerID:   customer.ID,
		DiscountRate: rate,
		ExpiresAt:    nil,
		Reason:       utils.ToPtr("Created via Signup"),
	}); err != nil {
		return err
	}

	return nil
}

func (s *SignupFlowImpl) createAuditLog(ctx context.Context, customer *models.Customer, action, description string, success bool, errorMsg *string, metadata *ClientMetadata) error {
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
		ErrorMessage: errorMsg,
	}

	// Extract request ID from context if available
	requestID := ctx.Value(utils.RequestIDKey)
	if requestID != nil {
		requestIDStr, ok := requestID.(string)
		if ok {
			audit.RequestID = &requestIDStr
		}
	}

	if err := s.auditRepo.Save(ctx, audit); err != nil {
		return err
	}

	return nil
}

func (s *SignupFlowImpl) maskMobileNumber(mobile string) string {
	if len(mobile) < 8 {
		return mobile
	}
	// Show +989****1234 format
	return mobile[:4] + "****" + mobile[len(mobile)-4:]
}

func (s *SignupFlowImpl) validateOTPVerificationRequest(ctx context.Context, req *dto.OTPVerificationRequest) error {
	// Validate customer exists
	customer, err := getCustomer(ctx, s.customerRepo, req.CustomerID)
	if err != nil {
		return err
	}

	// Validate OTP type
	if req.OTPType != models.OTPTypeMobile && req.OTPType != models.OTPTypeEmail {
		return ErrInvalidOTPType
	}

	// Validate OTP code format (6 digits)
	if len(req.OTPCode) != 6 {
		return ErrInvalidOTPCode
	}

	// Check if customer is already verified for this OTP type
	if req.OTPType == models.OTPTypeMobile && utils.IsTrue(customer.IsMobileVerified) {
		return ErrAlreadyVerified
	}
	if req.OTPType == models.OTPTypeEmail && utils.IsTrue(customer.IsEmailVerified) {
		return ErrAlreadyVerified
	}

	return nil
}

func (s *SignupFlowImpl) validateOTPResendRequest(ctx context.Context, req *dto.OTPResendRequest) error {
	// Validate customer exists
	customer, err := getCustomer(ctx, s.customerRepo, req.CustomerID)
	if err != nil {
		return err
	}

	// Validate OTP type
	if req.OTPType != models.OTPTypeMobile && req.OTPType != models.OTPTypeEmail {
		return ErrInvalidOTPType
	}

	// Check if customer is already verified for this OTP type
	if req.OTPType == models.OTPTypeMobile && utils.IsTrue(customer.IsMobileVerified) {
		return ErrAlreadyVerified
	}
	if req.OTPType == models.OTPTypeEmail && utils.IsTrue(customer.IsEmailVerified) {
		return ErrAlreadyVerified
	}

	return nil
}
