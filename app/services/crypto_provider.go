package services

import (
	"context"
	"time"
)

type QuoteInput struct {
	FiatAmountToman uint64
	Coin            string
	Network         string
}

type QuoteResult struct {
	ExpectedCoinAmount string
	ExchangeRate       string
	RateSource         string
	ExpiresAt          *time.Time
}

type ProvisionInput struct {
	QuoteInput
	Label       string
	CallbackURL string
}

type ProvisionResult struct {
	DepositAddress    string
	DepositMemo       string
	ProviderRequestID string
	ExpiresAt         *time.Time
}

type DepositInfo struct {
	TxHash                string
	AmountCoin            string
	Confirmations         int
	RequiredConfirmations int
	ToAddress             string
	DestinationTag        string
	Status                string // detected|confirmed|credited
	DetectedAt            *time.Time
	ConfirmedAt           *time.Time
	CreditedAt            *time.Time
}

type CryptoPaymentProvider interface {
	Name() string
	GetQuote(ctx context.Context, in QuoteInput) (*QuoteResult, error)
	ProvisionDeposit(ctx context.Context, in ProvisionInput) (*ProvisionResult, error)
	GetDeposits(ctx context.Context, providerRequestID string) ([]DepositInfo, error)
	VerifyTx(ctx context.Context, txHash string) (*DepositInfo, error)
}
