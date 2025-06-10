// Package businessflow contains the core business logic and use cases for payment workflows
package businessflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PaymentFlow handles the complete payment business logic
type PaymentFlow interface {
	ChargeWallet(ctx context.Context, req *dto.ChargeWalletRequest, metadata *ClientMetadata) (*dto.ChargeWalletResponse, error)
	PaymentCallback(ctx context.Context, callback *dto.PaymentCallbackRequest, metadata *ClientMetadata) (string, error)
}

// PaymentFlowImpl implements the payment business flow
type PaymentFlowImpl struct {
	paymentRequestRepo  repository.PaymentRequestRepository
	walletRepo          repository.WalletRepository
	customerRepo        repository.CustomerRepository
	auditRepo           repository.AuditLogRepository
	balanceSnapshotRepo repository.BalanceSnapshotRepository
	transactionRepo     repository.TransactionRepository
	db                  *gorm.DB

	// Atipay configuration
	atipayBaseURL  string
	atipayAPIKey   string
	atipayTerminal string

	// Domain configuration for redirect URLs
	domain string
}

// NewPaymentFlow creates a new payment flow instance
func NewPaymentFlow(
	paymentRequestRepo repository.PaymentRequestRepository,
	walletRepo repository.WalletRepository,
	customerRepo repository.CustomerRepository,
	auditRepo repository.AuditLogRepository,
	balanceSnapshotRepo repository.BalanceSnapshotRepository,
	transactionRepo repository.TransactionRepository,
	db *gorm.DB,
	atipayBaseURL, atipayAPIKey, atipayTerminal string,
	domain string,
) PaymentFlow {
	return &PaymentFlowImpl{
		paymentRequestRepo:  paymentRequestRepo,
		walletRepo:          walletRepo,
		customerRepo:        customerRepo,
		auditRepo:           auditRepo,
		balanceSnapshotRepo: balanceSnapshotRepo,
		transactionRepo:     transactionRepo,
		db:                  db,
		atipayBaseURL:       atipayBaseURL,
		atipayAPIKey:        atipayAPIKey,
		atipayTerminal:      atipayTerminal,
		domain:              domain,
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
		return nil, NewBusinessError("CHARGE_WALLET_FAILED", "Charge wallet failed", err)
	}

	var customer *models.Customer
	// Use transaction for atomicity
	var paymentRequest *models.PaymentRequest
	var atipayToken string

	err := repository.WithTransaction(ctx, p.db, func(txCtx context.Context) error {
		var err error
		customer, err = p.customerRepo.ByID(txCtx, req.CustomerID)
		if err != nil {
			return err
		}
		if customer == nil {
			return ErrCustomerNotFound
		}

		// Check if customer has a wallet, create one if it doesn't exist
		wallet, err := p.walletRepo.ByCustomerID(txCtx, req.CustomerID)
		if err != nil {
			return err
		}

		if wallet == nil {
			// Create new wallet for customer
			wallet = &models.Wallet{
				UUID:       uuid.New(),
				CustomerID: customer.ID,
				Metadata: map[string]any{
					"created_via": "charge_wallet",
					"created_at":  utils.UTCNow().Format(time.RFC3339),
				},
			}

			if err := p.walletRepo.SaveWithInitialSnapshot(txCtx, wallet); err != nil {
				return err
			}

			// Create audit log for wallet creation
			walletCreatedMsg := fmt.Sprintf("Wallet created for customer %d", customer.ID)
			_ = p.createAuditLog(txCtx, customer, models.AuditActionWalletCreated, walletCreatedMsg, true, nil, metadata)

			// Update customer with wallet reference
			customer.Wallet = wallet
		}

		// Create payment request
		paymentRequest, err = p.createPaymentRequest(txCtx, *customer, req.Amount)
		if err != nil {
			return err
		}

		// Call Atipay to get token
		atipayRequest := CallAtipayGetTokenRequest{
			Amount:        req.Amount,
			CellNumber:    customer.RepresentativeMobile,
			Description:   paymentRequest.Description,
			InvoiceNumber: paymentRequest.InvoiceNumber,
			RedirectURL:   paymentRequest.RedirectURL,
		}
		atipayToken, err = p.callAtipayGetToken(txCtx, atipayRequest, *paymentRequest)
		if err != nil {
			return err
		}

		// Update payment request with Atipay token
		paymentRequest.AtipayToken = atipayToken
		paymentRequest.AtipayStatus = "OK"
		paymentRequest.Status = models.PaymentRequestStatusTokenized
		paymentRequest.StatusReason = "payment request tokenized successfully"
		paymentRequest.UpdatedAt = utils.UTCNow()
		paymentRequest.Metadata["atipay_token"] = atipayToken
		paymentRequest.Metadata["atipay_status"] = "OK"
		paymentRequest.Metadata["atipay_status_reason"] = "payment request tokenized successfully"
		paymentRequest.Metadata["atipay_status_updated_at"] = utils.UTCNow().Format(time.RFC3339)

		if err := p.paymentRequestRepo.Save(txCtx, paymentRequest); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Charge wallet failed: %s", err.Error())
		_ = p.createAuditLog(ctx, customer, models.AuditActionWalletChargeFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("CHARGE_WALLET_FAILED", "Failed to charge wallet", err)
	}

	// Create success audit log
	msg := fmt.Sprintf("Charge wallet successfully: %s", paymentRequest.UUID)
	_ = p.createAuditLog(ctx, customer, models.AuditActionWalletChargeCompleted, msg, true, nil, metadata)

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
func (p *PaymentFlowImpl) createPaymentRequest(ctx context.Context, customer models.Customer, amount uint64) (*models.PaymentRequest, error) {
	// Generate unique invoice number
	invoiceNumber := fmt.Sprintf("INV-%s-%d", uuid.New().String(), time.Now().Unix())

	// Set expiration time (30 minutes from now)
	expiresAt := time.Now().Add(30 * time.Minute)

	// Create payment request
	paymentRequest := &models.PaymentRequest{
		UUID:          uuid.New(),
		CorrelationID: uuid.New(),
		CustomerID:    customer.ID,
		WalletID:      customer.Wallet.ID,
		Amount:        amount,
		Currency:      "TMN",
		Description:   "charge wallet",
		InvoiceNumber: invoiceNumber,
		CellNumber:    customer.RepresentativeMobile,
		RedirectURL:   fmt.Sprintf("https://%s/api/v1/payment-result", p.domain),
		AtipayToken:   "",
		AtipayStatus:  "",
		Status:        models.PaymentRequestStatusCreated,
		StatusReason:  "payment request created",
		ExpiresAt:     &expiresAt,
		Metadata: map[string]any{
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
func (p *PaymentFlowImpl) callAtipayGetToken(ctx context.Context, atipayRequest CallAtipayGetTokenRequest, paymentRequest models.PaymentRequest) (string, error) {
	// Prepare Atipay request payload
	atipayPayload := map[string]any{
		"amount":        atipayRequest.Amount * 10, // TMN to IRR
		"cellNumber":    atipayRequest.CellNumber,
		"description":   atipayRequest.Description,
		"invoiceNumber": paymentRequest.InvoiceNumber,
		"redirectUrl":   atipayRequest.RedirectURL,
		"apiKey":        p.atipayAPIKey,
	}

	// Convert to JSON
	payloadBytes, err := json.Marshal(atipayPayload)
	if err != nil {
		return "", err
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/v1/get-token", p.atipayBaseURL)
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
	paymentRequest.UpdatedAt = utils.UTCNow()
	paymentRequest.Metadata["atipay_callback_status"] = mapping.Status
	paymentRequest.Metadata["atipay_callback_status_message"] = mapping.Message
	paymentRequest.Metadata["atipay_callback_status_description"] = mapping.Description
	paymentRequest.Metadata["atipay_callback_status_updated_at"] = utils.UTCNow().Format(time.RFC3339)
	paymentRequest.Metadata["atipay_callback_status_success"] = mapping.Success

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

// VerifyPaymentRequest represents the request to verify payment with Atipay
type VerifyPaymentRequest struct {
	ReferenceNumber string `json:"reference_number" validate:"required"`
}

// VerifyPaymentResponse represents the response after payment verification
type VerifyPaymentResponse struct {
	Message         string    `json:"message"`
	Success         bool      `json:"success"`
	Amount          uint64    `json:"amount"`
	ReferenceNumber string    `json:"reference_number"`
	VerifiedAt      time.Time `json:"verified_at"`
	Status          string    `json:"status"`
}

// PaymentCallback handles the callback from Atipay after payment completion
func (p *PaymentFlowImpl) PaymentCallback(ctx context.Context, callback *dto.PaymentCallbackRequest, metadata *ClientMetadata) (string, error) {
	// Validate callback data
	if err := p.validateCallbackRequest(callback); err != nil {
		return "", NewBusinessError("PAYMENT_CALLBACK_VALIDATION_FAILED", "Payment callback validation failed", err)
	}

	var customer *models.Customer
	var paymentRequest *models.PaymentRequest
	var mapping PaymentStatusMapping

	// Process callback within transaction
	err := repository.WithTransaction(ctx, p.db, func(txCtx context.Context) error {
		var err error

		// Find the payment request by reservation number (our invoice number)
		paymentRequest, err = p.paymentRequestRepo.ByInvoiceNumber(txCtx, callback.ReservationNumber)
		if err != nil {
			return err
		}
		if paymentRequest == nil {
			return ErrPaymentRequestNotFound
		}

		customer, err = p.customerRepo.ByID(txCtx, paymentRequest.CustomerID)
		if err != nil {
			return err
		}
		if customer == nil {
			return ErrCustomerNotFound
		}

		// Check if this callback has already been processed
		if paymentRequest.Status != models.PaymentRequestStatusPending {
			return ErrPaymentRequestAlreadyProcessed
		}

		// Determine payment status based on callback
		mapping = p.getPaymentStatusMapping(callback.Status, callback.State)

		// TODO: Check expiration time

		// Update payment request with callback data
		p.updatePaymentRequestWithCallback(paymentRequest, callback, mapping)

		// Save updated payment request
		if err := p.paymentRequestRepo.Save(txCtx, paymentRequest); err != nil {
			return err
		}

		// Create audit log for callback processing
		if err := p.createPaymentCallbackAuditLog(txCtx, paymentRequest, mapping, metadata); err != nil {
			return err
		}

		// If payment was successful, increase user balance
		if mapping.Success {
			// Verify payment with Atipay before finalizing
			verificationResult, err := p.verifyPaymentWithAtipay(txCtx, callback.ReferenceNumber)
			if err != nil {
				return err
			}

			// Check if verified amount matches the original amount
			if verificationResult.Amount != paymentRequest.Amount*10 { // Convert Tomans to Rials
				// Amount mismatch - mark payment as failed and refund will occur
				mapping.Status = models.PaymentRequestStatusFailed
				mapping.Success = false
				mapping.Message = "Payment verification failed: amount mismatch"
				mapping.Description = fmt.Sprintf("Verified amount (%d Rials) does not match original amount (%d Rials)",
					verificationResult.Amount, paymentRequest.Amount*10)

				// Update payment request status
				p.updatePaymentRequestWithCallback(paymentRequest, callback, mapping)

				// Create audit log for verification failure
				verificationFailedMsg := fmt.Sprintf("Payment verification failed for request %s: amount mismatch", paymentRequest.UUID)
				_ = p.createAuditLog(txCtx, customer, models.AuditActionPaymentFailed, verificationFailedMsg, false, nil, metadata)

				return nil
			}

			// Verification successful - proceed with balance increase
			if err := p.increaseUserBalance(txCtx, paymentRequest, callback); err != nil {
				return err
			}

			// Create audit log for successful balance increase
			balanceIncreaseMsg := fmt.Sprintf("Wallet balance increased by %d Tomans for payment request %s",
				paymentRequest.Amount, paymentRequest.UUID)
			_ = p.createAuditLog(txCtx, customer, models.AuditActionWalletChargeCompleted, balanceIncreaseMsg, true, nil, metadata)
		}

		return nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Payment callback failed: %s", err.Error())
		_ = p.createAuditLog(ctx, customer, models.AuditActionPaymentCallbackProcessed, errMsg, false, &errMsg, metadata)

		return "", NewBusinessError("PAYMENT_CALLBACK_FAILED", "Payment callback failed", err)
	}

	msg := fmt.Sprintf("Payment callback processed: %s", paymentRequest.UUID)
	_ = p.createAuditLog(ctx, customer, models.AuditActionPaymentCallbackProcessed, msg, true, nil, metadata)

	// Generate HTML response based on payment status
	htmlResponse, err := p.generatePaymentResultHTML(paymentRequest, callback, mapping)
	if err != nil {
		return "", NewBusinessError("PAYMENT_CALLBACK_HTML_GENERATION_FAILED", "Failed to generate HTML response", err)
	}

	return htmlResponse, nil
}

// createPaymentCallbackAuditLog creates an audit log entry for payment callback processing
func (p *PaymentFlowImpl) createPaymentCallbackAuditLog(ctx context.Context, paymentRequest *models.PaymentRequest, mapping PaymentStatusMapping, metadata *ClientMetadata) error {
	// Find customer for audit logging
	customer, err := p.customerRepo.ByID(ctx, paymentRequest.CustomerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return ErrCustomerNotFound
	}

	// Determine audit action based on payment success
	var auditAction string
	switch mapping.Status {
	case models.PaymentRequestStatusCompleted:
		auditAction = models.AuditActionPaymentCompleted
	case models.PaymentRequestStatusFailed:
		auditAction = models.AuditActionPaymentFailed
	case models.PaymentRequestStatusCancelled:
		auditAction = models.AuditActionPaymentCancelled
	case models.PaymentRequestStatusExpired:
		auditAction = models.AuditActionPaymentExpired
	default:
		auditAction = models.AuditActionPaymentCallbackProcessed
	}

	// Create audit log
	return p.createAuditLog(ctx, customer, auditAction, mapping.Description, mapping.Success, nil, metadata)
}

// createAuditLog creates an audit log entry
func (p *PaymentFlowImpl) createAuditLog(ctx context.Context, customer *models.Customer, action string, description string, success bool, errorDetails *string, metadata *ClientMetadata) error {
	var customerID *uint
	if customer != nil {
		customerID = &customer.ID
	}

	ipAddress := "127.0.0.1"
	userAgent := ""
	if metadata != nil {
		ipAddress = metadata.IPAddress
		userAgent = metadata.UserAgent
	}

	auditLog := &models.AuditLog{
		CustomerID:   customerID,
		Action:       action,
		Description:  &description,
		Success:      &success,
		IPAddress:    &ipAddress,
		UserAgent:    &userAgent,
		Metadata:     json.RawMessage(`{"source": "payment_flow"}`),
		ErrorMessage: errorDetails,
	}

	if errorDetails != nil {
		// Create metadata with error details
		metadataMap := map[string]any{
			"source":        "payment_flow",
			"error_details": *errorDetails,
		}
		metadataBytes, _ := json.Marshal(metadataMap)
		auditLog.Metadata = metadataBytes
	}

	// Extract request ID from context if available
	requestID := ctx.Value(RequestIDKey)
	if requestID != nil {
		requestIDStr, ok := requestID.(string)
		if ok {
			auditLog.RequestID = &requestIDStr
		}
	}

	return p.auditRepo.Save(ctx, auditLog)
}

// increaseUserBalance increases the user's wallet balance when a payment is successful
func (p *PaymentFlowImpl) increaseUserBalance(ctx context.Context, paymentRequest *models.PaymentRequest, callbackRequest *dto.PaymentCallbackRequest) error {
	// Get current balance snapshot
	currentBalance, err := p.walletRepo.GetCurrentBalance(ctx, paymentRequest.WalletID)
	if err != nil {
		return err
	}
	if currentBalance == nil {
		return ErrBalanceSnapshotNotFound
	}

	newFreeBalance := currentBalance.FreeBalance + paymentRequest.Amount

	// Create new balance snapshot
	newBalanceSnapshot := &models.BalanceSnapshot{
		UUID:          uuid.New(),
		CorrelationID: paymentRequest.CorrelationID,
		WalletID:      paymentRequest.WalletID,
		CustomerID:    paymentRequest.CustomerID,
		FreeBalance:   newFreeBalance,
		FrozenBalance: currentBalance.FrozenBalance,
		LockedBalance: currentBalance.LockedBalance,
		TotalBalance:  newFreeBalance + currentBalance.FrozenBalance + currentBalance.LockedBalance,
		Reason:        "wallet_recharge",
		Description:   fmt.Sprintf("Wallet recharged with %d Tomans via Atipay", paymentRequest.Amount),
		Metadata: map[string]any{
			"source":             "payment_callback",
			"payment_request_id": paymentRequest.ID,
			"payment_reference":  callbackRequest.ReferenceNumber,
			"payment_trace":      callbackRequest.TraceNumber,
			"previous_balance":   currentBalance.FreeBalance,
			"recharge_amount":    paymentRequest.Amount,
			"new_balance":        newFreeBalance,
		},
	}

	// Save new balance snapshot
	if err := p.balanceSnapshotRepo.Save(ctx, newBalanceSnapshot); err != nil {
		return err
	}

	// Create transaction record
	transaction := &models.Transaction{
		UUID:              uuid.New(),
		CorrelationID:     paymentRequest.CorrelationID,
		Type:              models.TransactionTypeDeposit,
		Status:            models.TransactionStatusCompleted,
		Amount:            paymentRequest.Amount,
		Currency:          "TMN",
		WalletID:          paymentRequest.WalletID,
		CustomerID:        paymentRequest.CustomerID,
		BalanceBefore:     currentBalance.GetBalanceMap(),
		BalanceAfter:      newBalanceSnapshot.GetBalanceMap(),
		ExternalReference: callbackRequest.ReferenceNumber,
		ExternalTrace:     callbackRequest.TraceNumber,
		ExternalRRN:       callbackRequest.RRN,
		ExternalMaskedPAN: callbackRequest.MaskedPAN,
		Description:       fmt.Sprintf("Wallet recharge: %d Tomans", paymentRequest.Amount),
		Metadata: map[string]any{
			"source":             "payment_callback",
			"payment_request_id": paymentRequest.ID,
			"amount_tomans":      paymentRequest.Amount,
		},
	}

	// Save transaction
	if err := p.transactionRepo.Save(ctx, transaction); err != nil {
		return err
	}

	return nil
}

// generatePaymentResultHTML generates HTML response based on payment status
func (p *PaymentFlowImpl) generatePaymentResultHTML(paymentRequest *models.PaymentRequest, callback *dto.PaymentCallbackRequest, mapping PaymentStatusMapping) (string, error) {
	// Read template files
	var templateContent string
	var err error

	if mapping.Success {
		templateContent, err = p.readTemplate("templates/payment_success.html")
	} else {
		templateContent, err = p.readTemplate("templates/payment_failure.html")
	}

	if err != nil {
		return "", fmt.Errorf("failed to read template: %w", err)
	}

	// Prepare template data
	data := map[string]any{
		"Status":          mapping.Status,
		"Message":         mapping.Message,
		"Description":     mapping.Description,
		"Amount":          paymentRequest.Amount,
		"ReferenceNumber": callback.ReferenceNumber,
		"TraceNumber":     callback.TraceNumber,
		"RRN":             callback.RRN,
		"MaskedPAN":       callback.MaskedPAN,
		"ProcessedAt":     utils.UTCNow().Format("2006-01-02 15:04:05"),
	}

	// Simple template replacement (you could use a proper template engine like html/template)
	html := templateContent
	for key, value := range data {
		if value != nil {
			placeholder := "{{." + key + "}}"
			html = strings.ReplaceAll(html, placeholder, fmt.Sprintf("%v", value))
		}
	}

	return html, nil
}

// readTemplate reads a template file from the filesystem
func (p *PaymentFlowImpl) readTemplate(filename string) (string, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to read template file %s: %w", filename, err)
	}
	return string(content), nil
}

// verifyPaymentWithAtipay calls Atipay's verify-payment API to finalize the transaction
func (p *PaymentFlowImpl) verifyPaymentWithAtipay(ctx context.Context, referenceNumber string) (*AtipayVerificationResponse, error) {
	// Prepare Atipay verification request payload
	verificationPayload := map[string]any{
		"referenceNumber": referenceNumber,
		"apiKey":          p.atipayAPIKey,
	}

	// Convert to JSON
	payloadBytes, err := json.Marshal(verificationPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal verification payload: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/v1/verify-payment", p.atipayBaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create verification request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second, // Longer timeout for verification
	}

	// Make HTTP request
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call Atipay verification API: %w", err)
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("atipay verification API returned non-OK status: %d", resp.StatusCode)
	}

	// Parse response body
	var atipayResponse AtipayVerificationResponse
	if err := json.NewDecoder(resp.Body).Decode(&atipayResponse); err != nil {
		return nil, fmt.Errorf("failed to decode verification response: %w", err)
	}

	return &atipayResponse, nil
}

// AtipayVerificationResponse represents the response from Atipay's verify-payment API
type AtipayVerificationResponse struct {
	Amount uint64 `json:"amount"` // Amount in Rials
}
