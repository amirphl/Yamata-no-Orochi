package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

type CoinremitterWalletConfig struct {
	APIKey      string
	APIPassword string
}

type CoinremitterClient struct {
	BaseURL    string
	HTTPClient *http.Client
	Timeout    time.Duration
	Wallets    map[string]CoinremitterWalletConfig // coin -> credentials
}

func NewCoinremitterClient(baseURL string, timeout time.Duration, wallets map[string]CoinremitterWalletConfig) *CoinremitterClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &CoinremitterClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: timeout},
		Timeout:    timeout,
		Wallets:    wallets,
	}
}

func (c *CoinremitterClient) Name() string { return "coinremitter" }

// Quote using supported currency endpoint (price_in_usd)
type crSupportedCurrencyResp struct {
	Success bool `json:"success"`
	Data    []struct {
		CoinSymbol  string `json:"coin_symbol"`
		PriceInUSD  string `json:"price_in_usd"`
		MinimumDep  string `json:"minimum_deposit_amount"`
		NetworkName string `json:"network_name"`
		ExplorerURL string `json:"explorer_url"`
		Logo        string `json:"logo"`
	} `json:"data"`
}

func (c *CoinremitterClient) GetQuote(ctx context.Context, in QuoteInput) (*QuoteResult, error) {
	// POST /v1/supported/currency (no body needed per docs)
	url := c.BaseURL + "/supported/currency"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &QuoteResult{
			ExpectedCoinAmount: "",
			ExchangeRate:       "",
			RateSource:         "coinremitter:error",
			ExpiresAt:          nil,
		}, nil
	}
	var out crSupportedCurrencyResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	coin := strings.ToUpper(in.Coin)
	price := ""
	for _, it := range out.Data {
		if strings.EqualFold(it.CoinSymbol, coin) {
			price = it.PriceInUSD
			break
		}
	}
	exp := time.Now().Add(1 * time.Minute)
	return &QuoteResult{ExpectedCoinAmount: "", ExchangeRate: price, RateSource: "coinremitter:supported-currency", ExpiresAt: &exp}, nil
}

// Provision address
// POST /v1/address/create (headers x-api-key, x-api-password; form field label)

type crCreateAddressResp struct {
	Success bool `json:"success"`
	Data    struct {
		Address           string  `json:"address"`
		Label             string  `json:"label"`
		ExpireOnTimestamp *int64  `json:"expire_on_timestamp"`
		MinimumDeposit    *string `json:"minimum_deposit_amount"`
		ExplorerURL       *string `json:"explorer_url"`
	} `json:"data"`
}

func (c *CoinremitterClient) ProvisionDeposit(ctx context.Context, in ProvisionInput) (*ProvisionResult, error) {
	creds, ok := c.Wallets[strings.ToUpper(in.Coin)]
	if !ok || creds.APIKey == "" || creds.APIPassword == "" {
		return nil, errors.New("coinremitter: wallet credentials not configured for coin")
	}
	url := c.BaseURL + "/address/create"
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("label", in.Label)
	_ = writer.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("x-api-key", creds.APIKey)
	req.Header.Set("x-api-password", creds.APIPassword)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("coinremitter: status %d", resp.StatusCode)
	}
	var out crCreateAddressResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	addr := out.Data.Address
	var memo string
	var expires *time.Time
	if out.Data.ExpireOnTimestamp != nil && *out.Data.ExpireOnTimestamp > 0 {
		t := time.UnixMilli(*out.Data.ExpireOnTimestamp).UTC()
		expires = &t
	}
	return &ProvisionResult{DepositAddress: addr, DepositMemo: memo, ProviderRequestID: addr, ExpiresAt: expires}, nil
}

// Deposits listing is typically via webhooks or address tx listing; not used in our polling flow
func (c *CoinremitterClient) GetDeposits(ctx context.Context, providerRequestID string) ([]DepositInfo, error) {
	return []DepositInfo{}, nil
}

// VerifyTx requires explicit transaction endpoints; provide a stub to encourage webhook/manual verification
func (c *CoinremitterClient) VerifyTx(ctx context.Context, txHash string) (*DepositInfo, error) {
	return nil, errors.New("coinremitter: VerifyTx not implemented; use webhook or manual verification")
}
