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
	ErrCallbackRequestNil             = errors.New("callback request is nil")
	ErrReservationNumberRequired      = errors.New("reservation number is required")
	ErrReferenceNumberRequired        = errors.New("reference number is required")
	ErrStatusRequired                 = errors.New("status is required")
	ErrStateRequired                  = errors.New("state is required")
	ErrPaymentRequestNotFound         = errors.New("payment request not found")
	ErrPaymentRequestAlreadyProcessed = errors.New("payment request already processed")
	ErrPaymentRequestExpired          = errors.New("payment request expired")

	// Balance snapshot errors
	ErrBalanceSnapshotNotFound = errors.New("balance snapshot not found")

	// Tax wallet errors
	ErrTaxWalletNotFound                = errors.New("tax wallet not found")
	ErrTaxWalletBalanceSnapshotNotFound = errors.New("tax wallet balance snapshot not found")

	// Transaction history errors
	ErrInvalidPage           = errors.New("page must be at least 1")
	ErrInvalidPageSize       = errors.New("page size must be between 1 and 100")
	ErrStartDateAfterEndDate = errors.New("start date cannot be after end date")
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

func NewBusinessErrorf(code, message string, err error, args ...any) *BusinessError {
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

func IsWalletNotFound(err error) bool {
	return errors.Is(err, ErrWalletNotFound)
}

func IsAmountTooLow(err error) bool {
	return errors.Is(err, ErrAmountTooLow)
}

func IsAmountNotMultiple(err error) bool {
	return errors.Is(err, ErrAmountNotMultiple)
}

func IsAtipayTokenEmpty(err error) bool {
	return errors.Is(err, ErrAtipayTokenEmpty)
}
func IsCallbackRequestNil(err error) bool {
	return errors.Is(err, ErrCallbackRequestNil)
}

func IsReservationNumberRequired(err error) bool {
	return errors.Is(err, ErrReservationNumberRequired)
}

func IsReferenceNumberRequired(err error) bool {
	return errors.Is(err, ErrReferenceNumberRequired)
}

func IsStatusRequired(err error) bool {
	return errors.Is(err, ErrStatusRequired)
}

func IsStateRequired(err error) bool {
	return errors.Is(err, ErrStateRequired)
}

func IsPaymentRequestNotFound(err error) bool {
	return errors.Is(err, ErrPaymentRequestNotFound)
}

func IsPaymentRequestAlreadyProcessed(err error) bool {
	return errors.Is(err, ErrPaymentRequestAlreadyProcessed)
}

func IsPaymentRequestExpired(err error) bool {
	return errors.Is(err, ErrPaymentRequestExpired)
}

func IsBalanceSnapshotNotFound(err error) bool {
	return errors.Is(err, ErrBalanceSnapshotNotFound)
}

func IsTaxWalletNotFound(err error) bool {
	return errors.Is(err, ErrTaxWalletNotFound)
}

func IsTaxWalletBalanceSnapshotNotFound(err error) bool {
	return errors.Is(err, ErrTaxWalletBalanceSnapshotNotFound)
}

func IsInvalidPage(err error) bool {
	return errors.Is(err, ErrInvalidPage)
}

func IsInvalidPageSize(err error) bool {
	return errors.Is(err, ErrInvalidPageSize)
}

func IsStartDateAfterEndDate(err error) bool {
	return errors.Is(err, ErrStartDateAfterEndDate)
}
