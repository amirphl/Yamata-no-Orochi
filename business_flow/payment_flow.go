// Package businessflow contains the core business logic and use cases for payment workflows
package businessflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PaymentFlow handles the complete payment business logic
type PaymentFlow interface {
	GetPaymentToken(ctx context.Context, req *dto.GetPaymentTokenRequest, metadata *ClientMetadata) (*dto.GetPaymentTokenResponse, error)
	HandlePaymentCallback(ctx context.Context, callback *dto.PaymentCallbackRequest, metadata *ClientMetadata) (*dto.PaymentCallbackResponse, error)
}

// PaymentFlowImpl implements the payment business flow
type PaymentFlowImpl struct {
	paymentRequestRepo repository.PaymentRequestRepository
	walletRepo         repository.WalletRepository
	customerRepo       repository.CustomerRepository
	auditRepo          repository.AuditLogRepository
	db                 *gorm.DB

	// Atipay configuration
	atipayBaseURL  string
	atipayAPIKey   string
	atipayTerminal string
}

// NewPaymentFlow creates a new payment flow instance
func NewPaymentFlow(
	paymentRequestRepo repository.PaymentRequestRepository,
	walletRepo repository.WalletRepository,
	customerRepo repository.CustomerRepository,
	auditRepo repository.AuditLogRepository,
	db *gorm.DB,
	atipayBaseURL, atipayAPIKey, atipayTerminal string,
) PaymentFlow {
	return &PaymentFlowImpl{
		paymentRequestRepo: paymentRequestRepo,
		walletRepo:         walletRepo,
		customerRepo:       customerRepo,
		auditRepo:          auditRepo,
		db:                 db,
		atipayBaseURL:      atipayBaseURL,
		atipayAPIKey:       atipayAPIKey,
		atipayTerminal:     atipayTerminal,
	}
}

