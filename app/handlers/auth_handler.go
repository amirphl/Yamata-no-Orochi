// Package handlers contains HTTP request handlers and presentation layer logic for the API endpoints
package handlers

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
)

// AuthHandler handles authentication-related HTTP requests
type AuthHandler struct {
	signupFlow SignupFlow
	loginFlow  LoginFlow
	validator  *validator.Validate
}

// SignupFlow interface for business logic
type SignupFlow interface {
	InitiateSignup(ctx context.Context, req *SignupRequest, ipAddress, userAgent string) (*SignupResponse, error)
	VerifyOTP(ctx context.Context, req *OTPVerificationRequest, ipAddress, userAgent string) (*OTPVerificationResponse, error)
	ResendOTP(ctx context.Context, customerID uint, otpType string, ipAddress, userAgent string) error
}

// LoginFlow interface for business logic
type LoginFlow interface {
	Login(ctx context.Context, req *LoginRequest, ipAddress, userAgent string) (*LoginResult, error)
	ForgotPassword(ctx context.Context, req *ForgotPasswordRequest, ipAddress, userAgent string) (*PasswordResetResult, error)
	ResetPassword(ctx context.Context, req *ResetPasswordRequest, ipAddress, userAgent string) (*LoginResult, error)
}

// DTOs for this handler (avoiding import issues)

type LoginRequest struct {
	Identifier string `json:"identifier" validate:"required,min=3,max=255"`
	Password   string `json:"password" validate:"required,min=8,max=100"`
}

type LoginResult struct {
	Success      bool
	Customer     any
	AccountType  any
	Session      any
	ErrorCode    string
	ErrorMessage string
}

type ForgotPasswordRequest struct {
	Identifier string `json:"identifier" validate:"required,min=3,max=255"`
}

type PasswordResetResult struct {
	Success      bool
	CustomerID   uint
	MaskedPhone  string
	OTPExpiry    time.Time
	ErrorCode    string
	ErrorMessage string
}

type ResetPasswordRequest struct {
	CustomerID      uint   `json:"customer_id" validate:"required"`
	OTPCode         string `json:"otp_code" validate:"required,len=6,numeric"`
	NewPassword     string `json:"new_password" validate:"required,min=8,max=100"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=NewPassword"`
}

type SignupRequest struct {
	AccountType               string  `json:"account_type" validate:"required,oneof=individual independent_company marketing_agency"`
	CompanyName               *string `json:"company_name,omitempty" validate:"omitempty,max=60"`
	NationalID                *string `json:"national_id,omitempty" validate:"omitempty,len=11,numeric"`
	CompanyRegistrationCode   *string `json:"company_registration_code,omitempty" validate:"omitempty,max=20"`
	CompanyEconomicCode       *string `json:"company_economic_code,omitempty" validate:"omitempty,max=20"`
	CompanyPhone              *string `json:"company_phone,omitempty" validate:"omitempty"`
	CompanyFax                *string `json:"company_fax,omitempty" validate:"omitempty,max=20"`
	CompanyAddress            *string `json:"company_address,omitempty" validate:"omitempty,max=255"`
	CompanyPostalCode         *string `json:"company_postal_code,omitempty" validate:"omitempty,len=10,numeric"`
	RepresentativeFirstName   string  `json:"representative_first_name" validate:"required,max=255,alpha_space"`
	RepresentativeLastName    string  `json:"representative_last_name" validate:"required,max=255,alpha_space"`
	RepresentativeMobile      string  `json:"representative_mobile" validate:"required,mobile_format"`
	Email                     string  `json:"email" validate:"required,email,max=100"`
	Password                  string  `json:"password" validate:"required,min=8,max=100,password_strength"`
	ConfirmPassword           string  `json:"confirm_password" validate:"required,eqfield=Password"`
	AgreeToTerms              bool    `json:"agree_to_terms" validate:"required,eq=true"`
	ReferrerAgencyRefererCode *int64  `json:"referrer_agency_referer_code,omitempty" validate:"omitempty"`
}

