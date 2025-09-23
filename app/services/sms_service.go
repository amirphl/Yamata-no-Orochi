// Package services provides external service integrations and technical concerns like notifications and tokens
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/utils"
)

// SMSService handles SMS sending operations
type SMSService interface {
	SendOTP(ctx context.Context, recipient, message string, customerID *int64) error
	SendSMS(ctx context.Context, recipient, message string, customerID *int64) error
}

// SMSServiceImpl implements SMSService
type SMSServiceImpl struct {
	config *config.SMSConfig
	client *http.Client
}

// SMSRequest represents the request payload for SMS API
type SMSRequest struct {
	SrcNum         string `json:"srcNum"`               // Format: 98**********
	Recipient      string `json:"recipient"`            // Format: 98**********
	Body           string `json:"body"`                 // Message content
	CustomerID     *int64 `json:"customerId,omitempty"` // Optional customer ID
	RetryCount     int    `json:"retryCount"`           // Number of retries
	Type           int    `json:"type"`                 // Always 1
	ValidityPeriod int    `json:"validityPeriod"`       // Validity in seconds
}

// SMSResponse represents individual message result from SMS API
type SMSResponse struct {
	MessageID  int64  `json:"messageId"`
	SrcNum     string `json:"srcNum"`
	Recipient  string `json:"recipient"`
	CustomerID *int64 `json:"customerId,omitempty"`
	Status     string `json:"status"`
	StatusCode int    `json:"statusCode"`
}

// NewSMSService creates a new SMS service instance
func NewSMSService(cfg *config.SMSConfig) SMSService {
	return &SMSServiceImpl{
		config: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// SendOTP sends an OTP message via SMS
func (s *SMSServiceImpl) SendOTP(ctx context.Context, recipient, message string, customerID *int64) error {
	return s.SendSMS(ctx, recipient, message, customerID)
}

// SendSMS sends an SMS message
func (s *SMSServiceImpl) SendSMS(ctx context.Context, recipient, message string, customerID *int64) error {
	// Prepare the request payload
	request := SMSRequest{
		SrcNum:         s.config.SourceNumber,
		Recipient:      recipient,
		Body:           message,
		CustomerID:     customerID,
		RetryCount:     s.config.RetryCount,
		Type:           1, // Always 1 as per specification
		ValidityPeriod: s.config.ValidityPeriod,
	}

	// Convert request to JSON
	requestBody, err := json.Marshal([]SMSRequest{request})
	if err != nil {
		return fmt.Errorf("failed to marshal SMS request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("https://%s/api/v3.0.1/send", s.config.ProviderDomain)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.config.APIKey)

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send SMS request: %w", err)
	}
	defer resp.Body.Close()

	// Parse response - the API returns an array of message results directly
	var messageResults []SMSResponse
	if err := json.NewDecoder(resp.Body).Decode(&messageResults); err != nil {
		return fmt.Errorf("failed to decode SMS response: %w", err)
	}

	// Check if any messages failed
	if len(messageResults) > 0 {
		for _, result := range messageResults {
			if result.StatusCode != 200 || result.Status != "ACCEPTED" {
				return fmt.Errorf("SMS delivery failed: %s (status: %d)", result.Status, result.StatusCode)
			}
		}
	}

	return nil
}

// MockSMSService implements SMSService for testing
type MockSMSService struct {
	SentMessages []MockSMSMessage
}

// MockSMSMessage represents a mock SMS message
type MockSMSMessage struct {
	Recipient  string
	Message    string
	CustomerID *int64
	SentAt     time.Time
}

// NewMockSMSService creates a new mock SMS service
func NewMockSMSService() SMSService {
	return &MockSMSService{
		SentMessages: make([]MockSMSMessage, 0),
	}
}

// SendOTP sends a mock OTP message
func (m *MockSMSService) SendOTP(ctx context.Context, recipient, message string, customerID *int64) error {
	return m.SendSMS(ctx, recipient, message, customerID)
}

// SendSMS sends a mock SMS message
func (m *MockSMSService) SendSMS(ctx context.Context, recipient, message string, customerID *int64) error {
	mockMessage := MockSMSMessage{
		Recipient:  recipient,
		Message:    message,
		CustomerID: customerID,
		SentAt:     utils.UTCNow(),
	}
	fmt.Println("Mock SMS message sent:", mockMessage)
	m.SentMessages = append(m.SentMessages, mockMessage)
	return nil
}

// GetSentMessages returns all sent mock messages
func (m *MockSMSService) GetSentMessages() []MockSMSMessage {
	return m.SentMessages
}

// ClearSentMessages clears the sent messages list
func (m *MockSMSService) ClearSentMessages() {
	m.SentMessages = make([]MockSMSMessage, 0)
}
