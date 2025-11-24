package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// BithideClient implements CryptoPaymentProvider
// NOTE: Implemented using BitHide Public API described in openapi.json and docs
// Docs: https://docs.bithide.io/
type BithideClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	// confirmations per network or coin symbol
	DefaultConfirmations map[string]int // key: network or coin (e.g., "ETH", "BNB", "XRP", "DOGE")
}

func NewBithideClient(baseURL, apiKey string, timeout time.Duration, defaultConfirmations map[string]int) *BithideClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &BithideClient{
		BaseURL:              strings.TrimRight(baseURL, "/"),
		APIKey:               apiKey,
		HTTPClient:           &http.Client{Timeout: timeout},
		DefaultConfirmations: defaultConfirmations,
	}
}

func (c *BithideClient) Name() string { return "bithide" }

// GetQuote fetches the latest USDT price for the requested coin using Wallex public API
// RateSource will be set to "wallex:<PAIR>", ExchangeRate will be the numeric price string (USDT per 1 COIN)
// Example pairs: ETHUSDT, BNBUSDT, XRPUSDT, DOGEUSDT
func (c *BithideClient) GetQuote(ctx context.Context, in QuoteInput) (*QuoteResult, error) {
	pair := pairForCoin(strings.ToUpper(in.Coin))
	if pair == "" {
		return &QuoteResult{ExpectedCoinAmount: "", ExchangeRate: "", RateSource: "unsupported-coin", ExpiresAt: nil}, nil
	}
	price, err := c.fetchWallexPrice(ctx, pair)
	if err != nil {
		// Do not fail hard; return empty quote with error source tag
		return &QuoteResult{ExpectedCoinAmount: "", ExchangeRate: "", RateSource: "wallex:error", ExpiresAt: nil}, nil
	}
	exp := time.Now().Add(1 * time.Minute)
	return &QuoteResult{
		ExpectedCoinAmount: "",
		ExchangeRate:       price, // USDT per 1 COIN
		RateSource:         "wallex:" + pair,
		ExpiresAt:          &exp,
	}, nil
}

// ----- Address/GetAddress -----

// AddressRequestParam according to OpenAPI (subset used)
type addressRequestParam struct {
	Currency     string `json:"Currency,omitempty"`
	ApiKey       string `json:"ApiKey,omitempty"`
	New          bool   `json:"New"`
	Label        string `json:"Label,omitempty"`
	CallBackLink string `json:"CallBackLink,omitempty"`
}

type addressItem struct {
	Address *string `json:"Address"`
	Memo    *string `json:"Memo"`
	Tag     *string `json:"Tag"`
}

type addressResponse struct {
	Status       string        `json:"Status"`
	ErrorCode    *string       `json:"ErrorCode"`
	ErrorMessage *string       `json:"ErrorMessage"`
	Address      *string       `json:"Address"`
	Memo         *string       `json:"Memo"`
	Addresses    []addressItem `json:"Addresses"`
	Error        *struct {
		Code *string `json:"Code"`
	} `json:"Error"`
}

func (c *BithideClient) ProvisionDeposit(ctx context.Context, in ProvisionInput) (*ProvisionResult, error) {
	body := addressRequestParam{Currency: in.Coin, ApiKey: c.APIKey, New: true, Label: in.Label, CallBackLink: in.CallbackURL}
	var ar addressResponse
	if err := c.postJSON(ctx, "/Address/GetAddress", body, &ar); err != nil {
		return nil, err
	}
	if ar.ErrorCode != nil && *ar.ErrorCode != "" {
		return nil, fmt.Errorf("bithide: address error %s: %s", *ar.ErrorCode, toStr(ar.ErrorMessage))
	}
	addr, memo := extractAddressMemo(&ar)
	if addr == "" {
		return nil, errors.New("bithide: empty address in response")
	}
	// We do not receive a provider request id; use label for correlation if needed
	return &ProvisionResult{DepositAddress: addr, DepositMemo: memo, ProviderRequestID: in.Label, ExpiresAt: nil}, nil
}

// ----- Transaction/List -----