type SignupResponse struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	CustomerID uint   `json:"customer_id,omitempty"`
}

type OTPVerificationRequest struct {
	CustomerID uint   `json:"customer_id" validate:"required"`
	OTPCode    string `json:"otp_code" validate:"required,len=6,numeric"`
	OTPType    string `json:"otp_type" validate:"required,oneof=mobile email"`
}

type OTPVerificationResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	AccessToken string `json:"access_token,omitempty"`
}

type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Details any    `json:"details,omitempty"`
}

func (h *AuthHandler) ErrorResponse(c fiber.Ctx, statusCode int, message, errorCode string, details any) error {
	return c.Status(statusCode).JSON(APIResponse{
		Success: false,
		Message: message,
		Error: ErrorDetail{
			Code:    errorCode,
			Details: details,
		},
	})
}

func (h *AuthHandler) SuccessResponse(c fiber.Ctx, statusCode int, message string, data any) error {
	return c.Status(statusCode).JSON(APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(signupFlow SignupFlow, loginFlow LoginFlow) *AuthHandler {
	handler := &AuthHandler{
		signupFlow: signupFlow,
		loginFlow:  loginFlow,
		validator:  validator.New(),
	}

	// Setup custom validations
	handler.setupCustomValidations()

	return handler
}

// Signup handles the user registration process
// @Summary User Registration
// @Description Register a new user account
// @Tags authentication
// @Accept json
// @Produce json
// @Param request body SignupRequest true "User registration data"
// @Success 200 {object} SignupResponse "Registration successful"
// @Failure 400 {object} APIResponse "Validation error"
// @Failure 409 {object} APIResponse "User already exists"
// @Failure 500 {object} APIResponse "Internal server error"
// @Router /auth/signup [post]
func (h *AuthHandler) Signup(c fiber.Ctx) error {
	var req SignupRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	// Validate request
	if err := h.validator.Struct(&req); err != nil {
		var validationErrors []string
		for _, err := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, getValidationErrorMessage(err))
		}
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", validationErrors)
	}

	// Get client information
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")

	// Call business logic
	result, err := h.signupFlow.InitiateSignup(context.Background(), &req, ipAddress, userAgent)
	if err != nil {
		if errors.Is(err, errors.New("user already exists")) {
			return h.ErrorResponse(c, fiber.StatusConflict, "User already exists", "USER_EXISTS", nil)
		}
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR", err.Error())
	}

	return h.SuccessResponse(c, fiber.StatusOK, result.Message, fiber.Map{
		"customer_id": result.CustomerID,
	})
}

// VerifyOTP handles OTP verification for signup completion
// @Summary Verify OTP
// @Description Verify OTP to complete registration
// @Tags authentication
// @Accept json
// @Produce json
// @Param request body OTPVerificationRequest true "OTP verification data"
// @Success 200 {object} OTPVerificationResponse "OTP verified successfully"
// @Failure 400 {object} APIResponse "Invalid OTP or request"
// @Failure 500 {object} APIResponse "Internal server error"
// @Router /auth/verify [post]
func (h *AuthHandler) VerifyOTP(c fiber.Ctx) error {
	var req OTPVerificationRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	// Validate request
	if err := h.validator.Struct(&req); err != nil {
		var validationErrors []string
		for _, err := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, getValidationErrorMessage(err))
		}
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", validationErrors)
	}

	// Get client information
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")

	// Call business logic
	result, err := h.signupFlow.VerifyOTP(context.Background(), &req, ipAddress, userAgent)
	if err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "OTP verification failed", "OTP_VERIFICATION_FAILED", err.Error())
	}

	return h.SuccessResponse(c, fiber.StatusOK, result.Message, fiber.Map{
		"access_token": result.AccessToken,
	})
}

