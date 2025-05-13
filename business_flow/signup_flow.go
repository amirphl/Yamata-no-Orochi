// Package businessflow contains the core business logic and use cases for authentication workflows
package businessflow

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/app/services"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
)

// SignupFlow handles the complete signup business logic
type SignupFlow interface {
	InitiateSignup(ctx context.Context, req *dto.SignupRequest) (*dto.SignupResponse, error)
	VerifyOTP(ctx context.Context, req *dto.OTPVerificationRequest) (*dto.OTPVerificationResponse, error)
	ResendOTP(ctx context.Context, customerID uint, otpType string) error
}

// SignupFlowImpl implements the signup business flow
type SignupFlowImpl struct {
	customerRepo    repository.CustomerRepository
	accountTypeRepo repository.AccountTypeRepository
	otpRepo         repository.OTPVerificationRepository
	sessionRepo     repository.CustomerSessionRepository
	auditRepo       repository.AuditLogRepository
	tokenService    services.TokenService
	notificationSvc services.NotificationService
}

// NewSignupFlow creates a new signup flow instance
func NewSignupFlow(
	customerRepo repository.CustomerRepository,
	accountTypeRepo repository.AccountTypeRepository,
	otpRepo repository.OTPVerificationRepository,
	sessionRepo repository.CustomerSessionRepository,
	auditRepo repository.AuditLogRepository,
	tokenService services.TokenService,
	notificationSvc services.NotificationService,
) SignupFlow {
	return &SignupFlowImpl{
		customerRepo:    customerRepo,
		accountTypeRepo: accountTypeRepo,
		otpRepo:         otpRepo,
		sessionRepo:     sessionRepo,
		auditRepo:       auditRepo,
		tokenService:    tokenService,
		notificationSvc: notificationSvc,
	}
}