// GetPaymentToken handles the complete process of getting a payment token from Atipay
func (p *PaymentFlowImpl) GetPaymentToken(ctx context.Context, req *dto.GetPaymentTokenRequest, metadata *ClientMetadata) (*dto.GetPaymentTokenResponse, error) {
	// Validate business rules
	if err := p.validateGetTokenRequest(ctx, req); err != nil {
		return nil, NewBusinessError("PAYMENT_VALIDATION_FAILED", "Payment validation failed", err)
	}

	// Use transaction for atomicity
	var paymentRequest *models.PaymentRequest
	var atipayToken string

	err := repository.WithTransaction(ctx, p.db, func(txCtx context.Context) error {
		// Create payment request record
		var err error
		paymentRequest, err = p.createPaymentRequest(txCtx, req)
		if err != nil {
			return err
		}

		// Call Atipay to get token
		atipayToken, err = p.callAtipayGetToken(txCtx, req, paymentRequest)
		if err != nil {
			return err
		}

		// Update payment request with Atipay token
		paymentRequest.AtipayToken = atipayToken
		paymentRequest.Status = models.PaymentRequestStatusTokenized
		if err := p.paymentRequestRepo.Save(txCtx, paymentRequest); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Payment token generation failed: %s", err.Error())
		_ = p.createAuditLog(ctx, nil, "payment_token_failed", errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("PAYMENT_TOKEN_FAILED", "Failed to generate payment token", err)
	}

	// Create success audit log
	msg := fmt.Sprintf("Payment token generated successfully: %s", paymentRequest.UUID)
	_ = p.createAuditLog(ctx, nil, "payment_token_generated", msg, true, nil, metadata)

	// Build response
	response := &dto.GetPaymentTokenResponse{
		Message:          "Payment token generated successfully",
		PaymentRequestID: paymentRequest.ID,
		UUID:             paymentRequest.UUID.String(),
		AtipayToken:      atipayToken,
		RedirectURL:      fmt.Sprintf("%s/redirect-to-gateway", p.atipayBaseURL),
		Amount:           paymentRequest.Amount,
		Description:      paymentRequest.Description,
		ExpiresAt:        *paymentRequest.ExpiresAt,
		Status:           string(paymentRequest.Status),
	}

	return response, nil
}

// validateGetTokenRequest validates the business rules for getting a payment token
func (p *PaymentFlowImpl) validateGetTokenRequest(ctx context.Context, req *dto.GetPaymentTokenRequest) error {
	// Check if customer exists and is active
	customer, err := p.customerRepo.ByID(ctx, req.CustomerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return ErrCustomerNotFound
	}
	if customer.IsActive != nil && !*customer.IsActive {
		return ErrAccountInactive
	}

	// Check if customer has a wallet
	wallet, err := p.walletRepo.ByCustomerID(ctx, req.CustomerID)
	if err != nil {
		return err
	}
	if wallet == nil {
		return ErrWalletNotFound
	}

	// Validate amount (minimum 10000 Tomans and must be multiple of 10000)
	if req.Amount < 10000 {
		return ErrAmountTooLow
	}
	if req.Amount%10000 != 0 {
		return ErrAmountNotMultiple
	}

	// Validate description
	if len(req.Description) == 0 || len(req.Description) > 255 {
		return fmt.Errorf("description must be between 1 and 255 characters")
	}

	// Validate redirect URL
	if len(req.RedirectURL) == 0 {
		return fmt.Errorf("redirect URL is required")
	}

	return nil
}

// createPaymentRequest creates a new payment request record
func (p *PaymentFlowImpl) createPaymentRequest(ctx context.Context, req *dto.GetPaymentTokenRequest) (*models.PaymentRequest, error) {
	// Generate unique invoice number
	invoiceNumber := fmt.Sprintf("INV-%s-%d", uuid.New().String(), time.Now().Unix())

	// Set expiration time (30 minutes from now)
	expiresAt := time.Now().Add(30 * time.Minute)

	// Create payment request
	paymentRequest := &models.PaymentRequest{
		CorrelationID: uuid.New(),
		CustomerID:    req.CustomerID,
		Amount:        req.Amount,
		Currency:      "TMN",
		Description:   req.Description,
		InvoiceNumber: invoiceNumber,
		CellNumber:    req.CellNumber,
		RedirectURL:   req.RedirectURL,
		Status:        models.PaymentRequestStatusCreated,
		ExpiresAt:     &expiresAt,
		Metadata: map[string]interface{}{
			"source": "wallet_recharge",
		},
	}

	// Save to database
	if err := p.paymentRequestRepo.Save(ctx, paymentRequest); err != nil {
		return nil, err
	}

	return paymentRequest, nil
}

// callAtipayGetToken calls Atipay's get-token API
func (p *PaymentFlowImpl) callAtipayGetToken(ctx context.Context, req *dto.GetPaymentTokenRequest, paymentRequest *models.PaymentRequest) (string, error) {
	// Prepare Atipay request payload
	atipayPayload := map[string]interface{}{
		"amount":        req.Amount,
		"cellNumber":    req.CellNumber,
		"description":   req.Description,
		"invoiceNumber": paymentRequest.InvoiceNumber,
		"redirectUrl":   req.RedirectURL,
		"apiKey":        p.atipayAPIKey,
	}

	// Convert to JSON
	payloadBytes, err := json.Marshal(atipayPayload)
	if err != nil {
		return "", err
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/get-token", p.atipayBaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Make HTTP request
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("atipay API returned non-OK status: %d", resp.StatusCode)
	}

	// Parse response body
	var atipayResponse struct {
		Status           string `json:"status"`
		Token            string `json:"token"`
		ErrorCode        string `json:"errorCode,omitempty"`
		ErrorDescription string `json:"errorDescription,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&atipayResponse); err != nil {
		return "", err
	}

	// Check for errors in response
	if atipayResponse.Status != "OK" {
		errorMsg := "unknown error"
		if atipayResponse.ErrorDescription != "" {
			errorMsg = atipayResponse.ErrorDescription
		}
		if atipayResponse.ErrorCode != "" {
			errorMsg = fmt.Sprintf("%s (code: %s)", errorMsg, atipayResponse.ErrorCode)
		}
		return "", fmt.Errorf("atipay API error: %s", errorMsg)
	}

	// Validate token
	if atipayResponse.Token == "" {
		return "", ErrAtipayTokenEmpty
	}

	return atipayResponse.Token, nil
}

// validateCallbackRequest validates the callback request from Atipay
func (p *PaymentFlowImpl) validateCallbackRequest(callback *dto.PaymentCallbackRequest) error {
	if callback == nil {
		return fmt.Errorf("callback request is nil")
	}
	if callback.ReservationNumber == "" {
		return fmt.Errorf("reservation number is required")
	}
	if callback.ReferenceNumber == "" {
		return fmt.Errorf("reference number is required")
	}
	if callback.Status == "" {
		return fmt.Errorf("status is required")
	}
	if callback.State == "" {
		return fmt.Errorf("state is required")
	}
	return nil
}

// parseAtipayStatus parses Atipay status and state into meaningful values
func (p *PaymentFlowImpl) parseAtipayStatus(status, state string) (string, string) {
	// Return the raw values for now, can be enhanced with mapping if needed
	return status, state
}

// HandlePaymentCallback handles the callback from Atipay after payment completion
func (p *PaymentFlowImpl) HandlePaymentCallback(ctx context.Context, callback *dto.PaymentCallbackRequest, metadata *ClientMetadata) (*dto.PaymentCallbackResponse, error) {
	// Validate callback data
	if err := p.validateCallbackRequest(callback); err != nil {
		return nil, err
	}

	// Find the payment request by reservation number (our invoice number)
	paymentRequest, err := p.paymentRequestRepo.ByInvoiceNumber(ctx, callback.ReservationNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to find payment request: %w", err)
	}
	if paymentRequest == nil {
		return nil, fmt.Errorf("payment request not found for reservation number: %s", callback.ReservationNumber)
	}

	// Check if this callback has already been processed
	if paymentRequest.Status != models.PaymentRequestStatusPending {
		return &dto.PaymentCallbackResponse{
			Message:          "Callback already processed",
			Success:          true,
			PaymentRequestID: paymentRequest.ID,
			Status:           string(paymentRequest.Status),
		}, nil
	}

	// Parse status and state
	status, state := p.parseAtipayStatus(callback.Status, callback.State)

	// Update payment request status based on callback
	var newStatus models.PaymentRequestStatus
	var success bool
	var message string

	switch {
	case status == "2" && state == "OK":
		// Payment successful
		newStatus = models.PaymentRequestStatusCompleted
		success = true
		message = "Payment completed successfully"

		// Update payment request with Atipay details
		paymentRequest.Status = newStatus
		paymentRequest.PaymentReference = callback.ReferenceNumber
		paymentRequest.PaymentTrace = callback.TraceNumber
		paymentRequest.PaymentMaskedPAN = callback.MaskedPAN
		paymentRequest.PaymentRRN = callback.RRN
		paymentRequest.PaymentState = callback.State
		paymentRequest.PaymentStatus = callback.Status
		paymentRequest.StatusReason = "Payment completed successfully via Atipay"

	case status == "1" || state == "CanceledByUser":
		// User cancelled
		newStatus = models.PaymentRequestStatusCancelled
		success = false
		message = "Payment cancelled by user"

	case status == "3" || state == "Failed":
		// Payment failed
		newStatus = models.PaymentRequestStatusFailed
		success = false
		message = "Payment failed"

	case status == "4" || state == "SessionIsNull":
		// Session expired
		newStatus = models.PaymentRequestStatusExpired
		success = false
		message = "Payment session expired"

	case status == "5" || state == "InvalidParameters":
		// Invalid parameters
		newStatus = models.PaymentRequestStatusFailed
		success = false
		message = "Invalid payment parameters"

	case status == "8" || state == "MerchantIpAddressIsInvalid":
		// IP address invalid
		newStatus = models.PaymentRequestStatusFailed
		success = false
		message = "Merchant IP address invalid"

	case status == "10" || state == "TokenNotFound":
		// Token not found
		newStatus = models.PaymentRequestStatusFailed
		success = false
		message = "Payment token not found"

	case status == "11" || state == "TokenRequired":
		// Token required
		newStatus = models.PaymentRequestStatusFailed
		success = false
		message = "Payment token required"

	case status == "12" || state == "TerminalNotFound":
		// Terminal not found
		newStatus = models.PaymentRequestStatusFailed
		success = false
		message = "Payment terminal not found"

	default:
		// Unknown status
		newStatus = models.PaymentRequestStatusFailed
		success = false
		message = fmt.Sprintf("Unknown payment status: %s, state: %s", callback.Status, callback.State)
	}

	// Update payment request status
	paymentRequest.Status = newStatus
	if err := p.paymentRequestRepo.Save(ctx, paymentRequest); err != nil {
		return nil, fmt.Errorf("failed to update payment request status: %w", err)
	}

	// Create audit log
	customer, err := p.customerRepo.ByID(ctx, paymentRequest.CustomerID)
	if err != nil {
		// Log error but don't fail the callback
		errorMsg := err.Error()
		_ = p.createAuditLog(ctx, nil, "payment_callback_audit_failed",
			fmt.Sprintf("Failed to find customer %d for audit logging", paymentRequest.CustomerID),
			false, &errorMsg, metadata)
	} else {
		auditAction := "payment_callback_processed"
		if success {
			auditAction = "payment_completed"
		}
		_ = p.createAuditLog(ctx, customer, auditAction,
			fmt.Sprintf("Payment callback processed: %s", message),
			success, nil, metadata)
	}

	return &dto.PaymentCallbackResponse{
		Message:          message,
		Success:          success,
		PaymentRequestID: paymentRequest.ID,
		Status:           string(newStatus),
	}, nil
}

// verifyPayment verifies the payment with Atipay
func (p *PaymentFlowImpl) verifyPayment(ctx context.Context, req *dto.VerifyPaymentRequest, metadata *ClientMetadata) (*dto.VerifyPaymentResponse, error) {
	// TODO: Implement payment verification
	// This would involve:
	// 1. Calling Atipay's verify-payment API
	// 2. Processing the verification result
	// 3. Updating the payment request status
	// 4. Creating audit logs

	return &dto.VerifyPaymentResponse{
		Message:         "Payment verification completed",
		Success:         true,
		Amount:          0, // TODO: Get from Atipay
		ReferenceNumber: req.ReferenceNumber,
		VerifiedAt:      time.Now(),
		Status:          "verified",
	}, nil
}

// createAuditLog creates an audit log entry
func (p *PaymentFlowImpl) createAuditLog(ctx context.Context, customer *models.Customer, action string, description string, success bool, errorDetails *string, metadata *ClientMetadata) error {
	auditLog := &models.AuditLog{
		Action:      action,
		Description: &description,
		Success:     &success,
		IPAddress:   &metadata.IPAddress,
		UserAgent:   &metadata.UserAgent,
		Metadata:    json.RawMessage(`{"source": "payment_flow"}`),
	}

	if customer != nil {
		auditLog.CustomerID = &customer.ID
	}

	if errorDetails != nil {
		// Create metadata with error details
		metadataMap := map[string]interface{}{
			"source":        "payment_flow",
			"error_details": *errorDetails,
		}
		metadataBytes, _ := json.Marshal(metadataMap)
		auditLog.Metadata = metadataBytes
	}

	return p.auditRepo.Save(ctx, auditLog)
}