// ResendOTP handles OTP resend requests
// @Summary Resend OTP
// @Description Resend OTP for verification
// @Tags authentication
// @Accept json
// @Produce json
// @Param customer_id path uint true "Customer ID"
// @Success 200 {object} APIResponse "OTP resent successfully"
// @Failure 400 {object} APIResponse "Invalid request"
// @Failure 500 {object} APIResponse "Internal server error"
// @Router /auth/resend-otp/{customer_id} [post]
func (h *AuthHandler) ResendOTP(c fiber.Ctx) error {
	customerIDStr := c.Params("customer_id")
	customerID, err := strconv.ParseUint(customerIDStr, 10, 32)
	if err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid customer ID", "INVALID_CUSTOMER_ID", err.Error())
	}

	// For now, we'll resend mobile OTP
	otpType := "mobile"

	// Get client information
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")

	// Call business logic
	err = h.signupFlow.ResendOTP(context.Background(), uint(customerID), otpType, ipAddress, userAgent)
	if err != nil {
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to resend OTP", "RESEND_OTP_FAILED", err.Error())
	}

	return h.SuccessResponse(c, fiber.StatusOK, "OTP resent successfully", nil)
}

// Health endpoint for monitoring
func (h *AuthHandler) Health(c fiber.Ctx) error {
	return h.SuccessResponse(c, fiber.StatusOK, "Auth service is healthy", fiber.Map{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"service":   "auth-handler",
	})
}

// Custom validation setup
func (h *AuthHandler) setupCustomValidations() {
	// Register custom validation for alpha characters with spaces
	h.validator.RegisterValidation("alpha_space", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		for _, char := range value {
			if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == ' ') {
				return false
			}
		}
		return true
	})

	// Register custom validation for mobile format
	h.validator.RegisterValidation("mobile_format", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		// Iranian mobile format: +989xxxxxxxxx
		if len(value) != 14 {
			return false
		}
		if value[:4] != "+989" {
			return false
		}
		// Check if remaining characters are digits
		for _, char := range value[4:] {
			if char < '0' || char > '9' {
				return false
			}
		}
		return true
	})

	// Register custom validation for password strength
	h.validator.RegisterValidation("password_strength", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()

		hasUpper := false
		hasNumber := false

		for _, char := range value {
			if char >= 'A' && char <= 'Z' {
				hasUpper = true
			}
			if char >= '0' && char <= '9' {
				hasNumber = true
			}
		}

		return hasUpper && hasNumber
	})

	h.validator.RegisterValidation("numeric", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		for _, char := range value {
			if char < '0' || char > '9' {
				return false
			}
		}
		return true
	})
}

func getValidationErrorMessage(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return err.Field() + " is required"
	case "email":
		return "Invalid email format"
	case "min":
		return err.Field() + " must be at least " + err.Param() + " characters"
	case "max":
		return err.Field() + " must be at most " + err.Param() + " characters"
	case "len":
		return err.Field() + " must be exactly " + err.Param() + " characters"
	case "oneof":
		return err.Field() + " must be one of: " + err.Param()
	case "eqfield":
		return err.Field() + " must match " + err.Param()
	case "alpha_space":
		return err.Field() + " must contain only letters and spaces"
	case "mobile_format":
		return "Mobile number must be in format +989xxxxxxxxx"
	case "password_strength":
		return "Password must contain at least 1 uppercase letter and 1 number"
	case "numeric":
		return err.Field() + " must contain only numbers"
	default:
		return err.Field() + " is invalid"
	}
}

