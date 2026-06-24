package businessflow

import (
	"errors"
	"strings"
	"testing"
)

func TestBusinessErrorError(t *testing.T) {
	t.Parallel()

	t.Run("with wrapped error", func(t *testing.T) {
		inner := errors.New("inner")
		be := &BusinessError{Code: "C1", Message: "outer", Err: inner}
		got := be.Error()
		if !strings.Contains(got, "outer") {
			t.Fatalf("expected message in Error(), got: %q", got)
		}
		if !strings.Contains(got, "inner") {
			t.Fatalf("expected inner error in Error(), got: %q", got)
		}
	})

	t.Run("without wrapped error", func(t *testing.T) {
		be := &BusinessError{Code: "C2", Message: "standalone", Err: nil}
		if be.Error() != "standalone" {
			t.Fatalf("expected %q, got %q", "standalone", be.Error())
		}
	})
}

func TestBusinessErrorUnwrap(t *testing.T) {
	t.Parallel()

	inner := errors.New("inner")
	be := &BusinessError{Err: inner}
	if !errors.Is(be, inner) {
		t.Fatal("Unwrap should expose the inner error to errors.Is")
	}
}

func TestNewBusinessError(t *testing.T) {
	t.Parallel()

	inner := errors.New("cause")
	be := NewBusinessError("CODE", "message", inner)
	if be.Code != "CODE" {
		t.Fatalf("expected code %q, got %q", "CODE", be.Code)
	}
	if be.Message != "message" {
		t.Fatalf("expected message %q, got %q", "message", be.Message)
	}
	if !errors.Is(be, inner) {
		t.Fatal("expected inner error to be wrapped")
	}
}

func TestNewBusinessErrorf(t *testing.T) {
	t.Parallel()

	inner := errors.New("cause")
	be := NewBusinessErrorf("CODE", "message for %s", inner, "customer")
	if !strings.Contains(be.Message, "customer") {
		t.Fatalf("expected formatted message, got %q", be.Message)
	}
}

func TestErrorSentinels(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		sentinel  error
		predicate func(error) bool
	}{
		{"CustomerNotFound", ErrCustomerNotFound, IsCustomerNotFound},
		{"AccountInactive", ErrAccountInactive, IsAccountInactive},
		{"AccountTypeNotFound", ErrAccountTypeNotFound, IsAccountTypeNotFound},
		{"IncorrectPassword", ErrIncorrectPassword, IsIncorrectPassword},
		{"MobileNumberNotVerified", ErrMobileNumberNotVerified, IsMobileNumberNotVerified},
		{"EmailAlreadyExists", ErrEmailAlreadyExists, IsEmailAlreadyExists},
		{"MobileAlreadyExists", ErrMobileAlreadyExists, IsMobileAlreadyExists},
		{"NationalIDAlreadyExists", ErrNationalIDAlreadyExists, IsNationalIDAlreadyExists},
		{"NationalIDRequired", ErrNationalIDRequired, IsNationalIDRequired},
		{"AgencyNotFound", ErrAgencyNotFound, IsAgencyNotFound},
		{"AgencyInactive", ErrAgencyInactive, IsAgencyInactive},
		{"NoValidOTPFound", ErrNoValidOTPFound, IsNoValidOTPFound},
		{"InvalidOTPCode", ErrInvalidOTPCode, IsInvalidOTPCode},
		{"OTPExpired", ErrOTPExpired, IsOTPExpired},
		{"CacheNotAvailable", ErrCacheNotAvailable, IsCacheNotAvailable},
		{"AuthenticationFailed", ErrAuthenticationFailed, IsAuthenticationFailed},
		{"RateLimitExceeded", ErrRateLimitExceeded, IsRateLimitExceeded},
		{"CampaignNotFound", ErrCampaignNotFound, IsCampaignNotFound},
		{"CampaignAccessDenied", ErrCampaignAccessDenied, IsCampaignAccessDenied},
		{"InsufficientFunds", ErrInsufficientFunds, IsInsufficientFunds},
		{"WalletNotFound", ErrWalletNotFound, IsWalletNotFound},
		{"AmountTooLow", ErrAmountTooLow, IsAmountTooLow},
		{"AmountNotMultiple", ErrAmountNotMultiple, IsAmountNotMultiple},
		{"PaymentRequestNotFound", ErrPaymentRequestNotFound, IsPaymentRequestNotFound},
		{"TransactionNotFound", ErrTransactionNotFound, IsTransactionNotFound},
		{"InvalidPage", ErrInvalidPage, IsInvalidPage},
		{"InvalidPageSize", ErrInvalidPageSize, IsInvalidPageSize},
		{"StartDateAfterEndDate", ErrStartDateAfterEndDate, IsStartDateAfterEndDate},
		{"DiscountRateOutOfRange", ErrDiscountRateOutOfRange, IsDiscountRateOutOfRange},
		{"AdminNotFound", ErrAdminNotFound, IsAdminNotFound},
		{"AdminInactive", ErrAdminInactive, IsAdminInactive},
		{"BotNotFound", ErrBotNotFound, IsBotNotFound},
		{"BotInactive", ErrBotInactive, IsBotInactive},
		{"LineNumberNotFound", ErrLineNumberNotFound, IsLineNumberNotFound},
		{"SegmentPriceFactorNotFound", ErrSegmentPriceFactorNotFound, IsSegmentPriceFactorNotFound},
		{"ShortLinkNotFound", ErrShortLinkNotFound, IsShortLinkNotFound},
		{"CryptoRequestNotFound", ErrCryptoRequestNotFound, IsCryptoRequestNotFound},
		{"DepositReceiptNotFound", ErrDepositReceiptNotFound, IsDepositReceiptNotFound},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if !tc.predicate(tc.sentinel) {
				t.Errorf("Is%s(Err%s) should return true", tc.name, tc.name)
			}
			if tc.predicate(errors.New("unrelated")) {
				t.Errorf("Is%s should return false for an unrelated error", tc.name)
			}
			wrapped := errors.Join(errors.New("wrapper"), tc.sentinel)
			if !tc.predicate(wrapped) {
				t.Errorf("Is%s should return true for wrapped sentinel", tc.name)
			}
		})
	}
}

func TestScheduleTimeErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  error
		pred func(error) bool
	}{
		{ErrScheduleTimeNotPresent, IsScheduleTimeNotPresent},
		{ErrScheduleTimeTooSoon, IsScheduleTimeTooSoon},
		{ErrScheduleTimeMustBeUTC, IsScheduleTimeMustBeUTC},
		{ErrScheduleTimeOutsideWindow, IsScheduleTimeOutsideWindow},
		{ErrCampaignRescheduleNotAllowed, IsCampaignRescheduleNotAllowed},
	}
	for _, tc := range cases {
		if !tc.pred(tc.err) {
			t.Errorf("predicate should return true for %v", tc.err)
		}
	}
}
