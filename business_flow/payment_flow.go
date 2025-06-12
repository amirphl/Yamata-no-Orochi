// Package businessflow contains the core business logic and use cases for payment workflows
package businessflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
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
	PaymentCallback(ctx context.Context, callback *dto.AtipayRequest, metadata *ClientMetadata) (string, error)
	GetTransactionHistory(ctx context.Context, req *dto.GetTransactionHistoryRequest, metadata *ClientMetadata) (*dto.TransactionHistoryResponse, error)
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
	atipayAPIKey, atipayTerminal string,
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
		atipayAPIKey:        atipayAPIKey,
		atipayTerminal:      atipayTerminal,
		domain:              domain,
	}
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
		if customer.IsActive != nil && !*customer.IsActive {
			return ErrAccountInactive
		}

		// Check if customer has a wallet, create one if it doesn't exist
		wallet, err := p.walletRepo.ByCustomerID(txCtx, req.CustomerID)
		if err != nil {
			return err
		}

		if wallet == nil {
			meta := map[string]any{
				"created_via": "charge_wallet",
				"created_at":  utils.UTCNow(),
			}
			b, err := json.Marshal(meta)
			if err != nil {
				return err
			}

			// Create new wallet for customer
			wallet = &models.Wallet{
				UUID:       uuid.New(),
				CustomerID: customer.ID,
				Metadata:   json.RawMessage(b),
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

		atipayToken, err = p.callAtipayGetToken(txCtx, *paymentRequest)
		if err != nil {
			return err
		}

		// Update payment request with Atipay token
		paymentRequest.AtipayToken = atipayToken
		paymentRequest.AtipayStatus = "OK"
		paymentRequest.Status = models.PaymentRequestStatusTokenized
		paymentRequest.StatusReason = "payment request tokenized successfully"
		paymentRequest.UpdatedAt = utils.UTCNow()

		if err := p.paymentRequestRepo.Update(txCtx, paymentRequest); err != nil {
			return err
		}

		paymentRequest.Status = models.PaymentRequestStatusPending
		paymentRequest.StatusReason = "payment request pending"
		paymentRequest.UpdatedAt = utils.UTCNow()
		if err := p.paymentRequestRepo.Update(txCtx, paymentRequest); err != nil {
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
	msg := fmt.Sprintf("Generated payment token for request %s", paymentRequest.UUID)
	_ = p.createAuditLog(ctx, customer, models.AuditActionWalletChargeCompleted, msg, true, nil, metadata)

	// Build response
	response := &dto.ChargeWalletResponse{
		Message: "Generated payment token successfully",
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

	// Validate amount (minimum 100000 Tomans and must be multiple of 10000)
	if req.Amount < 100000 {
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
		RedirectURL:   fmt.Sprintf("https://%s/api/v1/payments/callback", p.domain),
		AtipayToken:   "", // Will be set later
		AtipayStatus:  "", // Will be set later
		// Payment*: "",   // Will be set later
		Status:       models.PaymentRequestStatusCreated,
		StatusReason: "payment request created",
		ExpiresAt:    &expiresAt,
		Metadata:     json.RawMessage(`{"source": "wallet_recharge"}`),
	}

	// Save to database
	if err := p.paymentRequestRepo.Save(ctx, paymentRequest); err != nil {
		return nil, err
	}

	return paymentRequest, nil
}

// callAtipayGetToken calls Atipay's get-token API
func (p *PaymentFlowImpl) callAtipayGetToken(ctx context.Context, paymentRequest models.PaymentRequest) (string, error) {
	// Prepare Atipay request payload
	atipayPayload := map[string]any{
		"amount":        paymentRequest.Amount * 10, // TMN to IRR
		"cellNumber":    paymentRequest.CellNumber,
		"description":   paymentRequest.Description,
		"invoiceNumber": paymentRequest.InvoiceNumber,
		"redirectUrl":   paymentRequest.RedirectURL,
		"apiKey":        p.atipayAPIKey,
	}

	// Convert to JSON
	payloadBytes, err := json.Marshal(atipayPayload)
	if err != nil {
		return "", err
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://mipg.atipay.net/v1/get-token", bytes.NewReader(payloadBytes))
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
	if atipayResponse.Status != "200 OK" {
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
func (p *PaymentFlowImpl) validateCallbackRequest(callback *dto.AtipayRequest) error {
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

// updatePaymentRequest updates the payment request with callback data
func (p *PaymentFlowImpl) updatePaymentRequest(ctx context.Context, paymentRequest *models.PaymentRequest, atipayRequest *dto.AtipayRequest, mapping PaymentStatusMapping) error {
	metadata, _ := json.Marshal(map[string]any{
		"atipay_callback_status":             string(mapping.Status),
		"atipay_callback_status_message":     mapping.Message,
		"atipay_callback_status_description": mapping.Description,
		"atipay_callback_status_updated_at":  utils.UTCNow().Format(time.RFC3339),
		"atipay_callback_status_success":     strconv.FormatBool(mapping.Success),
	})

	paymentRequest.Metadata = metadata
	paymentRequest.Status = mapping.Status
	paymentRequest.StatusReason = mapping.Description
	paymentRequest.UpdatedAt = utils.UTCNow()

	// Only update payment details for successful payments
	if mapping.Success {
		paymentRequest.PaymentReference = atipayRequest.ReferenceNumber
		paymentRequest.PaymentReservation = atipayRequest.ReservationNumber
		paymentRequest.PaymentTerminal = atipayRequest.TerminalID
		paymentRequest.PaymentTrace = atipayRequest.TraceNumber
		paymentRequest.PaymentMaskedPAN = atipayRequest.MaskedPAN
		paymentRequest.PaymentRRN = atipayRequest.RRN
		paymentRequest.PaymentState = atipayRequest.State
		paymentRequest.PaymentStatus = atipayRequest.Status
	}

	// Save updated payment request
	if err := p.paymentRequestRepo.Update(ctx, paymentRequest); err != nil {
		return err
	}

	return nil
}

// PaymentCallback handles the callback from Atipay after payment completion
func (p *PaymentFlowImpl) PaymentCallback(ctx context.Context, atipayRequest *dto.AtipayRequest, metadata *ClientMetadata) (string, error) {
	// Validate callback data
	if err := p.validateCallbackRequest(atipayRequest); err != nil {
		return "", NewBusinessError("PAYMENT_CALLBACK_VALIDATION_FAILED", "Payment callback validation failed", err)
	}

	var customer *models.Customer
	var paymentRequest *models.PaymentRequest
	var mapping PaymentStatusMapping

	// Process callback within transaction
	err := repository.WithTransaction(ctx, p.db, func(txCtx context.Context) error {
		var err error

		// Find the payment request by reservation number (our invoice number)
		paymentRequest, err = p.paymentRequestRepo.ByInvoiceNumber(txCtx, atipayRequest.ReservationNumber)
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
		if customer.IsActive != nil && !*customer.IsActive {
			return ErrAccountInactive
		}

		// Check if this callback has already been processed
		if paymentRequest.Status != models.PaymentRequestStatusPending {
			return ErrPaymentRequestAlreadyProcessed
		}

		if paymentRequest.ExpiresAt != nil && paymentRequest.ExpiresAt.Before(utils.UTCNow()) {
			return ErrPaymentRequestExpired
		}

		// Determine payment status based on callback
		mapping = p.getPaymentStatusMapping(atipayRequest.Status, atipayRequest.State)

		// Update payment request with callback data
		if err := p.updatePaymentRequest(txCtx, paymentRequest, atipayRequest, mapping); err != nil {
			return err
		}

		// If payment was successful, increase user balance
		if mapping.Success {
			// Verify payment with Atipay before finalizing
			verificationResult, err := p.verifyPaymentWithAtipay(txCtx, atipayRequest.ReferenceNumber)
			if err != nil {
				// Failed but don't return error to avoid rollback
				mapping.Status = models.PaymentRequestStatusFailed
				mapping.Success = false
				mapping.Message = "Payment verification failed"
				mapping.Description = "Payment verification failed: " + err.Error()

				// Update payment request status
				if err := p.updatePaymentRequest(txCtx, paymentRequest, atipayRequest, mapping); err != nil {
					return err
				}

				// Create audit log for verification failure
				errMsg := fmt.Sprintf("Payment verification failed for request %s: %s", paymentRequest.UUID, err.Error())
				_ = p.createAuditLog(txCtx, customer, models.AuditActionPaymentFailed, mapping.Description, false, &errMsg, metadata)

				return nil
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
				if err := p.updatePaymentRequest(txCtx, paymentRequest, atipayRequest, mapping); err != nil {
					return err
				}

				// Create audit log for verification failure
				verificationFailedMsg := fmt.Sprintf("Payment verification failed for request %s: amount mismatch", paymentRequest.UUID)
				_ = p.createAuditLog(txCtx, customer, models.AuditActionPaymentFailed, mapping.Description, false, &verificationFailedMsg, metadata)

				return nil
			}

			// Verification successful - proceed with balance increase
			if err := p.increaseUserBalance(txCtx, paymentRequest, atipayRequest); err != nil {
				// Failed but don't return error to avoid rollback
				mapping.Status = models.PaymentRequestStatusFailed
				mapping.Success = false
				mapping.Message = "Increase user balance failed"
				mapping.Description = "Increase user balance failed: " + err.Error()

				// Update payment request status
				if err := p.updatePaymentRequest(txCtx, paymentRequest, atipayRequest, mapping); err != nil {
					return err
				}

				// Create audit log for increase user balance failure
				increaseUserBalanceFailedMsg := fmt.Sprintf("Increase user balance failed for request %s: %s", paymentRequest.UUID, err.Error())
				_ = p.createAuditLog(txCtx, customer, models.AuditActionPaymentFailed, mapping.Description, false, &increaseUserBalanceFailedMsg, metadata)

				return nil
			}

			// Create audit log for successful balance increase
			totalAmount := paymentRequest.Amount
			userAmount := uint64(float64(totalAmount) / (1 + utils.TaxRate))
			taxAmount := totalAmount - userAmount
			balanceIncreaseMsg := fmt.Sprintf("Wallet balance increased by %d Tomans (after %d Tomans tax) for payment request %s",
				userAmount, taxAmount, paymentRequest.UUID)
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
	htmlResponse, err := p.generatePaymentResultHTML(paymentRequest, atipayRequest, mapping)
	if err != nil {
		return "", NewBusinessError("PAYMENT_CALLBACK_HTML_GENERATION_FAILED", "Failed to generate HTML response", err)
	}

	return htmlResponse, nil
}

// increaseUserBalance increases the user's wallet balance when a payment is successful
// It splits the payment amount: 90% goes to user wallet, 10% goes to tax wallet
func (p *PaymentFlowImpl) increaseUserBalance(ctx context.Context, paymentRequest *models.PaymentRequest, atipayRequest *dto.AtipayRequest) error {
	// Calculate amounts after tax
	totalAmount := paymentRequest.Amount
	userAmount := uint64(float64(totalAmount) / (1 + utils.TaxRate))
	taxAmount := totalAmount - userAmount

	// Get current balance snapshot for user wallet
	currentBalance, err := p.walletRepo.GetCurrentBalance(ctx, paymentRequest.WalletID)
	if err != nil {
		return err
	}
	if currentBalance == nil {
		return ErrBalanceSnapshotNotFound
	}

	// Get tax wallet
	taxWallet, err := p.walletRepo.ByUUID(ctx, utils.TaxWalletUUID)
	if err != nil {
		return err
	}
	if taxWallet == nil {
		return ErrTaxWalletNotFound
	}

	// Get current tax wallet balance
	taxWalletBalance, err := p.walletRepo.GetCurrentBalance(ctx, taxWallet.ID)
	if err != nil {
		return err
	}
	if taxWalletBalance == nil {
		return ErrTaxWalletBalanceSnapshotNotFound
	}

	metadata, err := json.Marshal(map[string]any{
		"source":             "payment_callback",
		"payment_request_id": paymentRequest.ID,
		"amount":             userAmount,
		"tax_amount":         taxAmount,
		"total_payment":      totalAmount,
		"atipay_response":    atipayRequest,
	})
	if err != nil {
		return err
	}

	// Update user wallet balance
	newUserFreeBalance := currentBalance.FreeBalance + userAmount
	newUserBalanceSnapshot := &models.BalanceSnapshot{
		UUID:          uuid.New(),
		CorrelationID: paymentRequest.CorrelationID,
		WalletID:      paymentRequest.WalletID,
		CustomerID:    paymentRequest.CustomerID,
		FreeBalance:   newUserFreeBalance,
		FrozenBalance: currentBalance.FrozenBalance,
		LockedBalance: currentBalance.LockedBalance,
		TotalBalance:  newUserFreeBalance + currentBalance.FrozenBalance + currentBalance.LockedBalance,
		Reason:        "wallet_recharge",
		Description:   fmt.Sprintf("Wallet recharged with %d Tomans via Atipay (after %d Tomans tax)", userAmount, taxAmount),
		Metadata:      metadata,
	}

	// Save new user balance snapshot
	if err := p.balanceSnapshotRepo.Save(ctx, newUserBalanceSnapshot); err != nil {
		return err
	}

	// Update tax wallet balance
	newTaxFreeBalance := taxWalletBalance.FreeBalance + taxAmount
	newTaxBalanceSnapshot := &models.BalanceSnapshot{
		UUID:          uuid.New(),
		CorrelationID: paymentRequest.CorrelationID,
		WalletID:      taxWallet.ID,
		CustomerID:    taxWallet.CustomerID,
		FreeBalance:   newTaxFreeBalance,
		FrozenBalance: taxWalletBalance.FrozenBalance,
		LockedBalance: taxWalletBalance.LockedBalance,
		TotalBalance:  newTaxFreeBalance + taxWalletBalance.FrozenBalance + taxWalletBalance.LockedBalance,
		Reason:        "tax_collection",
		Description:   fmt.Sprintf("Tax collected: %d Tomans from payment request %s", taxAmount, paymentRequest.UUID),
		Metadata:      metadata,
	}

	// Save new tax balance snapshot
	if err := p.balanceSnapshotRepo.Save(ctx, newTaxBalanceSnapshot); err != nil {
		return err
	}

	balanceBefore, err := currentBalance.GetBalanceMap()
	if err != nil {
		return err
	}
	balanceAfter, err := newUserBalanceSnapshot.GetBalanceMap()
	if err != nil {
		return err
	}
	taxBalanceBefore, err := taxWalletBalance.GetBalanceMap()
	if err != nil {
		return err
	}
	taxBalanceAfter, err := newTaxBalanceSnapshot.GetBalanceMap()
	if err != nil {
		return err
	}

	// Create transaction record for user wallet
	userTransaction := &models.Transaction{
		UUID:              uuid.New(),
		CorrelationID:     paymentRequest.CorrelationID,
		Type:              models.TransactionTypeDeposit,
		Status:            models.TransactionStatusCompleted,
		Amount:            userAmount,
		Currency:          "TMN",
		WalletID:          paymentRequest.WalletID,
		CustomerID:        paymentRequest.CustomerID,
		BalanceBefore:     balanceBefore,
		BalanceAfter:      balanceAfter,
		ExternalReference: atipayRequest.ReferenceNumber,
		ExternalTrace:     atipayRequest.TraceNumber,
		ExternalRRN:       atipayRequest.RRN,
		ExternalMaskedPAN: atipayRequest.MaskedPAN,
		Description:       fmt.Sprintf("Wallet recharge: %d Tomans (after %d Tomans tax)", userAmount, taxAmount),
		Metadata:          metadata,
	}

	// Create transaction record for tax wallet
	taxTransaction := &models.Transaction{
		UUID:              uuid.New(),
		CorrelationID:     paymentRequest.CorrelationID,
		Type:              models.TransactionTypeDeposit,
		Status:            models.TransactionStatusCompleted,
		Amount:            taxAmount,
		Currency:          "TMN",
		WalletID:          taxWallet.ID,
		CustomerID:        taxWallet.CustomerID,
		BalanceBefore:     taxBalanceBefore,
		BalanceAfter:      taxBalanceAfter,
		ExternalReference: atipayRequest.ReferenceNumber,
		ExternalTrace:     atipayRequest.TraceNumber,
		ExternalRRN:       atipayRequest.RRN,
		ExternalMaskedPAN: atipayRequest.MaskedPAN,
		Description:       fmt.Sprintf("Tax collection: %d Tomans from payment request %s", taxAmount, paymentRequest.UUID),
		Metadata:          metadata,
	}

	// Save both transactions
	if err := p.transactionRepo.Save(ctx, userTransaction); err != nil {
		return err
	}
	if err := p.transactionRepo.Save(ctx, taxTransaction); err != nil {
		return err
	}

	return nil
}

// generatePaymentResultHTML generates HTML response based on payment status
func (p *PaymentFlowImpl) generatePaymentResultHTML(paymentRequest *models.PaymentRequest, callback *dto.AtipayRequest, mapping PaymentStatusMapping) (string, error) {
	// Read template files
	var templateContent string
	var err error

	if mapping.Success {
		templateContent, err = p.readTemplate("templates/payment_success.html")
	} else {
		templateContent, err = p.readTemplate("templates/payment_failure.html")
	}

	if err != nil {
		return "", err
	}

	// Calculate tax amounts
	totalAmount := paymentRequest.Amount
	userAmount := uint64(float64(totalAmount) / (1 + utils.TaxRate))
	taxAmount := totalAmount - userAmount

	// Prepare template data
	data := map[string]any{
		"Status":          mapping.Status,
		"Message":         mapping.Message,
		"TotalAmount":     totalAmount,
		"TaxAmount":       taxAmount,
		"NetAmount":       userAmount,
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
		return "", err
	}
	return string(content), nil
}

// AtipayVerificationResponse represents the response from Atipay's verify-payment API
type AtipayVerificationResponse struct {
	Amount uint64 `json:"amount"` // Amount in Rials
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
		return nil, err
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://mipg.atipay.net/v1/verify-payment", bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Make HTTP request
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("atipay verification API returned non-OK status: %d", resp.StatusCode)
	}

	// Parse response body
	var atipayVerificationResponse AtipayVerificationResponse
	if err := json.NewDecoder(resp.Body).Decode(&atipayVerificationResponse); err != nil {
		return nil, err
	}

	return &atipayVerificationResponse, nil
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

	err := p.auditRepo.Save(ctx, auditLog)
	if err != nil {
		return err
	}
	return nil
}

// GetTransactionHistory retrieves the transaction history for a customer with pagination and filtering
func (p *PaymentFlowImpl) GetTransactionHistory(ctx context.Context, req *dto.GetTransactionHistoryRequest, metadata *ClientMetadata) (response *dto.TransactionHistoryResponse, err error) {
	defer func() {
		if err != nil {
			err = NewBusinessError("GET_TRANSACTION_HISTORY_FAILED", "Get transaction history failed", err)
		}
	}()

	// Validate business rules
	if err := p.validateGetTransactionHistoryRequest(req); err != nil {
		return nil, err
	}

	// Get customer to verify they exist and are active
	customer, err := p.customerRepo.ByID(ctx, req.CustomerID)
	if err != nil {
		return nil, err
	}
	if customer == nil {
		return nil, ErrCustomerNotFound
	}
	if customer.IsActive != nil && !*customer.IsActive {
		return nil, ErrAccountInactive
	}

	// Get customer's wallet
	wallet, err := p.walletRepo.ByCustomerID(ctx, req.CustomerID)
	if err != nil {
		return nil, err
	}
	if wallet == nil {
		return nil, ErrWalletNotFound
	}

	// Calculate offset for pagination
	offset := (req.Page - 1) * req.PageSize

	filter := models.TransactionFilter{
		WalletID:      &wallet.ID,
		CustomerID:    &customer.ID,
		CreatedAfter:  req.StartDate,
		CreatedBefore: req.EndDate,
	}
	if req.Type != nil {
		filter.Type = utils.ToPtr(models.TransactionType(*req.Type))
	}
	if req.Status != nil {
		filter.Status = utils.ToPtr(models.TransactionStatus(*req.Status))
	}

	// Get transactions with pagination using available methods
	transactions, err := p.transactionRepo.ByFilter(ctx, filter, "created_at DESC", int(req.PageSize), int(offset))
	if err != nil {
		return nil, err
	}

	filter = models.TransactionFilter{
		WalletID:   &wallet.ID,
		CustomerID: &customer.ID,
	}
	totalCount, err := p.transactionRepo.Count(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Convert transactions to transaction history items
	items := make([]dto.TransactionHistoryItem, 0)
	for _, transaction := range transactions {
		item, err := p.convertTransactionToTransactionHistoryItem(transaction)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	// Calculate pagination info
	pagination := p.calculatePaginationInfo(req.Page, req.PageSize, uint(totalCount))

	// Create audit log for successful retrieval
	msg := fmt.Sprintf("Transaction history retrieved: %d items for customer %d", len(items), req.CustomerID)
	_ = p.createAuditLog(ctx, customer, models.AuditActionTransactionHistoryRetrieved, msg, true, nil, metadata)

	response = &dto.TransactionHistoryResponse{
		Items:      items,
		Pagination: pagination,
	}

	return response, nil
}

// validateGetTransactionHistoryRequest validates the transaction history request
func (p *PaymentFlowImpl) validateGetTransactionHistoryRequest(req *dto.GetTransactionHistoryRequest) error {
	if req.CustomerID == 0 {
		return ErrCustomerNotFound
	}
	if req.Page < 1 {
		return ErrInvalidPage
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		return ErrInvalidPageSize
	}
	if req.StartDate != nil && req.EndDate != nil && req.StartDate.After(*req.EndDate) {
		return ErrStartDateAfterEndDate
	}
	return nil
}

// convertTransactionToTransactionHistoryItem converts a transaction model to a transaction history item DTO
func (p *PaymentFlowImpl) convertTransactionToTransactionHistoryItem(transaction *models.Transaction) (dto.TransactionHistoryItem, error) {
	// Get human-readable operation name
	operation := dto.TransactionTypeDisplay[transaction.Type]
	if operation == "" {
		operation = string(transaction.Type)
	}

	// Get human-readable status
	status := dto.TransactionStatusDisplay[transaction.Status]
	if status == "" {
		status = string(transaction.Status)
	}

	// Prepare external reference
	var externalRef *string
	if transaction.ExternalReference != "" {
		externalRef = &transaction.ExternalReference
	}

	var balanceBefore map[string]uint64
	var balanceAfter map[string]uint64

	err := json.Unmarshal(transaction.BalanceBefore, &balanceBefore)
	if err != nil {
		return dto.TransactionHistoryItem{}, err
	}
	err = json.Unmarshal(transaction.BalanceAfter, &balanceAfter)
	if err != nil {
		return dto.TransactionHistoryItem{}, err
	}

	return dto.TransactionHistoryItem{
		UUID:          transaction.UUID.String(),
		Status:        status,
		Amount:        transaction.Amount,
		Currency:      transaction.Currency,
		Operation:     operation,
		DateTime:      transaction.CreatedAt,
		ExternalRef:   externalRef,
		BalanceBefore: balanceBefore,
		BalanceAfter:  balanceAfter,
		Metadata:      nil,
	}, nil
}

// calculatePaginationInfo calculates pagination metadata
func (p *PaymentFlowImpl) calculatePaginationInfo(page, pageSize, totalItems uint) dto.TransactionHistoryPaginationInfo {
	totalPages := (totalItems + pageSize - 1) / pageSize // Ceiling division

	return dto.TransactionHistoryPaginationInfo{
		CurrentPage: page,
		PageSize:    pageSize,
		TotalItems:  totalItems,
		TotalPages:  totalPages,
		HasNext:     page < totalPages,
		HasPrevious: page > 1,
	}
}
