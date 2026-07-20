package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/config"
)

const (
	payamRetryBaseDelay   = 1 * time.Second
	payamRetryMaxDelay    = 2 * time.Minute
	payamRetryMaxAttempts = 5 // 0 means unlimited retries until success or context cancellation.
)

func payamRetryBackoffDelay(attempt int) time.Duration {
	return retryBackoffDelay(attempt, payamRetryBaseDelay, payamRetryMaxDelay)
}

func isPayamRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "http status: 500") ||
		strings.Contains(msg, "http status: 502") ||
		strings.Contains(msg, "http status: 503") ||
		strings.Contains(msg, "http status: 504") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "timed out") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "temporary failure") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "eof")
}

type PayamSMSItem struct {
	Recipient  string
	Body       string
	TrackingID string
}

type PayamSMSResponseItem struct {
	TrackingID string  `json:"customerId"`
	Mobile     string  `json:"mobile"`
	ServerID   *string `json:"serverId"`
	ErrorCode  *string `json:"errorCode"`
	Desc       *string `json:"description"`
}

type PayamStatusResponse struct {
	TrackingID            string  `json:"customerId"`
	ServerID              *string `json:"serverId"`
	TotalParts            int64   `json:"totalParts"`
	TotalDeliveredParts   int64   `json:"totalDeliveredParts"`
	TotalUndeliveredParts int64   `json:"totalUnDeliveredParts"`
	TotalUnknownParts     int64   `json:"totalUnKnownParts"`
	Status                string  `json:"status"`
}

// PayamStatusFetchResult keeps the provider payload alongside its parsed items.
// RawResponse is set whenever an HTTP response body was received, even if the
// response is empty, partial, non-2xx, or cannot be decoded.
type PayamStatusFetchResult struct {
	Items       []PayamStatusResponse
	RawResponse *string
}

type PayamSMSClient interface {
	SendBatch(ctx context.Context, sender string, items []PayamSMSItem) ([]PayamSMSResponseItem, error)
	GetToken(ctx context.Context) (string, error)
	FetchStatus(ctx context.Context, token string, ids []string) (PayamStatusFetchResult, error)
}

type httpPayamSMSClient struct {
	cfg    config.PayamSMSConfig
	client *http.Client
}

func newHTTPPayamSMSClient(cfg config.PayamSMSConfig) *httpPayamSMSClient {
	return newHTTPPayamSMSClientWithClient(cfg, newHTTPClient(60*time.Second))
}

func newHTTPPayamSMSClientWithClient(cfg config.PayamSMSConfig, client *http.Client) *httpPayamSMSClient {
	if client == nil {
		client = newHTTPClient(60 * time.Second)
	}
	return &httpPayamSMSClient{
		cfg:    cfg,
		client: client,
	}
}

// SendBatch sends a batch of SMS messages with exponential backoff retries.
func (c *httpPayamSMSClient) SendBatch(ctx context.Context, sender string, items []PayamSMSItem) ([]PayamSMSResponseItem, error) {
	if len(items) == 0 {
		return nil, nil
	}
	// GetToken already retries internally; no need to re-fetch on each send retry.
	token, err := c.GetToken(ctx)
	if err != nil {
		return nil, err
	}

	var out []PayamSMSResponseItem
	for attempt := 0; ; attempt++ {
		out, err = c.sendBatchOnce(ctx, sender, items, token)
		if !isPayamRetryableError(err) {
			return out, err
		}
		if payamRetryMaxAttempts > 0 && attempt+1 >= payamRetryMaxAttempts {
			break
		}
		if sleepErr := sleepWithContext(ctx, payamRetryBackoffDelay(attempt)); sleepErr != nil {
			return out, ctx.Err()
		}
	}
	return out, err
}

