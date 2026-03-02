package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/utils"
)

type PayamSMSItem struct {
	Recipient  string
	Body       string
	trackingID string
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

type PayamSMSClient interface {
	SendBatchWithBodies(ctx context.Context, sender string, items []PayamSMSItem) ([]PayamSMSResponseItem, error)
	GetToken(ctx context.Context) (string, error)
	FetchStatus(ctx context.Context, token string, ids []string) ([]PayamStatusResponse, error)
}

type httpPayamSMSClient struct {
	cfg    config.PayamSMSConfig
	client *http.Client
}

func newHTTPPayamSMSClient(cfg config.PayamSMSConfig) *httpPayamSMSClient {
	return &httpPayamSMSClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// SendBatchWithBodies sends a batch using custom per-recipient bodies.
func (c *httpPayamSMSClient) SendBatchWithBodies(ctx context.Context, sender string, items []PayamSMSItem) ([]PayamSMSResponseItem, error) {
	if len(items) == 0 {
		return nil, nil
	}
	token, err := c.GetToken(ctx)
	if err != nil {
		return nil, err
	}
	payload := struct {
		Sender   string `json:"sender"`
		SMSItems []any  `json:"smsItems"`
	}{
		Sender:   sender,
		SMSItems: make([]any, 0, len(items)),
	}
	sendDate, err := utils.TehranNow()
	if err != nil {
		return nil, err
	}
	sendDate = sendDate.Add(time.Minute)

	for _, it := range items {
		payload.SMSItems = append(payload.SMSItems, map[string]any{
			"recipient":  it.Recipient,
			"body":       it.Body,
			"customerId": it.trackingID,
			"sendDate":   sendDate.Format("2006-01-02 15:04:05"),
		})
	}
	b, _ := json.Marshal(payload)
	url := "https://www.payamsms.com/panel/webservice/sendMultipleWithSrc"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
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

func (c *httpPayamSMSClient) GetToken(ctx context.Context) (string, error) {
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

	url := fmt.Sprintf("%s?systemName=%s&username=%s&password=%s&scope=%s&grant_type=%s", tokenURL, systemName, username, password, scope, grantType)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
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

func (c *httpPayamSMSClient) FetchStatus(ctx context.Context, token string, trackingIDs []string) ([]PayamStatusResponse, error) {
	if len(trackingIDs) == 0 {
		return nil, fmt.Errorf("no tracking ids provided")
	}
	baseURL := "https://www.payamsms.com/report/webservice/status"
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
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
	log.Printf("payamsms status curl: curl -X GET %q -H %q", u.String(), "Authorization: Bearer "+token)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		body := strings.TrimSpace(string(bodyBytes))
		if readErr != nil {
			body = fmt.Sprintf("unable to read response body: %v", readErr)
		}
		return nil, fmt.Errorf("payamsms status http status: %d, body: %s", resp.StatusCode, body)
	}

	var out []PayamStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
