// Package businessflow contains the core business logic and use cases for payment workflows
package businessflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// // VerifyPaymentRequest represents the request to verify payment with Atipay
// type VerifyPaymentRequest struct {
// 	ReferenceNumber string `json:"reference_number" validate:"required"` // Value received from Atipay callback
// }

// // VerifyPaymentResponse represents the response after payment verification
// type VerifyPaymentResponse struct {
// 	Message         string    `json:"message"`
// 	Success         bool      `json:"success"`
// 	Amount          uint64    `json:"amount"`           // Verified amount from Atipay
// 	ReferenceNumber string    `json:"reference_number"` // Atipay reference number
// 	VerifiedAt      time.Time `json:"verified_at"`      // When verification was completed
// 	Status          string    `json:"status"`           // Final payment status
// }

// // PaymentStatus represents the status of a payment request
// type PaymentStatus string

// const (
// 	PaymentStatusCreated   PaymentStatus = "created"   // Payment request created, waiting for Atipay token
// 	PaymentStatusTokenized PaymentStatus = "tokenized" // Atipay token received, waiting for user payment
// 	PaymentStatusPending   PaymentStatus = "pending"   // User redirected to Atipay, payment in progress
// 	PaymentStatusCompleted PaymentStatus = "completed" // Payment completed successfully
// 	PaymentStatusFailed    PaymentStatus = "failed"    // Payment failed
// 	PaymentStatusCancelled PaymentStatus = "cancelled" // User cancelled payment
// 	PaymentStatusExpired   PaymentStatus = "expired"   // Payment request expired
// 	PaymentStatusRefunded  PaymentStatus = "refunded"  // Payment was refunded
// )

// // PaymentError represents payment-related errors
// type PaymentError struct {
// 	Code        string `json:"code"`
// 	Message     string `json:"message"`
// 	Description string `json:"description,omitempty"`
// }

