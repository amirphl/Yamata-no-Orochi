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
	ErrMobileNumberNotVerified = errors.New("mobile number not verified")
	ErrEmailAlreadyExists      = errors.New("email already exists")
	ErrMobileAlreadyExists     = errors.New("mobile number already exists")
	ErrNationalIDAlreadyExists = errors.New("national ID already exists")
	ErrAgencyNotFound          = errors.New("agency not found")
	ErrAgencyInactive          = errors.New("agency is inactive")

	// Company/Business account errors
	ErrCompanyFieldsRequired = errors.New("company fields are required for business accounts")
	ErrShebaNumberRequired   = errors.New("sheba number is required for marketing agencies")
	ErrShebaNumberInvalid    = errors.New("sheba number is invalid")

	// Referrer agency errors
	ErrReferrerAgencyNotFound            = errors.New("referrer agency not found")
	ErrReferrerMustBeAgency              = errors.New("referrer must be a marketing agency")
	ErrReferrerAgencyInactive            = errors.New("referrer agency is inactive")
	ErrReferrerAgencyShebaNumberRequired = errors.New("referrer agency sheba number is required")

	// OTP-related errors
	ErrNoValidOTPFound   = errors.New("no valid OTP found")
	ErrInvalidOTPCode    = errors.New("invalid OTP code")
	ErrInvalidOTPType    = errors.New("invalid OTP type")
	ErrOTPExpired        = errors.New("OTP has expired")
	ErrCacheNotAvailable = errors.New("cache not available")

	ErrAlreadyVerified = errors.New("already verified")

	// Campaign-related errors
	ErrCampaignNotFound             = errors.New("campaign not found")
	ErrCampaignAccessDenied         = errors.New("campaign access denied")
	ErrCampaignUpdateNotAllowed     = errors.New("campaign update not allowed")
	ErrInsufficientCampaignCapacity = errors.New("insufficient campaign capacity")
	ErrCampaignTitleRequired        = errors.New("campaign title is required")
	ErrCampaignContentRequired      = errors.New("campaign content is required")
	ErrCampaignSegmentRequired      = errors.New("campaign segment is required")
	ErrCampaignLineNumberRequired   = errors.New("campaign line number is required")
	ErrCampaignLineNumberNotActive  = errors.New("campaign line number is not active")
	ErrCampaignBudgetRequired       = errors.New("campaign budget is required")
	ErrCampaignSexRequired          = errors.New("campaign sex is required")
	ErrCampaignAdLinkRequired       = errors.New("campaign ad link is required")
	ErrScheduleTimeNotPresent       = errors.New("schedule time is not present")
	ErrScheduleTimeTooSoon          = errors.New("schedule time is too soon")
	ErrCampaignCityRequired         = errors.New("campaign city is required")
	ErrCampaignSubsegmentRequired   = errors.New("campaign subsegment is required")
	ErrCampaignTagsRequired         = errors.New("campaign tags is required")
	ErrCampaignUpdateRequired       = errors.New("at least one field must be provided for update")
	ErrCampaignUUIDRequired         = errors.New("campaign UUID is required")

	ErrCampaignNotWaitingForApproval   = errors.New("campaign is not waiting for approval")
	ErrFreezeTransactionNotFound       = errors.New("freeze transaction not found for campaign")
	ErrMultipleFreezeTransactionsFound = errors.New("multiple freeze transactions found for campaign")

	// Payment-related errors
	ErrWalletNotFound           = errors.New("wallet not found")
	ErrAmountTooLow             = errors.New("amount is too low")
	ErrAmountNotMultiple        = errors.New("amount must be a multiple of 10000")
	ErrAtipayTokenEmpty         = errors.New("atipay token is empty")
	ErrInsufficientFunds        = errors.New("insufficient funds")
	ErrReferrerAgencyIDRequired = errors.New("referrer agency ID is required")
	ErrAgencyDiscountNotFound   = errors.New("agency discount not found")

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

	// Tax and system user errors
	ErrSystemUserNotFound            = errors.New("system user not found")
	ErrSystemUserShebaNumberNotFound = errors.New("system user sheba number not found")

	// Tax and System wallet errors
	ErrTaxWalletNotFound                   = errors.New("tax wallet not found")
	ErrTaxWalletBalanceSnapshotNotFound    = errors.New("tax wallet balance snapshot not found")
	ErrSystemWalletNotFound                = errors.New("system wallet not found")
	ErrSystemWalletBalanceSnapshotNotFound = errors.New("system wallet balance snapshot not found")

	// Filter errors
	ErrInvalidPage           = errors.New("page must be at least 1")
	ErrInvalidPageSize       = errors.New("page size must be between 1 and 100")
	ErrStartDateAfterEndDate = errors.New("start date cannot be after end date")

	// Agency discount errors
	ErrDiscountRateOutOfRange              = errors.New("discount rate must be between 0 and 0.5")
	ErrAgencyCannotCreateDiscountForItself = errors.New("agency cannot create discount for itself")
	ErrCustomerNotUnderAgency              = errors.New("customer is not under this agency")
	ErrAgencyCannotListDiscountsForItself  = errors.New("agency cannot list discounts for itself")

	// Admin and captcha related errors
	ErrAdminNotFound  = errors.New("admin not found")
	ErrAdminInactive  = errors.New("admin account is inactive")
	ErrInvalidCaptcha = errors.New("invalid captcha")

	// Bot related errors
	ErrBotNotFound = errors.New("bot not found")
	ErrBotInactive = errors.New("bot account is inactive")

	// Line number related errors
	ErrLineNumberValueRequired = errors.New("line number value is required")
	ErrPriceFactorInvalid      = errors.New("price factor must be greater than zero")
	ErrLineNumberAlreadyExists = errors.New("line number already exists")
	ErrLineNumberNotFound      = errors.New("line number not found")
	ErrLineNumberNotActive     = errors.New("line number is not active")

	// Ticket related errors
	ErrTicketNotFound = errors.New("ticket not found")

	// Short link related errors
	ErrShortLinkNotFound = errors.New("short link not found")
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

