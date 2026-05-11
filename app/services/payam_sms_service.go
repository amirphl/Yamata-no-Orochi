// Package services provides external service integrations and technical concerns like notifications and tokens
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/utils"
)

type PayamSMSSMSService struct {
	smsConfig   *config.SMSConfig
	payamConfig *config.PayamSMSConfig
	client      *http.Client
	mu          sync.RWMutex
	token       string
	tokenAt     time.Time
}

const payamSMSTokenTTL = time.Hour

type payamSMSTokenResponse struct {
	AccessToken string `json:"access_token"`
}

type payamSMSBulkPayload struct {
	Sender   string                 `json:"sender"`
	SMSItems []payamSMSBulkItemBody `json:"smsItems"`
}

type payamSMSBulkItemBody struct {
	Recipient  string `json:"recipient"`
	Body       string `json:"body"`
	CustomerID string `json:"customerId"`
	SendDate   string `json:"sendDate"`
}

type payamSMSBulkResponseItem struct {
	TrackingID string  `json:"customerId"`
	Mobile     string  `json:"mobile"`
	ServerID   *string `json:"serverId"`
	ErrorCode  *string `json:"errorCode"`
	Desc       *string `json:"description"`
}

func NewPayamSMSService(smsCfg *config.SMSConfig, payamCfg *config.PayamSMSConfig) SMSService {
	return &PayamSMSSMSService{
		smsConfig:   smsCfg,
		payamConfig: payamCfg,
		client: &http.Client{
			Timeout: smsCfg.Timeout,
		},
	}
}

func (s *PayamSMSSMSService) SendOTP(ctx context.Context, recipient, message string, customerID *int64) error {
	return s.SendSMS(ctx, recipient, message, customerID)
}

func (s *PayamSMSSMSService) SendSMS(ctx context.Context, recipient, message string, customerID *int64) error {
	return s.SendBulk(ctx, []string{recipient}, message, customerID)
}

func (s *PayamSMSSMSService) SendBulk(ctx context.Context, recipients []string, message string, customerID *int64) error {
	if len(recipients) == 0 {
		return nil
	}

	token, err := s.getToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get PayamSMS access token: %w", err)
	}

	sendDate, err := utils.TehranNow()
	if err != nil {
		return fmt.Errorf("failed to resolve Tehran time: %w", err)
	}
	sendDate = sendDate.Add(5 * time.Second) // Add a small delay to ensure the sendDate is in the future

	payload := payamSMSBulkPayload{
		Sender:   s.smsConfig.SourceNumber,
		SMSItems: make([]payamSMSBulkItemBody, 0, len(recipients)),
	}
	for idx, recipient := range recipients {
		payload.SMSItems = append(payload.SMSItems, payamSMSBulkItemBody{
			Recipient:  recipient,
			Body:       message,
			CustomerID: buildPayamSMSCustomerID(nil, idx),
			SendDate:   sendDate.Format("2006-01-02 15:04:05"),
		})
	}

	requestBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal PayamSMS request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.payamsms.com/panel/webservice/sendMultipleWithSrc", bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("failed to create PayamSMS request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send PayamSMS request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PayamSMS send http status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var results []payamSMSBulkResponseItem
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return fmt.Errorf("failed to decode PayamSMS response: %w", err)
	}

	for _, result := range results {
		if result.ErrorCode != nil && strings.TrimSpace(*result.ErrorCode) != "" {
			description := ""
			if result.Desc != nil {
				description = strings.TrimSpace(*result.Desc)
			}
			return fmt.Errorf("PayamSMS delivery failed for %s: %s (%s)", result.Mobile, description, strings.TrimSpace(*result.ErrorCode))
		}
	}

	return nil
}

func (s *PayamSMSSMSService) getToken(ctx context.Context) (string, error) {
	s.mu.RLock()
	if s.token != "" && time.Since(s.tokenAt) < payamSMSTokenTTL {
		token := s.token
		s.mu.RUnlock()
		return token, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token != "" && time.Since(s.tokenAt) < payamSMSTokenTTL {
		return s.token, nil
	}

	tokenURL := strings.TrimSpace(s.payamConfig.TokenURL)
	if tokenURL == "" {
		tokenURL = "https://www.payamsms.com/auth/oauth/token"
	}

	query := url.Values{}
	query.Set("systemName", s.payamConfig.SystemName)
	query.Set("username", s.payamConfig.Username)
	query.Set("password", s.payamConfig.Password)
	query.Set("scope", defaultString(s.payamConfig.Scope, "webservice"))
	query.Set("grant_type", defaultString(s.payamConfig.GrantType, "password"))

	requestURL := tokenURL
	if strings.Contains(tokenURL, "?") {
		requestURL += "&" + query.Encode()
	} else {
		requestURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create PayamSMS token request: %w", err)
	}
	if strings.TrimSpace(s.payamConfig.RootAccessToken) != "" {
		req.Header.Set("Authorization", "Basic "+s.payamConfig.RootAccessToken)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request PayamSMS token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("PayamSMS token http status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tokenResp payamSMSTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode PayamSMS token response: %w", err)
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return "", fmt.Errorf("PayamSMS token response did not contain access_token")
	}

	s.token = tokenResp.AccessToken
	s.tokenAt = utils.UTCNow()

	return s.token, nil
}

func buildPayamSMSCustomerID(customerID *int64, idx int) string {
	if customerID != nil {
		if idx == 0 {
			return strconv.FormatInt(*customerID, 10)
		}
		return fmt.Sprintf("%d-%d", *customerID, idx)
	}
	return fmt.Sprintf("otp-%d-%d", utils.UTCNowUnixNano(), idx)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