type listRequest struct {
	Page          int         `json:"Page"`
	Count         int         `json:"Count"`
	Offset        int         `json:"Offset,omitempty"`
	Search        string      `json:"Search,omitempty"`
	SortField     string      `json:"SortField,omitempty"`
	SortDirection string      `json:"SortDirection,omitempty"`
	Filter        interface{} `json:"Filter,omitempty"`
	ApiKey        string      `json:"ApiKey,omitempty"`
}

type transactionVM struct {
	Id                 int64   `json:"Id"`
	Type               int     `json:"Type"` // 1=Deposit
	Date               string  `json:"Date"`
	NodeTime           *string `json:"NodeTime"`
	BlockNumber        *int64  `json:"BlockNumber"`
	TxId               *string `json:"TxId"`
	Cryptocurrency     *string `json:"Cryptocurrency"`
	Amount             float64 `json:"Amount"`
	AmountUSD          float64 `json:"AmountUSD"`
	Rate               float64 `json:"Rate"`
	Commission         float64 `json:"Commission"`
	CommissionCurrency *string `json:"CommissionCurrency"`
	DestinationAddress *string `json:"DestinationAddress"`
	Comment            *string `json:"Comment"`
	Status             int     `json:"Status"` // 2=Completed, 5=WaitingConfirmation
}

type listResponse struct {
	Page   int             `json:"Page"`
	Count  int             `json:"Count"`
	List   []transactionVM `json:"List"`
	Offset int             `json:"Offset"`
	Total  int64           `json:"Total"`
	Status *string         `json:"Status"`
	// optional error fields
	ErrorCode    *string `json:"ErrorCode"`
	ErrorMessage *string `json:"ErrorMessage"`
}

func (c *BithideClient) GetDeposits(ctx context.Context, providerRequestID string) ([]DepositInfo, error) {
	// providerRequestID: we use Label set during ProvisionDeposit
	if strings.TrimSpace(providerRequestID) == "" {
		return []DepositInfo{}, nil
	}
	req := listRequest{
		Page:   1,
		Count:  50,
		Search: providerRequestID,
		ApiKey: c.APIKey,
	}
	var lr listResponse
	if err := c.postJSON(ctx, "/Transaction/List", req, &lr); err != nil {
		return nil, err
	}
	if lr.ErrorCode != nil && *lr.ErrorCode != "" {
		return nil, fmt.Errorf("bithide: transaction list error %s: %s", *lr.ErrorCode, toStr(lr.ErrorMessage))
	}
	infos := make([]DepositInfo, 0)
	for _, t := range lr.List {
		if t.Type != 1 { // deposit only
			continue
		}
		coin := toStr(t.Cryptocurrency)
		status := mapTxStatus(t.Status)
		confReq := c.requiredConfirmations(coin)
		var detectedAt, confirmedAt *time.Time
		if t.Date != "" {
			if dt, err := time.Parse(time.RFC3339, t.Date); err == nil {
				detectedAt = &dt
			}
		}
		if t.NodeTime != nil && *t.NodeTime != "" {
			if nt, err := time.Parse(time.RFC3339, *t.NodeTime); err == nil {
				confirmedAt = &nt
			}
		}
		toAddr := toStr(t.DestinationAddress)
		infos = append(infos, DepositInfo{
			TxHash:                toStr(t.TxId),
			AmountCoin:            fmt.Sprintf("%g", t.Amount),
			Confirmations:         0, // not exposed in public API
			RequiredConfirmations: confReq,
			ToAddress:             toAddr,
			DestinationTag:        "",
			Status:                status,
			DetectedAt:            detectedAt,
			ConfirmedAt:           confirmedAt,
			CreditedAt:            nil,
		})
	}
	return infos, nil
}