func IsMobileNumberNotVerified(err error) bool {
	return errors.Is(err, ErrMobileNumberNotVerified)
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

func IsAgencyNotFound(err error) bool {
	return errors.Is(err, ErrAgencyNotFound)
}

func IsAgencyInactive(err error) bool {
	return errors.Is(err, ErrAgencyInactive)
}

func IsCompanyFieldsRequired(err error) bool {
	return errors.Is(err, ErrCompanyFieldsRequired)
}

func IsShebaNumberRequired(err error) bool {
	return errors.Is(err, ErrShebaNumberRequired)
}

func IsShebaNumberInvalid(err error) bool {
	return errors.Is(err, ErrShebaNumberInvalid)
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

func IsInsufficientCampaignCapacity(err error) bool {
	return errors.Is(err, ErrInsufficientCampaignCapacity)
}

func IsCampaignTitleRequired(err error) bool {
	return errors.Is(err, ErrCampaignTitleRequired)
}

func IsCampaignContentRequired(err error) bool {
	return errors.Is(err, ErrCampaignContentRequired)
}

func IsCampaignSegmentRequired(err error) bool {
	return errors.Is(err, ErrCampaignSegmentRequired)
}

func IsCampaignLineNumberRequired(err error) bool {
	return errors.Is(err, ErrCampaignLineNumberRequired)
}

func IsCampaignLineNumberNotActive(err error) bool {
	return errors.Is(err, ErrCampaignLineNumberNotActive)
}

func IsCampaignBudgetRequired(err error) bool {
	return errors.Is(err, ErrCampaignBudgetRequired)
}

func IsCampaignSexRequired(err error) bool {
	return errors.Is(err, ErrCampaignSexRequired)
}

func IsCampaignAdLinkRequired(err error) bool {
	return errors.Is(err, ErrCampaignAdLinkRequired)
}

func IsScheduleTimeNotPresent(err error) bool {
	return errors.Is(err, ErrScheduleTimeNotPresent)
}

func IsScheduleTimeTooSoon(err error) bool {
	return errors.Is(err, ErrScheduleTimeTooSoon)
}

func IsCampaignCityRequired(err error) bool {
	return errors.Is(err, ErrCampaignCityRequired)
}

func IsCampaignSubsegmentRequired(err error) bool {
	return errors.Is(err, ErrCampaignSubsegmentRequired)
}

func IsCampaignTagsRequired(err error) bool {
	return errors.Is(err, ErrCampaignTagsRequired)
}

func IsCampaignUpdateRequired(err error) bool {
	return errors.Is(err, ErrCampaignUpdateRequired)
}

func IsCampaignUUIDRequired(err error) bool {
	return errors.Is(err, ErrCampaignUUIDRequired)
}

func IsCampaignNotWaitingForApproval(err error) bool {
	return errors.Is(err, ErrCampaignNotWaitingForApproval)
}

func IsFreezeTransactionNotFound(err error) bool {
	return errors.Is(err, ErrFreezeTransactionNotFound)
}

func IsMultipleFreezeTransactionsFound(err error) bool {
	return errors.Is(err, ErrMultipleFreezeTransactionsFound)
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

func IsReferrerAgencyShebaNumberRequired(err error) bool {
	return errors.Is(err, ErrReferrerAgencyShebaNumberRequired)
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

func IsCacheNotAvailable(err error) bool {
	return errors.Is(err, ErrCacheNotAvailable)
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

func IsInsufficientFunds(err error) bool {
	return errors.Is(err, ErrInsufficientFunds)
}

func IsReferrerAgencyIDRequired(err error) bool {
	return errors.Is(err, ErrReferrerAgencyIDRequired)
}

func IsAgencyDiscountNotFound(err error) bool {
	return errors.Is(err, ErrAgencyDiscountNotFound)
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

func IsSystemUserNotFound(err error) bool {
	return errors.Is(err, ErrSystemUserNotFound)
}

func IsSystemUserShebaNumberNotFound(err error) bool {
	return errors.Is(err, ErrSystemUserShebaNumberNotFound)
}

func IsTaxWalletNotFound(err error) bool {
	return errors.Is(err, ErrTaxWalletNotFound)
}

func IsTaxWalletBalanceSnapshotNotFound(err error) bool {
	return errors.Is(err, ErrTaxWalletBalanceSnapshotNotFound)
}

func IsSystemWalletNotFound(err error) bool {
	return errors.Is(err, ErrSystemWalletNotFound)
}

func IsSystemWalletBalanceSnapshotNotFound(err error) bool {
	return errors.Is(err, ErrSystemWalletBalanceSnapshotNotFound)
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

func IsDiscountRateOutOfRange(err error) bool {
	return errors.Is(err, ErrDiscountRateOutOfRange)
}

func IsAgencyCannotCreateDiscountForItself(err error) bool {
	return errors.Is(err, ErrAgencyCannotCreateDiscountForItself)
}

func IsCustomerNotUnderAgency(err error) bool {
	return errors.Is(err, ErrCustomerNotUnderAgency)
}

func IsAgencyCannotListDiscountsForItself(err error) bool {
	return errors.Is(err, ErrAgencyCannotListDiscountsForItself)
}

func IsAdminNotFound(err error) bool {
	return errors.Is(err, ErrAdminNotFound)
}

func IsAdminInactive(err error) bool {
	return errors.Is(err, ErrAdminInactive)
}

func IsInvalidCaptcha(err error) bool {
	return errors.Is(err, ErrInvalidCaptcha)
}

func IsBotNotFound(err error) bool {
	return errors.Is(err, ErrBotNotFound)
}

func IsBotInactive(err error) bool {
	return errors.Is(err, ErrBotInactive)
}

func IsLineNumberValueRequired(err error) bool {
	return errors.Is(err, ErrLineNumberValueRequired)
}

func IsPriceFactorInvalid(err error) bool {
	return errors.Is(err, ErrPriceFactorInvalid)
}

func IsLineNumberAlreadyExists(err error) bool {
	return errors.Is(err, ErrLineNumberAlreadyExists)
}

func IsLineNumberNotFound(err error) bool {
	return errors.Is(err, ErrLineNumberNotFound)
}

func IsLineNumberNotActive(err error) bool {
	return errors.Is(err, ErrLineNumberNotActive)
}

func IsTicketNotFound(err error) bool {
	return errors.Is(err, ErrTicketNotFound)
}

func IsShortLinkNotFound(err error) bool {
	return errors.Is(err, ErrShortLinkNotFound)
}