// InitiateSignup handles the complete signup process
func (s *SignupFlowImpl) InitiateSignup(ctx context.Context, req *dto.SignupRequest) (*dto.SignupResponse, error) {
	// Validate business rules
	if err := s.validateSignupRequest(ctx, req); err != nil {
		return nil, err
	}

	// Use transaction for atomicity
	var customer *models.Customer
	var otpCode string

	err := repository.WithTransaction(ctx, s.customerRepo.(*repository.CustomerRepositoryImpl).BaseRepository.DB, func(txCtx context.Context) error {
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

		// Create audit log
		return s.createAuditLog(txCtx, &customer.ID, models.AuditActionSignupInitiated, "Signup initiated successfully", true, nil)
	})

	if err != nil {
		// Log failed signup attempt
		errMsg := err.Error()
		s.createAuditLog(ctx, nil, models.AuditActionSignupInitiated, "Signup initiation failed", false, &errMsg)
		return nil, fmt.Errorf("signup failed: %w", err)
	}

	// Send OTP via SMS (outside transaction to avoid rollback on SMS failure)
	go func() {
		message := fmt.Sprintf("Your verification code is: %s. Valid for 5 minutes.", otpCode)
		if err := s.notificationSvc.SendSMS(customer.RepresentativeMobile, message); err != nil {
			// Log SMS failure but don't fail the signup
			errMsg := err.Error()
			s.createAuditLog(context.Background(), &customer.ID, models.AuditActionOTPGenerated, "SMS sending failed", false, &errMsg)
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
func (s *SignupFlowImpl) VerifyOTP(ctx context.Context, req *dto.OTPVerificationRequest) (*dto.OTPVerificationResponse, error) {
	var customer *models.Customer
	var tokens struct {
		access  string
		refresh string
	}

	err := repository.WithTransaction(ctx, s.customerRepo.(*repository.CustomerRepositoryImpl).BaseRepository.DB, func(txCtx context.Context) error {
		// Find customer
		var err error
		customer, err = s.customerRepo.ByID(txCtx, req.CustomerID)
		if err != nil {
			return err
		}
		if customer == nil {
			return errors.New("customer not found")
		}

		// Verify OTP
		if err := s.verifyOTPCode(txCtx, req.CustomerID, req.OTPCode, req.OTPType); err != nil {
			return err
		}

		// Mark customer as verified and complete signup
		if err := s.completeSignup(txCtx, customer, req.OTPType); err != nil {
			return err
		}

		// Generate tokens
		tokens.access, tokens.refresh, err = s.tokenService.GenerateTokens(customer.ID)
		if err != nil {
			return err
		}

		// Create session
		if err := s.createSession(txCtx, customer.ID, tokens.access, tokens.refresh); err != nil {
			return err
		}

		// Create audit logs
		if err := s.createAuditLog(txCtx, &customer.ID, models.AuditActionOTPVerified, "OTP verified successfully", true, nil); err != nil {
			return err
		}

		return s.createAuditLog(txCtx, &customer.ID, models.AuditActionSignupCompleted, "Signup completed successfully", true, nil)
	})

	if err != nil {
		errMsg := err.Error()
		s.createAuditLog(ctx, &req.CustomerID, models.AuditActionOTPFailed, "OTP verification failed", false, &errMsg)
		return nil, fmt.Errorf("OTP verification failed: %w", err)
	}

	return &dto.OTPVerificationResponse{
		Message:      "Signup completed successfully!",
		Token:        tokens.access,
		RefreshToken: tokens.refresh,
		Customer:     s.customerToDTO(customer),
	}, nil
}

// ResendOTP generates and sends a new OTP
func (s *SignupFlowImpl) ResendOTP(ctx context.Context, customerID uint, otpType string) error {
	customer, err := s.customerRepo.ByID(ctx, customerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return errors.New("customer not found")
	}

	// Expire old OTPs
	if err := s.otpRepo.ExpireOldOTPs(ctx, customerID, otpType); err != nil {
		return err
	}

	// Generate new OTP
	target := customer.RepresentativeMobile
	if otpType == models.OTPTypeEmail {
		target = customer.Email
	}

	otpCode, err := s.generateAndSaveOTP(ctx, customerID, target, otpType)
	if err != nil {
		return err
	}

	// Send notification
	message := fmt.Sprintf("Your new verification code is: %s. Valid for 5 minutes.", otpCode)
	if otpType == models.OTPTypeMobile {
		return s.notificationSvc.SendSMS(target, message)
	} else {
		return s.notificationSvc.SendEmail(target, "Verification Code", message)
	}
}

// Private helper methods

func (s *SignupFlowImpl) validateSignupRequest(ctx context.Context, req *dto.SignupRequest) error {
	// Check if email already exists
	existingCustomer, err := s.customerRepo.ByEmail(ctx, req.Email)
	if err != nil {
		return err
	}
	if existingCustomer != nil {
		return errors.New("email already exists")
	}

	// Check if mobile already exists
	existingCustomer, err = s.customerRepo.ByMobile(ctx, req.RepresentativeMobile)
	if err != nil {
		return err
	}
	if existingCustomer != nil {
		return errors.New("mobile number already exists")
	}

	// Validate company fields for business accounts
	if req.AccountType == models.AccountTypeIndependentCompany || req.AccountType == models.AccountTypeMarketingAgency {
		if req.CompanyName == nil || req.NationalID == nil || req.CompanyPhone == nil ||
			req.CompanyAddress == nil || req.PostalCode == nil {
			return errors.New("company fields are required for business accounts")
		}

		// Check if national ID already exists
		if existingCustomer, err := s.customerRepo.ByNationalID(ctx, *req.NationalID); err != nil {
			return err
		} else if existingCustomer != nil {
			return errors.New("national ID already exists")
		}
	}

	// Check if agency exists (for marketing_agency account type)
	if req.ReferrerAgencyCode != nil {
		agencyFilter := models.CustomerFilter{
			AgencyRefererCode: req.ReferrerAgencyCode,
			IsActive:          utils.ToPtr(true),
		}
		agencies, err := s.customerRepo.ByFilter(ctx, agencyFilter, "", 0, 0)
		if err != nil {
			return fmt.Errorf("failed to validate referrer agency: %w", err)
		}
		if len(agencies) == 0 {
			return fmt.Errorf("referrer agency not found: %d", *req.ReferrerAgencyCode)
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
		return nil, errors.New("invalid account type")
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Find referrer agency if provided
	var referrerAgencyID *uint
	if req.ReferrerAgencyCode != nil {
		// Find agency by agency_referer_code
		referrerAgency, err := s.customerRepo.ByAgencyRefererCode(ctx, *req.ReferrerAgencyCode)
		if err != nil {
			return nil, fmt.Errorf("failed to find referrer agency: %w", err)
		}
		if referrerAgency == nil {
			return nil, errors.New("referrer agency not found")
		}
		if referrerAgency.AccountType.TypeName != models.AccountTypeMarketingAgency {
			return nil, errors.New("referrer must be a marketing agency")
		}
		if !utils.IsTrue(referrerAgency.IsActive) {
			return nil, errors.New("referrer agency is inactive")
		}
		referrerAgencyID = &referrerAgency.ID
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
		RepresentativeFirstName: req.RepresentativeFirstName,
		RepresentativeLastName:  req.RepresentativeLastName,
		RepresentativeMobile:    req.RepresentativeMobile,
		Email:                   req.Email,
		PasswordHash:            string(hashedPassword),
		ReferrerAgencyID:        referrerAgencyID,
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
	otpCode, err := s.generateOTPCode()
	if err != nil {
		return "", err
	}

	// Create OTP record
	otp := &models.OTPVerification{
		CustomerID:    customerID,
		OTPCode:       otpCode,
		OTPType:       otpType,
		TargetValue:   target,
		Status:        models.OTPStatusPending,
		AttemptsCount: 0,
		MaxAttempts:   3,
		ExpiresAt:     time.Now().Add(5 * time.Minute),
		CorrelationID: uuid.New(), // Generate new UUID for correlation ID
	}

	err = s.otpRepo.Save(ctx, otp)
	if err != nil {
		return "", err
	}

	return otpCode, nil
}

func (s *SignupFlowImpl) generateOTPCode() (string, error) {
	// Generate a secure 6-digit number
	max := big.NewInt(999999)
	min := big.NewInt(100000)

	n, err := rand.Int(rand.Reader, new(big.Int).Sub(max, min))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%06d", new(big.Int).Add(n, min).Int64()), nil
}

func (s *SignupFlowImpl) verifyOTPCode(ctx context.Context, customerID uint, code, otpType string) error {
	// Find active OTP
	otps, err := s.otpRepo.ListActiveOTPs(ctx, customerID)
	if err != nil {
		return err
	}

	var validOTP *models.OTPVerification
	for _, otp := range otps {
		if otp.OTPType == otpType && otp.Status == models.OTPStatusPending && otp.CanAttempt() {
			validOTP = otp
			break
		}
	}

	if validOTP == nil {
		return errors.New("no valid OTP found")
	}

	if validOTP.OTPCode != code {
		// Create failed attempt record with correlation ID
		failedOTP := *validOTP
		failedOTP.ID = 0
		failedOTP.CorrelationID = validOTP.CorrelationID // Use same correlation ID
		failedOTP.AttemptsCount++
		failedOTP.Status = models.OTPStatusFailed
		s.otpRepo.Save(ctx, &failedOTP)

		return errors.New("invalid OTP code")
	}

	// Create verified OTP record with correlation ID
	verifiedOTP := *validOTP
	verifiedOTP.ID = 0
	verifiedOTP.CorrelationID = validOTP.CorrelationID // Use same correlation ID
	verifiedOTP.Status = models.OTPStatusVerified
	verifiedOTP.VerifiedAt = utils.ToPtr(time.Now())

	return s.otpRepo.Save(ctx, &verifiedOTP)
}

func (s *SignupFlowImpl) completeSignup(ctx context.Context, customer *models.Customer, otpType string) error {
	// Update verification status for existing customer (maintain referential integrity)
	var isMobileVerified, isEmailVerified *bool
	var mobileVerifiedAt, emailVerifiedAt *time.Time

	switch otpType {
	case models.OTPTypeMobile:
		isMobileVerified = utils.ToPtr(true)
		mobileVerifiedAt = utils.ToPtr(time.Now())
	case models.OTPTypeEmail:
		isEmailVerified = utils.ToPtr(true)
		emailVerifiedAt = utils.ToPtr(time.Now())
	default:
		return errors.New("invalid OTP type")
	}

	return s.customerRepo.UpdateVerificationStatus(ctx, customer.ID, isMobileVerified, isEmailVerified, mobileVerifiedAt, emailVerifiedAt)
}

func (s *SignupFlowImpl) createSession(ctx context.Context, customerID uint, accessToken, refreshToken string) error {
	session := &models.CustomerSession{
		CorrelationID: uuid.New(),
		CustomerID:    customerID,
		SessionToken:  accessToken,
		RefreshToken:  &refreshToken,
		IsActive:      utils.ToPtr(true),
		ExpiresAt:     time.Now().Add(24 * time.Hour),
	}

	return s.sessionRepo.Save(ctx, session)
}

func (s *SignupFlowImpl) createAuditLog(ctx context.Context, customerID *uint, action, description string, success bool, errorMsg *string) error {
	audit := &models.AuditLog{
		CustomerID:   customerID,
		Action:       action,
		Description:  &description,
		Success:      utils.ToPtr(success),
		ErrorMessage: errorMsg,
	}

	return s.auditRepo.Save(ctx, audit)
}

func (s *SignupFlowImpl) maskMobileNumber(mobile string) string {
	if len(mobile) < 8 {
		return mobile
	}
	// Show +989****1234 format
	return mobile[:4] + "****" + mobile[len(mobile)-4:]
}

func (s *SignupFlowImpl) customerToDTO(customer *models.Customer) dto.CustomerDTO {
	return dto.CustomerDTO{
		ID:                      customer.ID,
		UUID:                    customer.UUID.String(),
		AccountType:             customer.AccountType.TypeName,
		CompanyName:             customer.CompanyName,
		NationalID:              customer.NationalID,
		CompanyPhone:            customer.CompanyPhone,
		CompanyAddress:          customer.CompanyAddress,
		PostalCode:              customer.PostalCode,
		RepresentativeFirstName: customer.RepresentativeFirstName,
		RepresentativeLastName:  customer.RepresentativeLastName,
		RepresentativeMobile:    customer.RepresentativeMobile,
		Email:                   customer.Email,
		IsEmailVerified:         customer.IsEmailVerified,
		IsMobileVerified:        customer.IsMobileVerified,
		IsActive:                customer.IsActive,
		CreatedAt:               customer.CreatedAt,
		ReferrerAgencyID:        customer.ReferrerAgencyID,
	}
}
