// Package handlers contains HTTP request handlers and presentation layer logic for the API endpoints
package handlers

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
)

// AuthHandlerInterface defines the contract for authentication handlers
type AuthHandlerInterface interface {
	Signup(c fiber.Ctx) error
	VerifyOTP(c fiber.Ctx) error
	ResendOTP(c fiber.Ctx) error
	Login(c fiber.Ctx) error
	ForgotPassword(c fiber.Ctx) error
	ResetPassword(c fiber.Ctx) error
}

// AuthHandler handles authentication-related HTTP requests
type AuthHandler struct {
	signupFlow businessflow.SignupFlow
	loginFlow  businessflow.LoginFlow
	validator  *validator.Validate
}

// APIResponse represents the standard API response structure
type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Error   any    `json:"error,omitempty"`
}

// ErrorDetail represents error details in API responses
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
func NewAuthHandler(signupFlow businessflow.SignupFlow, loginFlow businessflow.LoginFlow) *AuthHandler {
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
// @Description Register a new user account with email verification
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body dto.SignupRequest true "User registration data"
// @Success 200 {object} dto.APIResponse{data=dto.SignupResponse} "Registration initiated successfully"
// @Failure 400 {object} dto.APIResponse "Validation error"
// @Failure 409 {object} dto.APIResponse "User already exists"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/auth/signup [post]
func (h *AuthHandler) Signup(c fiber.Ctx) error {
	var req dto.SignupRequest
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
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business logic
	result, err := h.signupFlow.InitiateSignup(context.Background(), &req, metadata)
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

// VerifyOTP handles OTP verification for user registration
// @Summary Verify OTP
// @Description Verify OTP code sent to user's mobile number
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body dto.OTPVerificationRequest true "OTP verification data"
// @Success 200 {object} dto.APIResponse{data=dto.OTPVerificationResponse} "OTP verified successfully"
// @Failure 400 {object} dto.APIResponse "Invalid OTP or request"
// @Failure 404 {object} dto.APIResponse "User not found"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/auth/verify [post]
func (h *AuthHandler) VerifyOTP(c fiber.Ctx) error {
	var req dto.OTPVerificationRequest
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
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business logic
	result, err := h.signupFlow.VerifyOTP(context.Background(), &req, metadata)
	if err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "OTP verification failed", "OTP_VERIFICATION_FAILED", err.Error())
	}

	return h.SuccessResponse(c, fiber.StatusOK, result.Message, fiber.Map{
		"token":         result.Token,
		"refresh_token": result.RefreshToken,
		"customer":      result.Customer,
	})
}

// ResendOTP handles resending OTP to user's mobile number
// @Summary Resend OTP
// @Description Resend OTP code to user's mobile number
// @Tags Authentication
// @Accept json
// @Produce json
// @Param customer_id path string true "Customer ID"
// @Success 200 {object} dto.APIResponse "OTP resent successfully"
// @Failure 400 {object} dto.APIResponse "Invalid request"
// @Failure 404 {object} dto.APIResponse "User not found"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/auth/resend-otp/{customer_id} [post]
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
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business logic
	err = h.signupFlow.ResendOTP(context.Background(), uint(customerID), otpType, metadata)
	if err != nil {
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to resend OTP", "RESEND_OTP_FAILED", err.Error())
	}

	return h.SuccessResponse(c, fiber.StatusOK, "OTP resent successfully", nil)
}

// Health handles health check requests
// @Summary Health Check
// @Description Check the health status of the API
// @Tags Health
// @Accept json
// @Produce json
// @Success 200 {object} dto.APIResponse "Service is healthy"
// @Router /api/v1/health [get]
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
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body dto.LoginRequest true "Login credentials"
// @Success 200 {object} dto.APIResponse{data=dto.LoginResponse} "Login successful"
// @Failure 400 {object} dto.APIResponse "Invalid credentials"
// @Failure 401 {object} dto.APIResponse "Authentication failed"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/auth/login [post]
func (h *AuthHandler) Login(c fiber.Ctx) error {
	var req dto.LoginRequest
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
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business logic
	result, err := h.loginFlow.Login(context.Background(), &req, metadata)
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

// ForgotPassword handles password reset initiation
// @Summary Forgot Password
// @Description Initiate password reset by sending OTP to registered mobile
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body dto.ForgotPasswordRequest true "Password reset request"
// @Success 200 {object} dto.APIResponse{data=dto.ForgotPasswordResponse} "OTP sent successfully"
// @Failure 400 {object} dto.APIResponse "Invalid request"
// @Failure 404 {object} dto.APIResponse "User not found"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(c fiber.Ctx) error {
	var req dto.ForgotPasswordRequest
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
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business logic
	result, err := h.loginFlow.ForgotPassword(context.Background(), &req, metadata)
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

// ResetPassword handles password reset completion
// @Summary Reset Password
// @Description Complete password reset with OTP verification
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body dto.ResetPasswordRequest true "Password reset data"
// @Success 200 {object} dto.APIResponse{data=dto.ResetPasswordResponse} "Password reset successful"
// @Failure 400 {object} dto.APIResponse "Invalid request or OTP"
// @Failure 404 {object} dto.APIResponse "User not found"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/auth/reset [post]
func (h *AuthHandler) ResetPassword(c fiber.Ctx) error {
	var req dto.ResetPasswordRequest
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
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business logic
	result, err := h.loginFlow.ResetPassword(context.Background(), &req, metadata)
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