// PaymentFlow handles the complete payment business logic
type PaymentFlow interface {
	ChargeWallet(ctx context.Context, req *dto.ChargeWalletRequest, metadata *ClientMetadata) (*dto.ChargeWalletResponse, error)
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

type CallAtipayGetTokenRequest struct {
	Amount        uint64
	CellNumber    string
	Description   string
	InvoiceNumber string
	RedirectURL   string
}

// ChargeWallet handles the complete process of charging a wallet
func (p *PaymentFlowImpl) ChargeWallet(ctx context.Context, req *dto.ChargeWalletRequest, metadata *ClientMetadata) (*dto.ChargeWalletResponse, error) {
	// Validate business rules
	if err := p.validateChargeWalletRequest(ctx, req); err != nil {
		if errors.Is(err, ErrWalletNotFound) {
			// TODO
		}

		return nil, NewBusinessError("PAYMENT_VALIDATION_FAILED", "Payment validation failed", err)
	}

	// Use transaction for atomicity
	var paymentRequest *models.PaymentRequest
	var atipayToken string

	err := repository.WithTransaction(ctx, p.db, func(txCtx context.Context) error {
		customer, err := p.customerRepo.ByID(txCtx, req.CustomerID)
		if err != nil {
			return err
		}
		if customer == nil {
			return ErrCustomerNotFound
		}

		// Create payment request
		paymentRequest, err = p.createPaymentRequest(txCtx, *customer)
		if err != nil {
			return err
		}

		// Call Atipay to get token
		callAtipayGetTokenRequest := CallAtipayGetTokenRequest{
			Amount:        req.Amount,
			CellNumber:    customer.RepresentativeMobile,
			Description:   "charge wallet",
			InvoiceNumber: paymentRequest.InvoiceNumber,
			RedirectURL:   "",
		}
		atipayToken, err = p.callAtipayGetToken(txCtx, callAtipayGetTokenRequest, paymentRequest)
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
		errMsg := fmt.Sprintf("Charge wallet failed: %s", err.Error())
		_ = p.createAuditLog(ctx, nil, "charge_wallet_failed", errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("CHARGE_WALLET_FAILED", "Failed to charge wallet", err)
	}

	// Create success audit log
	msg := fmt.Sprintf("Charge wallet successfully: %s", paymentRequest.UUID)
	_ = p.createAuditLog(ctx, nil, "charge_wallet_success", msg, true, nil, metadata)

	// Build response
	response := &dto.ChargeWalletResponse{
		Message: "Charge wallet successfully",
		Success: true,
		Token:   atipayToken,
	}

	return response, nil
}

// validateChargeWalletRequest validates the business rules for charging a wallet
func (p *PaymentFlowImpl) validateChargeWalletRequest(ctx context.Context, req *dto.ChargeWalletRequest) error {
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

	return nil
}

// createPaymentRequest creates a new payment request record
func (p *PaymentFlowImpl) createPaymentRequest(ctx context.Context, customer models.Customer) (*models.PaymentRequest, error) {
	// Generate unique invoice number
	invoiceNumber := fmt.Sprintf("INV-%s-%d", uuid.New().String(), time.Now().Unix())

	// Set expiration time (30 minutes from now)
	expiresAt := time.Now().Add(30 * time.Minute)

	// Get current wallet balance
	currentBalance, err := p.walletRepo.GetCurrentBalance(ctx, customer.Wallet.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet balance: %w", err)
	}

	// Create payment request
	paymentRequest := &models.PaymentRequest{
		UUID:          uuid.New(),
		CorrelationID: uuid.New(),
		CustomerID:    customer.ID,
		WalletID:      customer.Wallet.ID,
		Amount:        currentBalance.FreeBalance,
		Currency:      "TMN",
		Description:   "charge wallet",
		InvoiceNumber: invoiceNumber,
		CellNumber:    customer.RepresentativeMobile,
		RedirectURL:   "",
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
func (p *PaymentFlowImpl) callAtipayGetToken(ctx context.Context, req CallAtipayGetTokenRequest, paymentRequest *models.PaymentRequest) (string, error) {
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
		return ErrCallbackRequestNil
	}
	if callback.ReservationNumber == "" {
		return ErrReservationNumberRequired
	}
	if callback.ReferenceNumber == "" {
		return ErrReferenceNumberRequired
	}
	if callback.Status == "" {
		return ErrStatusRequired
	}
	if callback.State == "" {
		return ErrStateRequired
	}
	return nil
}

// parseAtipayStatus parses Atipay status and state into meaningful values
func (p *PaymentFlowImpl) parseAtipayStatus(status, state string) (string, string) {
	// Return the raw values for now, can be enhanced with mapping if needed
	return status, state
}

// PaymentStatusMapping maps Atipay status codes to our payment statuses
type PaymentStatusMapping struct {
	Status      models.PaymentRequestStatus
	Success     bool
	Message     string
	Description string
}

// atipayStatusMappings defines the mapping from Atipay status/state to our payment statuses
var atipayStatusMappings = map[string]PaymentStatusMapping{
	"2_OK": {
		Status:      models.PaymentRequestStatusCompleted,
		Success:     true,
		Message:     "Payment completed successfully",
		Description: "Payment completed successfully via Atipay",
	},
	"1_CanceledByUser": {
		Status:      models.PaymentRequestStatusCancelled,
		Success:     false,
		Message:     "Payment cancelled by user",
		Description: "Payment cancelled by user via Atipay",
	},
	"3_Failed": {
		Status:      models.PaymentRequestStatusFailed,
		Success:     false,
		Message:     "Payment failed",
		Description: "Payment failed via Atipay",
	},
	"4_SessionIsNull": {
		Status:      models.PaymentRequestStatusExpired,
		Success:     false,
		Message:     "Payment session expired",
		Description: "Payment session expired via Atipay",
	},
	"5_InvalidParameters": {
		Status:      models.PaymentRequestStatusFailed,
		Success:     false,
		Message:     "Invalid payment parameters",
		Description: "Invalid payment parameters via Atipay",
	},
	"8_MerchantIpAddressIsInvalid": {
		Status:      models.PaymentRequestStatusFailed,
		Success:     false,
		Message:     "Merchant IP address invalid",
		Description: "Merchant IP address invalid via Atipay",
	},
	"10_TokenNotFound": {
		Status:      models.PaymentRequestStatusFailed,
		Success:     false,
		Message:     "Payment token not found",
		Description: "Payment token not found via Atipay",
	},
	"11_TokenRequired": {
		Status:      models.PaymentRequestStatusFailed,
		Success:     false,
		Message:     "Payment token required",
		Description: "Payment token required via Atipay",
	},
	"12_TerminalNotFound": {
		Status:      models.PaymentRequestStatusFailed,
		Success:     false,
		Message:     "Payment terminal not found",
		Description: "Payment terminal not found via Atipay",
	},
}

// getPaymentStatusMapping determines the payment status based on Atipay callback data
func (p *PaymentFlowImpl) getPaymentStatusMapping(status, state string) PaymentStatusMapping {
	// Try exact match first
	key := fmt.Sprintf("%s_%s", status, state)
	if mapping, exists := atipayStatusMappings[key]; exists {
		return mapping
	}

	// Try status-only match for some cases
	if mapping, exists := atipayStatusMappings[fmt.Sprintf("%s_", status)]; exists {
		return mapping
	}

	// Default to failed with unknown status
	return PaymentStatusMapping{
		Status:      models.PaymentRequestStatusFailed,
		Success:     false,
		Message:     fmt.Sprintf("Unknown payment status: %s, state: %s", status, state),
		Description: fmt.Sprintf("Unknown payment status: %s, state: %s via Atipay", status, state),
	}
}

// updatePaymentRequestWithCallback updates the payment request with callback data
func (p *PaymentFlowImpl) updatePaymentRequestWithCallback(paymentRequest *models.PaymentRequest, callback *dto.PaymentCallbackRequest, mapping PaymentStatusMapping) {
	paymentRequest.Status = mapping.Status
	paymentRequest.StatusReason = mapping.Description

	// Only update payment details for successful payments
	if mapping.Success {
		paymentRequest.PaymentReference = callback.ReferenceNumber
		paymentRequest.PaymentTrace = callback.TraceNumber
		paymentRequest.PaymentMaskedPAN = callback.MaskedPAN
		paymentRequest.PaymentRRN = callback.RRN
		paymentRequest.PaymentState = callback.State
		paymentRequest.PaymentStatus = callback.Status
	}
}

// HandlePaymentCallback handles the callback from Atipay after payment completion
func (p *PaymentFlowImpl) HandlePaymentCallback(ctx context.Context, callback *dto.PaymentCallbackRequest, metadata *ClientMetadata) (*dto.PaymentCallbackResponse, error) {
	// Validate callback data
	if err := p.validateCallbackRequest(callback); err != nil {
		return nil, fmt.Errorf("callback validation failed: %w", err)
	}

	var response *dto.PaymentCallbackResponse

	// Process callback within transaction
	err := repository.WithTransaction(ctx, p.db, func(txCtx context.Context) error {
		// Find the payment request by reservation number (our invoice number)
		paymentRequest, err := p.paymentRequestRepo.ByInvoiceNumber(txCtx, callback.ReservationNumber)
		if err != nil {
			return fmt.Errorf("failed to find payment request: %w", err)
		}
		if paymentRequest == nil {
			return NewBusinessError("PAYMENT_CALLBACK_FAILED", "Payment request not found for reservation number", nil)
		}

		// Check if this callback has already been processed
		if paymentRequest.Status != models.PaymentRequestStatusPending {
			response = &dto.PaymentCallbackResponse{
				Message: "Callback already processed",
				Success: true,
			}
			return nil
		}

		// Determine payment status based on callback
		mapping := p.getPaymentStatusMapping(callback.Status, callback.State)

		// Update payment request with callback data
		p.updatePaymentRequestWithCallback(paymentRequest, callback, mapping)

		// Save updated payment request
		if err := p.paymentRequestRepo.Save(txCtx, paymentRequest); err != nil {
			return fmt.Errorf("failed to update payment request status: %w", err)
		}

		// Create audit log
		if err := p.createPaymentCallbackAuditLog(txCtx, paymentRequest, mapping, metadata); err != nil {
			// Log error but don't fail the callback processing
			// Note: In production, you might want to log this to a proper logging system
		}

		// Set response
		response = &dto.PaymentCallbackResponse{
			Message:          mapping.Message,
			Success:          mapping.Success,
			PaymentRequestID: paymentRequest.ID,
			Status:           string(mapping.Status),
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("payment callback processing failed: %w", err)
	}

	return response, nil
}

// createPaymentCallbackAuditLog creates an audit log entry for payment callback processing
func (p *PaymentFlowImpl) createPaymentCallbackAuditLog(ctx context.Context, paymentRequest *models.PaymentRequest, mapping PaymentStatusMapping, metadata *ClientMetadata) error {
	// Find customer for audit logging
	customer, err := p.customerRepo.ByID(ctx, paymentRequest.CustomerID)
	if err != nil {
		return fmt.Errorf("failed to find customer for audit logging: %w", err)
	}

	// Determine audit action based on payment success
	auditAction := "payment_callback_processed"
	if mapping.Success {
		auditAction = "payment_completed"
	}

	// Create audit log
	return p.createAuditLog(ctx, customer, auditAction, mapping.Description, mapping.Success, nil, metadata)
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