func (c *httpPayamSMSClient) sendBatchOnce(ctx context.Context, sender string, items []PayamSMSItem, token string) ([]PayamSMSResponseItem, error) {
	payload := struct {
		Sender   string `json:"sender"`
		SMSItems []any  `json:"smsItems"`
	}{
		Sender:   sender,
		SMSItems: make([]any, 0, len(items)),
	}
	// sendDate, err := utils.TehranNow()
	// if err != nil {
	// 	return nil, err
	// }
	// sendDate = sendDate.Add(time.Minute)

	for _, it := range items {
		payload.SMSItems = append(payload.SMSItems, map[string]any{
			"recipient":  it.Recipient,
			"body":       it.Body,
			"customerId": it.TrackingID,
			// "sendDate":   sendDate.Format("2006-01-02 15:04:05"),
		})
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("payamsms sendBatch marshal payload: %w", err)
	}

	// BUG FIX 2: local variable renamed from `url` to `sendURL` to stop shadowing
	// the imported "net/url" package within this function.
	sendURL := "https://www.payamsms.com/panel/webservice/sendMultipleWithSrc"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("payamsms sendMultiple http status: %d", resp.StatusCode)
	}
	var out []PayamSMSResponseItem
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetToken fetches a fresh OAuth2 bearer token from PayamSMS with exponential backoff retries.
func (c *httpPayamSMSClient) GetToken(ctx context.Context) (string, error) {
	var (
		token string
		err   error
	)
	for attempt := 0; ; attempt++ {
		token, err = c.getTokenOnce(ctx)
		if !isPayamRetryableError(err) {
			return token, err
		}
		if payamRetryMaxAttempts > 0 && attempt+1 >= payamRetryMaxAttempts {
			break
		}
		if sleepErr := sleepWithContext(ctx, payamRetryBackoffDelay(attempt)); sleepErr != nil {
			return "", ctx.Err()
		}
	}
	return token, err
}

func (c *httpPayamSMSClient) getTokenOnce(ctx context.Context) (string, error) {
	tokenURL := c.cfg.TokenURL
	if tokenURL == "" {
		tokenURL = "https://www.payamsms.com/auth/oauth/token"
	}
	systemName := c.cfg.SystemName
	username := c.cfg.Username
	password := c.cfg.Password
	scope := c.cfg.Scope
	grantType := c.cfg.GrantType
	rootToken := c.cfg.RootAccessToken
	if scope == "" {
		scope = "webservice"
	}
	if grantType == "" {
		grantType = "password"
	}

	q := url.Values{}
	q.Set("systemName", systemName)
	q.Set("username", username)
	q.Set("password", password)
	q.Set("scope", scope)
	q.Set("grant_type", grantType)
	tokenReqURL := tokenURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenReqURL, nil)
	if err != nil {
		return "", err
	}
	if rootToken != "" {
		req.Header.Set("Authorization", "Basic "+rootToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("payamsms token http status: %d", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("empty access_token")
	}
	return out.AccessToken, nil
}

// FetchStatus retrieves delivery statuses for the given tracking IDs with exponential backoff retries.
func (c *httpPayamSMSClient) FetchStatus(ctx context.Context, token string, trackingIDs []string) (PayamStatusFetchResult, error) {
	var (
		out PayamStatusFetchResult
		err error
	)
	for attempt := 0; ; attempt++ {
		out, err = c.fetchStatusOnce(ctx, token, trackingIDs)
		if !isPayamRetryableError(err) {
			return out, err
		}
		if payamRetryMaxAttempts > 0 && attempt+1 >= payamRetryMaxAttempts {
			break
		}
		if sleepErr := sleepWithContext(ctx, payamRetryBackoffDelay(attempt)); sleepErr != nil {
			return out, ctx.Err()
		}
	}
	return out, err
}

func (c *httpPayamSMSClient) fetchStatusOnce(ctx context.Context, token string, trackingIDs []string) (PayamStatusFetchResult, error) {
	if len(trackingIDs) == 0 {
		return PayamStatusFetchResult{}, fmt.Errorf("no tracking ids provided")
	}
	baseURL := "https://www.payamsms.com/report/webservice/status"
	u, err := url.Parse(baseURL)
	if err != nil {
		return PayamStatusFetchResult{}, err
	}
	q := u.Query()
	q.Set("byCustomer", "true")
	for _, trackingID := range trackingIDs {
		if strings.TrimSpace(trackingID) != "" {
			q.Add("ids", strings.TrimSpace(trackingID))
		}
	}
	u.RawQuery = q.Encode()

	// Log a curl command for manual retries when the API fails.
	// log.Printf("payamsms status curl: curl -X GET %q -H %q", u.String(), "Authorization: Bearer "+token)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return PayamStatusFetchResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client.Do(req)
	if err != nil {
		return PayamStatusFetchResult{}, err
	}
	defer resp.Body.Close()

	bodyBytes, readErr := io.ReadAll(resp.Body)
	rawResponse := string(bodyBytes)
	result := PayamStatusFetchResult{RawResponse: &rawResponse}
	if readErr != nil {
		return result, fmt.Errorf("read payamsms status response: %w", readErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("payamsms status http status: %d, body: %s", resp.StatusCode, strings.TrimSpace(rawResponse))
	}

	if err := json.Unmarshal(bodyBytes, &result.Items); err != nil {
		return result, err
	}
	return result, nil
}