// Login handles user authentication
// @Summary User Login
// @Description Authenticate user with email/mobile and password
// @Tags authentication
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login credentials"
// @Success 200 {object} APIResponse "Successful login"
// @Failure 400 {object} APIResponse "Invalid request"
// @Failure 401 {object} APIResponse "Authentication failed"
// @Failure 500 {object} APIResponse "Internal server error"
// @Router /auth/login [post]
func (h *AuthHandler) Login(c fiber.Ctx) error {
	var req LoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	// Validate request
	if err := h.validator.Struct(&req); err != nil {
		var validationErrors []string
		for _, err := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, getValidationErrorMessage(err))
		}
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", validationErrors)
	}

	// Get client information
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")

	// Call business logic
	result, err := h.loginFlow.Login(context.Background(), &req, ipAddress, userAgent)
	if err != nil {
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR", err.Error())
	}

	// Handle login result
	if !result.Success {
		statusCode := fiber.StatusUnauthorized
		if result.ErrorCode == "ACCOUNT_INACTIVE" {
			statusCode = fiber.StatusForbidden
		}

		return h.ErrorResponse(c, statusCode, result.ErrorMessage, result.ErrorCode, nil)
	}

	// Successful login
	return h.SuccessResponse(c, fiber.StatusOK, "Login successful", fiber.Map{
		"message": "User authenticated successfully - redirecting to dashboard",
	})
}

// ForgotPassword initiates the password reset process
// @Summary Forgot Password
// @Description Initiate password reset by sending OTP to registered mobile
// @Tags authentication
// @Accept json
// @Produce json
// @Param request body ForgotPasswordRequest true "User identifier"
// @Success 200 {object} APIResponse "OTP sent successfully"
// @Failure 400 {object} APIResponse "Invalid request"
// @Failure 404 {object} APIResponse "User not found"
// @Failure 500 {object} APIResponse "Internal server error"
// @Router /auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(c fiber.Ctx) error {
	var req ForgotPasswordRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	// Validate request
	if err := h.validator.Struct(&req); err != nil {
		var validationErrors []string
		for _, err := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, getValidationErrorMessage(err))
		}
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", validationErrors)
	}

	// Get client information
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")

	// Call business logic
	result, err := h.loginFlow.ForgotPassword(context.Background(), &req, ipAddress, userAgent)
	if err != nil {
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR", err.Error())
	}

	// Handle result
	if !result.Success {
		statusCode := fiber.StatusNotFound
		if result.ErrorCode == "ACCOUNT_INACTIVE" {
			statusCode = fiber.StatusForbidden
		}

		return h.ErrorResponse(c, statusCode, result.ErrorMessage, result.ErrorCode, nil)
	}

	// Successful response
	return h.SuccessResponse(c, fiber.StatusOK, "Password reset OTP sent to your mobile number", fiber.Map{
		"customer_id":  result.CustomerID,
		"masked_phone": result.MaskedPhone,
		"expires_in":   300, // 5 minutes
	})
}

// ResetPassword completes the password reset process
// @Summary Reset Password
// @Description Complete password reset with OTP verification
// @Tags authentication
// @Accept json
// @Produce json
// @Param request body ResetPasswordRequest true "Password reset data"
// @Success 200 {object} APIResponse "Password reset successful"
// @Failure 400 {object} APIResponse "Invalid request or OTP"
// @Failure 500 {object} APIResponse "Internal server error"
// @Router /auth/reset [post]
func (h *AuthHandler) ResetPassword(c fiber.Ctx) error {
	var req ResetPasswordRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	// Validate request
	if err := h.validator.Struct(&req); err != nil {
		var validationErrors []string
		for _, err := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, getValidationErrorMessage(err))
		}
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", validationErrors)
	}

	// Get client information
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")

	// Call business logic
	result, err := h.loginFlow.ResetPassword(context.Background(), &req, ipAddress, userAgent)
	if err != nil {
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR", err.Error())
	}

	// Handle result
	if !result.Success {
		statusCode := fiber.StatusBadRequest
		if result.ErrorCode == "CUSTOMER_NOT_FOUND" {
			statusCode = fiber.StatusNotFound
		}

		return h.ErrorResponse(c, statusCode, result.ErrorMessage, result.ErrorCode, nil)
	}

	// Successful password reset
	return h.SuccessResponse(c, fiber.StatusOK, "New password saved", fiber.Map{
		"password_changed_at": time.Now().Format(time.RFC3339),
	})
}