func (c *BithideClient) VerifyTx(ctx context.Context, txHash string) (*DepositInfo, error) {
	if strings.TrimSpace(txHash) == "" {
		return nil, errors.New("tx hash required")
	}
	req := listRequest{Page: 1, Count: 1, Search: txHash, ApiKey: c.APIKey}
	var lr listResponse
	if err := c.postJSON(ctx, "/Transaction/List", req, &lr); err != nil {
		return nil, err
	}
	if lr.ErrorCode != nil && *lr.ErrorCode != "" {
		return nil, fmt.Errorf("bithide: transaction list error %s: %s", *lr.ErrorCode, toStr(lr.ErrorMessage))
	}
	for _, t := range lr.List {
		if t.TxId == nil || !strings.EqualFold(*t.TxId, txHash) {
			continue
		}
		coin := toStr(t.Cryptocurrency)
		status := mapTxStatus(t.Status)
		confReq := c.requiredConfirmations(coin)
		var detectedAt, confirmedAt *time.Time
		if t.Date != "" {
			if dt, err := time.Parse(time.RFC3339, t.Date); err == nil {
				detectedAt = &dt
			}
		}
		if t.NodeTime != nil && *t.NodeTime != "" {
			if nt, err := time.Parse(time.RFC3339, *t.NodeTime); err == nil {
				confirmedAt = &nt
			}
		}
		toAddr := toStr(t.DestinationAddress)
		info := &DepositInfo{
			TxHash:                toStr(t.TxId),
			AmountCoin:            fmt.Sprintf("%g", t.Amount),
			Confirmations:         0,
			RequiredConfirmations: confReq,
			ToAddress:             toAddr,
			DestinationTag:        "",
			Status:                status,
			DetectedAt:            detectedAt,
			ConfirmedAt:           confirmedAt,
			CreditedAt:            nil,
		}
		return info, nil
	}
	return nil, errors.New("transaction not found")
}

// ----- HTTP helpers -----

func (c *BithideClient) postJSON(ctx context.Context, path string, payload any, out any) error {
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bithide: status %d for %s", resp.StatusCode, path)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func extractAddressMemo(ar *addressResponse) (string, string) {
	var addr string
	var memo string
	if ar == nil {
		return "", ""
	}
	if ar.Address != nil && *ar.Address != "" {
		addr = *ar.Address
	}
	if ar.Memo != nil && *ar.Memo != "" {
		memo = *ar.Memo
	}
	if addr == "" && len(ar.Addresses) > 0 {
		if ar.Addresses[0].Address != nil {
			addr = *ar.Addresses[0].Address
		}
		if memo == "" {
			if ar.Addresses[0].Memo != nil {
				memo = *ar.Addresses[0].Memo
			} else if ar.Addresses[0].Tag != nil {
				memo = *ar.Addresses[0].Tag
			}
		}
	}
	return addr, memo
}

func (c *BithideClient) requiredConfirmations(key string) int {
	if key == "" {
		return 1
	}
	if v, ok := c.DefaultConfirmations[strings.ToUpper(key)]; ok {
		return v
	}
	return 1
}

func mapTxStatus(status int) string {
	switch status {
	case 2:
		return "confirmed"
	case 5:
		return "detected"
	case 3:
		return "failed"
	default:
		return "detected"
	}
}

func toStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ----- Wallex pricing helpers -----

type wallexTradesResponse struct {
	Result struct {
		LatestTrades []struct {
			Symbol    string `json:"symbol"`
			Price     string `json:"price"`
			Timestamp string `json:"timestamp"`
		} `json:"latestTrades"`
	} `json:"result"`
	Message string `json:"message"`
	Success bool   `json:"success"`
}

func pairForCoin(coin string) string {
	switch coin {
	case "ETH":
		return "ETHUSDT"
	case "BNB":
		return "BNBUSDT"
	case "XRP":
		return "XRPUSDT"
	case "DOGE":
		return "DOGEUSDT"
	default:
		return ""
	}
}

func (c *BithideClient) fetchWallexPrice(ctx context.Context, pair string) (string, error) {
	url := "https://api.wallex.ir/v1/trades?symbol=" + strings.ToLower(pair)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("wallex: http %d", resp.StatusCode)
	}
	var wr wallexTradesResponse
	if err := json.NewDecoder(resp.Body).Decode(&wr); err != nil {
		return "", err
	}
	if !wr.Success || len(wr.Result.LatestTrades) == 0 {
		return "", errors.New("wallex: empty trades")
	}
	// Take the most recent trade price
	return wr.Result.LatestTrades[0].Price, nil
}
