// Package businessflow contains the core business logic and use cases for authentication workflows
package businessflow

import (
	"errors"
	"fmt"
)

// Business flow error constants
var (
	// Customer-related errors
	ErrCustomerNotFound = errors.New("customer not found")
	ErrUserNotFound     = errors.New("user not found")

	// Signup-related errors
	ErrEmailAlreadyExists      = errors.New("email already exists")
	ErrMobileAlreadyExists     = errors.New("mobile number already exists")
	ErrNationalIDAlreadyExists = errors.New("national ID already exists")
	ErrInvalidAccountType      = errors.New("invalid account type")
	ErrSignupFailed            = errors.New("signup failed")

	// Company/Business account errors
	ErrCompanyFieldsRequired = errors.New("company fields are required for business accounts")

	// Referrer agency errors
	ErrReferrerAgencyNotFound   = errors.New("referrer agency not found")
	ErrReferrerMustBeAgency     = errors.New("referrer must be a marketing agency")
	ErrReferrerAgencyInactive   = errors.New("referrer agency is inactive")
	ErrReferrerAgencyValidation = errors.New("failed to validate referrer agency")

	// OTP-related errors
	ErrNoValidOTPFound       = errors.New("no valid OTP found")
	ErrInvalidOTPCode        = errors.New("invalid OTP code")
	ErrInvalidOTPType        = errors.New("invalid OTP type")
	ErrOTPVerificationFailed = errors.New("OTP verification failed")

	// Account type errors
	ErrAccountTypeNotFound = errors.New("account type not found")

	// Session errors
	ErrSessionCreationFailed     = errors.New("failed to create session")
	ErrSessionInvalidationFailed = errors.New("failed to invalidate session")

	// Password reset errors
	ErrPasswordResetFailed  = errors.New("password reset failed")
	ErrPasswordHashFailed   = errors.New("failed to hash password")
	ErrPasswordUpdateFailed = errors.New("failed to update customer password")

	// OTP management errors
	ErrOTPExpirationFailed = errors.New("failed to expire OTP")
	ErrOTPGenerationFailed = errors.New("failed to generate OTP")
	ErrOTPSaveFailed       = errors.New("failed to save OTP")
	ErrOTPMarkUsedFailed   = errors.New("failed to mark OTP as used")

	// Login errors
	ErrLoginFailed        = errors.New("login failed")
	ErrLoginLoggingFailed = errors.New("failed to log login attempt")

	// Password reset logging errors
	ErrPasswordResetLoggingFailed = errors.New("failed to log password reset attempt")

	// Search errors
	ErrEmailSearchFailed  = errors.New("failed to search by email")
	ErrMobileSearchFailed = errors.New("failed to search by mobile")

	// Token errors
	ErrTokenGenerationFailed = errors.New("failed to generate tokens")
)

type BusinessError struct {
	Code    string
	Message string
	Err     error
}

func (e *BusinessError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *BusinessError) Unwrap() error {
	return e.Err
}

func NewBusinessError(code, message string, err error) *BusinessError {
	return &BusinessError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

func NewBusinessErrorf(code, message string, err error, args ...interface{}) *BusinessError {
	return &BusinessError{
		Code:    code,
		Message: fmt.Sprintf(message, args...),
		Err:     err,
	}
}

func IsCustomerNotFound(err error) bool {
	return errors.Is(err, ErrCustomerNotFound)
}

func IsUserNotFound(err error) bool {
	return errors.Is(err, ErrUserNotFound)
}

func IsEmailAlreadyExists(err error) bool {
	return errors.Is(err, ErrEmailAlreadyExists)
}

func IsMobileAlreadyExists(err error) bool {
	return errors.Is(err, ErrMobileAlreadyExists)
}

func IsNationalIDAlreadyExists(err error) bool {
	return errors.Is(err, ErrNationalIDAlreadyExists)
}

func IsInvalidAccountType(err error) bool {
	return errors.Is(err, ErrInvalidAccountType)
}

func IsCompanyFieldsRequired(err error) bool {
	return errors.Is(err, ErrCompanyFieldsRequired)
}

func IsReferrerAgencyNotFound(err error) bool {
	return errors.Is(err, ErrReferrerAgencyNotFound)
}

func IsReferrerMustBeAgency(err error) bool {
	return errors.Is(err, ErrReferrerMustBeAgency)
}

func IsReferrerAgencyInactive(err error) bool {
	return errors.Is(err, ErrReferrerAgencyInactive)
}

func IsNoValidOTPFound(err error) bool {
	return errors.Is(err, ErrNoValidOTPFound)
}

func IsInvalidOTPCode(err error) bool {
	return errors.Is(err, ErrInvalidOTPCode)
}

func IsInvalidOTPType(err error) bool {
	return errors.Is(err, ErrInvalidOTPType)
}

func IsOTPVerificationFailed(err error) bool {
	return errors.Is(err, ErrOTPVerificationFailed)
}

func IsAccountTypeNotFound(err error) bool {
	return errors.Is(err, ErrAccountTypeNotFound)
}

func IsSessionCreationFailed(err error) bool {
	return errors.Is(err, ErrSessionCreationFailed)
}

func IsPasswordResetFailed(err error) bool {
	return errors.Is(err, ErrPasswordResetFailed)
}

func IsLoginFailed(err error) bool {
	return errors.Is(err, ErrLoginFailed)
}

func IsTokenGenerationFailed(err error) bool {
	return errors.Is(err, ErrTokenGenerationFailed)
}

func IsLoginLoggingFailed(err error) bool {
	return errors.Is(err, ErrLoginLoggingFailed)
}

func IsPasswordResetLoggingFailed(err error) bool {
	return errors.Is(err, ErrPasswordResetLoggingFailed)
}

func IsPasswordHashFailed(err error) bool {
	return errors.Is(err, ErrPasswordHashFailed)
}

func IsPasswordUpdateFailed(err error) bool {
	return errors.Is(err, ErrPasswordUpdateFailed)
}

func IsOTPExpirationFailed(err error) bool {
	return errors.Is(err, ErrOTPExpirationFailed)
}

func IsOTPGenerationFailed(err error) bool {
	return errors.Is(err, ErrOTPGenerationFailed)
}

func IsOTPSaveFailed(err error) bool {
	return errors.Is(err, ErrOTPSaveFailed)
}

func IsOTPMarkUsedFailed(err error) bool {
	return errors.Is(err, ErrOTPMarkUsedFailed)
}

func IsSessionInvalidationFailed(err error) bool {
	return errors.Is(err, ErrSessionInvalidationFailed)
}

func IsEmailSearchFailed(err error) bool {
	return errors.Is(err, ErrEmailSearchFailed)
}

func IsMobileSearchFailed(err error) bool {
	return errors.Is(err, ErrMobileSearchFailed)
}
