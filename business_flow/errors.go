// Package businessflow contains the core business logic and use cases for authentication workflows
package businessflow

import (
	"errors"
	"fmt"
)

// Business flow error constants
var (
	// Customer-related errors
	ErrCustomerNotFound        = errors.New("customer not found")
	ErrAccountInactive         = errors.New("account is inactive")
	ErrAccountTypeNotFound     = errors.New("account type not found")
	ErrIncorrectPassword       = errors.New("incorrect password")
	ErrEmailAlreadyExists      = errors.New("email already exists")
	ErrMobileAlreadyExists     = errors.New("mobile number already exists")
	ErrNationalIDAlreadyExists = errors.New("national ID already exists")

	// Company/Business account errors
	ErrCompanyFieldsRequired = errors.New("company fields are required for business accounts")

	// Referrer agency errors
	ErrReferrerAgencyNotFound = errors.New("referrer agency not found")
	ErrReferrerMustBeAgency   = errors.New("referrer must be a marketing agency")
	ErrReferrerAgencyInactive = errors.New("referrer agency is inactive")

	// OTP-related errors
	ErrNoValidOTPFound = errors.New("no valid OTP found")
	ErrInvalidOTPCode  = errors.New("invalid OTP code")
	ErrInvalidOTPType  = errors.New("invalid OTP type")
	ErrOTPExpired      = errors.New("OTP has expired")

	ErrAlreadyVerified = errors.New("already verified")

	// SMS Campaign-related errors
	ErrCampaignNotFound             = errors.New("campaign not found")
	ErrCampaignAccessDenied         = errors.New("campaign access denied")
	ErrCampaignUpdateNotAllowed     = errors.New("campaign update not allowed")
	ErrInsufficientCampaignCapacity = errors.New("insufficient campaign capacity")

	// Payment-related errors
	ErrWalletNotFound    = errors.New("wallet not found")
	ErrAmountTooLow      = errors.New("amount is too low")
	ErrAmountNotMultiple = errors.New("amount must be a multiple of 10000")
	ErrAtipayTokenEmpty  = errors.New("atipay token is empty")

	// Payment callback errors
	ErrCallbackRequestNil        = errors.New("callback request is nil")
	ErrReservationNumberRequired = errors.New("reservation number is required")
	ErrReferenceNumberRequired   = errors.New("reference number is required")
	ErrStatusRequired            = errors.New("status is required")
	ErrStateRequired             = errors.New("state is required")
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

func IsAccountInactive(err error) bool {
	return errors.Is(err, ErrAccountInactive)
}

func IsAccountTypeNotFound(err error) bool {
	return errors.Is(err, ErrAccountTypeNotFound)
}

func IsIncorrectPassword(err error) bool {
	return errors.Is(err, ErrIncorrectPassword)
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

func IsCompanyFieldsRequired(err error) bool {
	return errors.Is(err, ErrCompanyFieldsRequired)
}

func IsCampaignNotFound(err error) bool {
	return errors.Is(err, ErrCampaignNotFound)
}

func IsCampaignAccessDenied(err error) bool {
	return errors.Is(err, ErrCampaignAccessDenied)
}

func IsCampaignUpdateNotAllowed(err error) bool {
	return errors.Is(err, ErrCampaignUpdateNotAllowed)
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

func IsOTPExpired(err error) bool {
	return errors.Is(err, ErrOTPExpired)
}

func IsAlreadyVerified(err error) bool {
	return errors.Is(err, ErrAlreadyVerified)
}
