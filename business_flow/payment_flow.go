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
	"github.com/amirphl/Yamata-no-Orochi/config"
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
	GetWalletBalance(ctx context.Context, req *dto.GetWalletBalanceRequest, metadata *ClientMetadata) (*dto.GetWalletBalanceResponse, error)
}

// PaymentFlowImpl implements the payment business flow
type PaymentFlowImpl struct {
	paymentRequestRepo  repository.PaymentRequestRepository
	walletRepo          repository.WalletRepository
	customerRepo        repository.CustomerRepository
	auditRepo           repository.AuditLogRepository
	balanceSnapshotRepo repository.BalanceSnapshotRepository
	transactionRepo     repository.TransactionRepository
	agencyDiscountRepo  repository.AgencyDiscountRepository
	db                  *gorm.DB

	// Atipay configuration
	atipayCfg     config.AtipayConfig
	sysCfg        config.SystemConfig
	deploymentCfg config.DeploymentConfig
}

// NewPaymentFlow creates a new payment flow instance
func NewPaymentFlow(
	paymentRequestRepo repository.PaymentRequestRepository,
	walletRepo repository.WalletRepository,
	customerRepo repository.CustomerRepository,
	auditRepo repository.AuditLogRepository,
	balanceSnapshotRepo repository.BalanceSnapshotRepository,
	transactionRepo repository.TransactionRepository,
	agencyDiscountRepo repository.AgencyDiscountRepository,
	db *gorm.DB,
	atipayCfg config.AtipayConfig,
	sysCfg config.SystemConfig,
	deploymentCfg config.DeploymentConfig,
) PaymentFlow {
	return &PaymentFlowImpl{
		paymentRequestRepo:  paymentRequestRepo,
		walletRepo:          walletRepo,
		customerRepo:        customerRepo,
		auditRepo:           auditRepo,
		balanceSnapshotRepo: balanceSnapshotRepo,
		transactionRepo:     transactionRepo,
		agencyDiscountRepo:  agencyDiscountRepo,
		db:                  db,
		atipayCfg:           atipayCfg,
		sysCfg:              sysCfg,
		deploymentCfg:       deploymentCfg,
	}
}

// ChargeWallet handles the complete process of charging a wallet
func (p *PaymentFlowImpl) ChargeWallet(ctx context.Context, req *dto.ChargeWalletRequest, metadata *ClientMetadata) (*dto.ChargeWalletResponse, error) {
	// Validate business rules
	if err := p.validateChargeWalletRequest(req); err != nil {
		return nil, NewBusinessError("CHARGE_WALLET_FAILED", "Charge wallet failed", err)
	}

	var customer models.Customer
	var paymentRequest *models.PaymentRequest
	var atipayToken string

	err := repository.WithTransaction(ctx, p.db, func(txCtx context.Context) error {
		var err error
		customer, err = getCustomer(txCtx, p.customerRepo, req.CustomerID)
		if err != nil {
			return err
		}

		// Check if customer has a wallet, create one if it doesn't exist
		wallet, err := p.walletRepo.ByCustomerID(txCtx, customer.ID)
		if err != nil {
			return err
		}
		// Update customer with wallet reference
		customer.Wallet = wallet

		// Create payment request
		paymentRequest, err = p.createPaymentRequest(txCtx, customer, req.AmountWithTax)
		if err != nil {
			return err
		}

		atipayToken, err = p.callAtipayGetToken(txCtx, customer, *paymentRequest)
		if err != nil {
			return err
		}

		// Update payment request with Atipay token
		// State: Tokenized -> Pending
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
		errMsg := fmt.Sprintf("Charge wallet failed for customer %d: %s", customer.ID, err.Error())
		_ = createAuditLog(ctx, p.auditRepo, &customer, models.AuditActionWalletChargeFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("CHARGE_WALLET_FAILED", "Failed to charge wallet", err)
	}

	// Create success audit log
	msg := fmt.Sprintf("Generated payment token for payment request %d for customer %d", paymentRequest.ID, customer.ID)
	_ = createAuditLog(ctx, p.auditRepo, &customer, models.AuditActionWalletChargeCompleted, msg, true, nil, metadata)

	// Build resp
	resp := &dto.ChargeWalletResponse{
		Message: "Generated payment token successfully",
		Success: true,
		Token:   atipayToken,
	}

	return resp, nil
}

// validateChargeWalletRequest validates the business rules for charging a wallet
func (p *PaymentFlowImpl) validateChargeWalletRequest(req *dto.ChargeWalletRequest) error {
	// Validate amount (minimum 1000 Tomans and must be multiple of 1000)
	if req.AmountWithTax < 1000 {
		return ErrAmountTooLow
	}
	if req.AmountWithTax%1000 != 0 {
		return ErrAmountNotMultiple
	}

	return nil
}

// createPaymentRequest creates a new payment request record
func (p *PaymentFlowImpl) createPaymentRequest(ctx context.Context, customer models.Customer, amountWithTax uint64) (*models.PaymentRequest, error) {
	if customer.ReferrerAgencyID == nil {
		return nil, ErrReferrerAgencyIDRequired
	}

	// Generate unique invoice number
	invoiceNumber := fmt.Sprintf("INV-%s-%d", uuid.New().String(), time.Now().Unix())

	// Set expiration time (30 minutes from now)
	expiresAt := time.Now().Add(30 * time.Minute)

	agencyDiscount, err := p.agencyDiscountRepo.GetActiveDiscount(ctx, *customer.ReferrerAgencyID, customer.ID)
	if err != nil {
		return nil, err
	}
	if agencyDiscount == nil {
		return nil, ErrAgencyDiscountNotFound
	}

	scatteredSettlementItems, err := p.calculateScatteredSettlementItems(ctx, customer, amountWithTax)
	if err != nil {
		return nil, err
	}

	metadata, _ := json.Marshal(map[string]any{
		"source":                "wallet_recharge",
		"amount_with_tax":       amountWithTax,
		"system_share_with_tax": scatteredSettlementItems[0].Amount,
		"agency_share_with_tax": scatteredSettlementItems[1].Amount,
		"agency_discount_id":    agencyDiscount.ID,
		"agency_id":             customer.ReferrerAgencyID,
		"customer_id":           customer.ID,
	})

	// Create payment request
	paymentRequest := &models.PaymentRequest{
		UUID:          uuid.New(),
		CorrelationID: uuid.New(),
		CustomerID:    customer.ID,
		WalletID:      customer.Wallet.ID,
		Amount:        amountWithTax,
		Currency:      utils.TomanCurrency,
		Description:   "charge wallet",
		InvoiceNumber: invoiceNumber,
		CellNumber:    customer.RepresentativeMobile,
		RedirectURL:   fmt.Sprintf("https://%s/api/v1/payments/callback/%s", p.deploymentCfg.Domain, invoiceNumber),
		AtipayToken:   "", // Will be set later
		AtipayStatus:  "", // Will be set later
		// Payment*: "",   // Will be set later
		Status:       models.PaymentRequestStatusCreated,
		StatusReason: "payment request created for wallet charge",
		ExpiresAt:    &expiresAt,
		Metadata:     json.RawMessage(metadata),
	}
	if err := p.paymentRequestRepo.Save(ctx, paymentRequest); err != nil {
		return nil, err
	}

	return paymentRequest, nil
}

type ScatteredSettlementItem struct {
	Amount uint64 `json:"amount"`
	IBAN   string `json:"iban"`
}

func (p *PaymentFlowImpl) calculateScatteredSettlementItems(ctx context.Context, customer models.Customer, amountWithTax uint64) ([]ScatteredSettlementItem, error) {
	// Default shares fallback (50/50) if no discount found
	var systemShareWithTax uint64
	var agencyShareWithTax uint64

	discountRate, shebaNumber, err := p.getAgencyDiscountAndIBAN(ctx, customer)
	if err != nil {
		return nil, err
	}

	x := float64(amountWithTax) / (1 - discountRate)
	systemShareWithTax = uint64(x / 2)
	agencyShareWithTax = uint64(amountWithTax - systemShareWithTax)

	scatteredSettlementItems := make([]ScatteredSettlementItem, 0, 2)

	//--------------------------------
	// NOTE: ORDER MATTERS
	// --------------------------------

	systemUser, err := getSystemUser(ctx, p.customerRepo, p.walletRepo, p.sysCfg)
	if err != nil {
		return nil, err
	}
	if systemUser.ShebaNumber == nil {
		return nil, ErrSystemUserShebaNumberNotFound
	}

	scatteredSettlementItems = append(scatteredSettlementItems, ScatteredSettlementItem{
		Amount: systemShareWithTax,
		IBAN:   *systemUser.ShebaNumber,
	})
	scatteredSettlementItems = append(scatteredSettlementItems, ScatteredSettlementItem{
		Amount: agencyShareWithTax,
		IBAN:   shebaNumber,
	})

	return scatteredSettlementItems, nil
}

func (p *PaymentFlowImpl) getAgencyDiscountAndIBAN(ctx context.Context, customer models.Customer) (float64, string, error) {
	// Determine discount and agency IBAN if referrer exists
	var discountRate float64
	var shebaNumber string

	agency, err := getAgency(ctx, p.customerRepo, *customer.ReferrerAgencyID)
	if err != nil {
		return 0, "", err
	}

	shebaNumber, err = ValidateShebaNumber(agency.ShebaNumber)
	if err != nil {
		return 0, "", err
	}

	ad, err := p.agencyDiscountRepo.GetActiveDiscount(ctx, agency.ID, customer.ID)
	if err != nil {
		return 0, "", err
	}

	if ad != nil {
		discountRate = ad.DiscountRate
	}

	return discountRate, shebaNumber, nil
}

// callAtipayGetToken calls Atipay's get-token API
func (p *PaymentFlowImpl) callAtipayGetToken(ctx context.Context, customer models.Customer, paymentRequest models.PaymentRequest) (string, error) {
	scatteredSettlementItems, err := p.calculateScatteredSettlementItems(ctx, customer, paymentRequest.Amount)
	if err != nil {
		return "", err
	}
	amountWithTaxIRR := paymentRequest.Amount * 10 // TO IRR
	scatteredSettlementItems[0].Amount *= 10       // TO IRR
	scatteredSettlementItems[1].Amount *= 10       // TO IRR

	refinedScatteredSettlementItems := make([]ScatteredSettlementItem, 0)
	for _, item := range scatteredSettlementItems {
		if item.Amount > 0 {
			refinedScatteredSettlementItems = append(refinedScatteredSettlementItems, item)
		}
	}

	// Merge scatteredSettlementItems with same sheba number (IBAN)
	for i := 0; i < len(refinedScatteredSettlementItems); i++ {
		for j := i + 1; j < len(refinedScatteredSettlementItems); j++ {
			if refinedScatteredSettlementItems[i].IBAN == refinedScatteredSettlementItems[j].IBAN {
				refinedScatteredSettlementItems[i].Amount += refinedScatteredSettlementItems[j].Amount
				refinedScatteredSettlementItems = append(refinedScatteredSettlementItems[:j], refinedScatteredSettlementItems[j+1:]...)
			}
		}
	}

	// Prepare Atipay request payload
	atipayPayload := map[string]any{
		"amount":        amountWithTaxIRR,
		"cellNumber":    paymentRequest.CellNumber,
		"description":   paymentRequest.Description,
		"invoiceNumber": paymentRequest.InvoiceNumber,
		"redirectUrl":   paymentRequest.RedirectURL,
		"apiKey":        p.atipayCfg.APIKey,
		"terminal":      p.atipayCfg.Terminal,
	}

	systemUser, err := getSystemUser(ctx, p.customerRepo, p.walletRepo, p.sysCfg)
	if err != nil {
		return "", err
	}
	if systemUser.ShebaNumber == nil {
		return "", ErrSystemUserShebaNumberNotFound
	}

	if len(refinedScatteredSettlementItems) > 1 ||
		(len(refinedScatteredSettlementItems) == 1 && refinedScatteredSettlementItems[0].IBAN != *systemUser.ShebaNumber) {
		atipayPayload["scatteredSettlementItems"] = refinedScatteredSettlementItems
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
		Message          string `json:"message,omitempty"`
		ParsiMessage     string `json:"faMessage,omitempty"`
		ErrorCode        string `json:"errorCode,omitempty"`
		ErrorDescription string `json:"errorDescription,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&atipayResponse); err != nil {
		return "", err
	}

	// Check for errors in response
	if atipayResponse.Status != "1" {
		errorMsg := "unknown error"
		if atipayResponse.Message != "" {
			errorMsg = atipayResponse.Message
		}
		if atipayResponse.ErrorCode != "" {
			errorMsg = fmt.Sprintf("%s (code: %s)", errorMsg, atipayResponse.ErrorCode)
		}
		if atipayResponse.ErrorDescription != "" {
			errorMsg = fmt.Sprintf("%s (description: %s)", errorMsg, atipayResponse.ErrorDescription)
		}
		if atipayResponse.ParsiMessage != "" {
			errorMsg = fmt.Sprintf("%s (persian message: %s)", errorMsg, atipayResponse.ParsiMessage)
		}
		return "", fmt.Errorf("atipay API error: %s", errorMsg)
	}

	// Validate token
	if atipayResponse.Token == "" {
		return "", ErrAtipayTokenEmpty
	}

	return atipayResponse.Token, nil
}

// PaymentCallback handles the callback from Atipay after payment completion
func (p *PaymentFlowImpl) PaymentCallback(ctx context.Context, atipayRequest *dto.AtipayRequest, metadata *ClientMetadata) (string, error) {
	// Validate callback data
	if err := p.validateCallbackRequest(atipayRequest); err != nil {
		return "", NewBusinessError("PAYMENT_CALLBACK_VALIDATION_FAILED", "Payment callback validation failed", err)
	}

	var customer models.Customer
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

		customer, err = getCustomer(txCtx, p.customerRepo, paymentRequest.CustomerID)
		if err != nil {
			return err
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

		// If payment was successful, increase customer balance
		if mapping.Success {
			// Verify payment with Atipay before finalizing
			verificationResult, err := p.verifyPaymentWithAtipay(txCtx, atipayRequest.ReferenceNumber)
			if err != nil {
				// Failed but don't return error to avoid rollback
				mapping.Status = models.PaymentRequestStatusFailed
				mapping.Success = false
				mapping.Message = "Payment verification failed (step 1)"
				mapping.Description = "Payment verification failed (step 1): " + err.Error()

				// Update payment request status
				if err := p.updatePaymentRequest(txCtx, paymentRequest, atipayRequest, mapping); err != nil {
					return err
				}

				// Create audit log for verification failure
				errMsg := fmt.Sprintf("Payment verification failed (step 1) for payment request %d: %s", paymentRequest.ID, err.Error())
				_ = createAuditLog(txCtx, p.auditRepo, &customer, models.AuditActionPaymentFailed, mapping.Description, false, &errMsg, metadata)

				return nil
			}

			// Check if verified amount matches the original amount
			if uint64(verificationResult.AmountIRR) != paymentRequest.Amount*10 { // Convert Tomans to Rials
				// Amount mismatch - mark payment as failed and refund will occur
				mapping.Status = models.PaymentRequestStatusFailed
				mapping.Success = false
				mapping.Message = "Payment verification failed (step 2): amount mismatch"
				mapping.Description = fmt.Sprintf("Verified amount (%f Rials) does not match original amount (%d Rials)",
					verificationResult.AmountIRR, paymentRequest.Amount*10)

				// Update payment request status
				if err := p.updatePaymentRequest(txCtx, paymentRequest, atipayRequest, mapping); err != nil {
					return err
				}

				// Create audit log for verification failure
				errMsg := fmt.Sprintf("Payment verification failed (step 2) for payment request %d: amount mismatch (verified: %f Rials, original: %d Rials)", paymentRequest.ID, verificationResult.AmountIRR, paymentRequest.Amount*10)
				_ = createAuditLog(txCtx, p.auditRepo, &customer, models.AuditActionPaymentFailed, mapping.Description, false, &errMsg, metadata)

				return nil
			}

			// Verification successful - proceed with balance increase
			if err := p.updateBalances(txCtx, paymentRequest, atipayRequest); err != nil {
				// Failed but don't return error to avoid rollback
				mapping.Status = models.PaymentRequestStatusFailed
				mapping.Success = false
				mapping.Message = "Increase customer balance failed (step 3)"
				mapping.Description = "Increase customer balance failed (step 3): " + err.Error()

				// Update payment request status
				if err := p.updatePaymentRequest(txCtx, paymentRequest, atipayRequest, mapping); err != nil {
					return err
				}

				// Create audit log for increase customer balance failure
				errMsg := fmt.Sprintf("Increase customer balance failed (step 3) for payment request %d: %s", paymentRequest.ID, err.Error())
				_ = createAuditLog(txCtx, p.auditRepo, &customer, models.AuditActionPaymentFailed, mapping.Description, false, &errMsg, metadata)

				return nil
			}

			// Create audit log for successful balance increase
			msg := fmt.Sprintf("Wallet balance increased for payment request %d", paymentRequest.ID)
			_ = createAuditLog(txCtx, p.auditRepo, &customer, models.AuditActionWalletChargeCompleted, msg, true, nil, metadata)
		}

		return nil
	})

	paymentRequestID := uint(0)
	if paymentRequest != nil {
		paymentRequestID = paymentRequest.ID
	}
	if err != nil {
		errMsg := fmt.Sprintf("Payment callback failed for payment request %d: %s", paymentRequestID, err.Error())
		_ = createAuditLog(ctx, p.auditRepo, &customer, models.AuditActionPaymentCallbackProcessed, errMsg, false, &errMsg, metadata)

		return "", NewBusinessError("PAYMENT_CALLBACK_FAILED", "Payment callback failed", err)
	}

	msg := fmt.Sprintf("Payment callback processed for payment request %d", paymentRequestID)
	_ = createAuditLog(ctx, p.auditRepo, &customer, models.AuditActionPaymentCallbackProcessed, msg, true, nil, metadata)

	// Generate HTML response based on payment status
	htmlResponse, err := p.generatePaymentResultHTML(ctx, paymentRequest, atipayRequest, mapping)
	if err != nil {
		return "", NewBusinessError("PAYMENT_CALLBACK_HTML_GENERATION_FAILED", "Failed to generate HTML response", err)
	}

	return htmlResponse, nil
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
		Message:     "Payment cancelled by customer",
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

func (p *PaymentFlowImpl) updateBalances(ctx context.Context, paymentRequest *models.PaymentRequest, atipayRequest *dto.AtipayRequest) error {
	_, err := getCustomer(ctx, p.customerRepo, paymentRequest.CustomerID)
	if err != nil {
		return err
	}

	// unmarshal metadata
	var m map[string]any
	if err := json.Unmarshal(paymentRequest.Metadata, &m); err != nil {
		return err
	}

	realWithTax := uint64(m["amount_with_tax"].(float64))
	systemShareWithTax := uint64(m["system_share_with_tax"].(float64))
	agencyShareWithTax := uint64(m["agency_share_with_tax"].(float64))
	agencyDiscountID := uint(m["agency_discount_id"].(float64))
	agencyID := uint(m["agency_id"].(float64))

	agencyWallet, err := getWallet(ctx, p.walletRepo, agencyID)
	if err != nil {
		return err
	}

	// Get tax wallet
	taxWallet, err := getTaxWallet(ctx, p.walletRepo, p.sysCfg)
	if err != nil {
		return err
	}

	systemWallet, err := getSystemWallet(ctx, p.walletRepo, p.sysCfg)
	if err != nil {
		return err
	}

	// Get current balance snapshot for customer wallet
	customerBalance, err := getLatestBalanceSnapshot(ctx, p.walletRepo, paymentRequest.WalletID)
	if err != nil {
		return err
	}

	agencyBalance, err := getLatestBalanceSnapshot(ctx, p.walletRepo, agencyWallet.ID)
	if err != nil {
		return err
	}

	// Get current tax wallet balance
	taxBalance, err := getLatestTaxWalletBalanceSnapshot(ctx, p.walletRepo, taxWallet.ID)
	if err != nil {
		return err
	}

	// Get current system wallet balance
	systemBalance, err := getLatestSystemWalletBalanceSnapshot(ctx, p.walletRepo, systemWallet.ID)
	if err != nil {
		return err
	}

	agencyDiscount, err := p.agencyDiscountRepo.ByID(ctx, agencyDiscountID)
	if err != nil {
		return err
	}
	if agencyDiscount == nil {
		return ErrAgencyDiscountNotFound
	}

	real := uint64(realWithTax * 10 / 11)
	tax := realWithTax - real
	realSystemShare := uint64(systemShareWithTax * 10 / 11)
	taxSystemShare := systemShareWithTax - realSystemShare
	realAgencyShare := uint64(agencyShareWithTax * 10 / 11)
	taxAgencyShare := agencyShareWithTax - realAgencyShare
	customerCredit := uint64(float64(real)/(1-agencyDiscount.DiscountRate)) - real

	metadata := map[string]any{
		"customer_id":           paymentRequest.CustomerID,
		"agency_id":             agencyID,
		"agency_discount_id":    agencyDiscountID,
		"source":                "payment_callback",
		"operation":             "increase_balance",
		"payment_request_id":    paymentRequest.ID,
		"amount_with_tax":       realWithTax,
		"amount":                real,
		"tax":                   tax,
		"system_share_with_tax": systemShareWithTax,
		"system_share":          realSystemShare,
		"system_share_tax":      taxSystemShare,
		"agency_share_with_tax": agencyShareWithTax,
		"agency_share":          realAgencyShare,
		"agency_share_tax":      taxAgencyShare,
		"customer_credit":       customerCredit,
		"atipay_response":       atipayRequest,
	}

	// Update customer wallet balance
	newCustomerFreeBalance := customerBalance.FreeBalance + real
	newCustomerCreditBalance := customerBalance.CreditBalance + customerCredit
	metadata["source"] = "payment_callback_increase_customer_free_plus_credit"
	metadata["operation"] = "increase_customer_free_plus_credit"
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	newCustomerBalanceSnapshot := &models.BalanceSnapshot{
		UUID:          uuid.New(),
		CorrelationID: paymentRequest.CorrelationID,
		WalletID:      paymentRequest.WalletID,
		CustomerID:    paymentRequest.CustomerID,
		FreeBalance:   newCustomerFreeBalance,
		FrozenBalance: customerBalance.FrozenBalance,
		LockedBalance: customerBalance.LockedBalance,
		CreditBalance: newCustomerCreditBalance,
		TotalBalance:  newCustomerFreeBalance + newCustomerCreditBalance + customerBalance.FrozenBalance + customerBalance.LockedBalance,
		Reason:        "wallet_recharge",
		Description:   fmt.Sprintf("Wallet recharged via Atipay (payment request %d)", paymentRequest.ID),
		Metadata:      metadataJSON,
	}
	if err := p.balanceSnapshotRepo.Save(ctx, newCustomerBalanceSnapshot); err != nil {
		return err
	}

	// Create transaction record for customer wallet
	balanceBefore, err := customerBalance.GetBalanceMap()
	if err != nil {
		return err
	}
	balanceAfter, err := newCustomerBalanceSnapshot.GetBalanceMap()
	if err != nil {
		return err
	}
	customerTransaction := &models.Transaction{
		UUID:              uuid.New(),
		CorrelationID:     paymentRequest.CorrelationID,
		Type:              models.TransactionTypeDeposit,
		Status:            models.TransactionStatusCompleted,
		Amount:            real + customerCredit,
		Currency:          utils.TomanCurrency,
		WalletID:          paymentRequest.WalletID,
		CustomerID:        paymentRequest.CustomerID,
		BalanceBefore:     balanceBefore,
		BalanceAfter:      balanceAfter,
		ExternalReference: atipayRequest.ReferenceNumber,
		ExternalTrace:     atipayRequest.TraceNumber,
		ExternalRRN:       atipayRequest.RRN,
		ExternalMaskedPAN: atipayRequest.MaskedPAN,
		Description:       fmt.Sprintf("Wallet recharge (payment request %d)", paymentRequest.ID),
		Metadata:          metadataJSON,
	}
	if err := p.transactionRepo.Save(ctx, customerTransaction); err != nil {
		return err
	}

	newAgencyLockedBalance := agencyBalance.LockedBalance + agencyShareWithTax
	metadata["source"] = "payment_callback_increase_agency_locked_(agency_share_with_tax)"
	metadata["operation"] = "increase_agency_locked"
	metadataJSON, err = json.Marshal(metadata)
	if err != nil {
		return err
	}
	newAgencyBalanceSnapshot := &models.BalanceSnapshot{
		UUID:          uuid.New(),
		CorrelationID: paymentRequest.CorrelationID,
		WalletID:      agencyWallet.ID,
		CustomerID:    agencyWallet.CustomerID,
		FreeBalance:   agencyBalance.FreeBalance,
		FrozenBalance: agencyBalance.FrozenBalance,
		LockedBalance: newAgencyLockedBalance,
		CreditBalance: agencyBalance.CreditBalance,
		TotalBalance:  agencyBalance.FreeBalance + agencyBalance.FrozenBalance + newAgencyLockedBalance + agencyBalance.CreditBalance,
		Reason:        "agency_share_with_tax",
		Description:   fmt.Sprintf("Agency share for payment request %d", paymentRequest.ID),
		Metadata:      metadataJSON,
	}
	if err := p.balanceSnapshotRepo.Save(ctx, newAgencyBalanceSnapshot); err != nil {
		return err
	}

	// Create transaction record for agency wallet
	agencyBalanceBefore, err := agencyBalance.GetBalanceMap()
	if err != nil {
		return err
	}
	agencyBalanceAfter, err := newAgencyBalanceSnapshot.GetBalanceMap()
	if err != nil {
		return err
	}
	agencyTransaction := &models.Transaction{
		UUID:              uuid.New(),
		CorrelationID:     paymentRequest.CorrelationID,
		Type:              models.TransactionTypeLock,
		Status:            models.TransactionStatusCompleted,
		Amount:            agencyShareWithTax,
		Currency:          utils.TomanCurrency,
		WalletID:          agencyWallet.ID,
		CustomerID:        agencyWallet.CustomerID,
		BalanceBefore:     agencyBalanceBefore,
		BalanceAfter:      agencyBalanceAfter,
		ExternalReference: atipayRequest.ReferenceNumber,
		ExternalTrace:     atipayRequest.TraceNumber,
		ExternalRRN:       atipayRequest.RRN,
		ExternalMaskedPAN: atipayRequest.MaskedPAN,
		Description:       fmt.Sprintf("Agency share for payment request %d", paymentRequest.ID),
		Metadata:          metadataJSON,
	}
	if err := p.transactionRepo.Save(ctx, agencyTransaction); err != nil {
		return err
	}

	// Update tax wallet balance
	newTaxLockedBalance := taxBalance.LockedBalance + taxSystemShare
	metadata["source"] = "payment_callback_increase_tax_locked_(tax_system_share)"
	metadata["operation"] = "increase_tax_locked"
	metadataJSON, err = json.Marshal(metadata)
	if err != nil {
		return err
	}
	newTaxBalanceSnapshot := &models.BalanceSnapshot{
		UUID:          uuid.New(),
		CorrelationID: paymentRequest.CorrelationID,
		WalletID:      taxWallet.ID,
		CustomerID:    taxWallet.CustomerID,
		FreeBalance:   taxBalance.FreeBalance,
		FrozenBalance: taxBalance.FrozenBalance,
		LockedBalance: newTaxLockedBalance,
		CreditBalance: taxBalance.CreditBalance,
		TotalBalance:  taxBalance.FreeBalance + taxBalance.FrozenBalance + newTaxLockedBalance + taxBalance.CreditBalance,
		Reason:        "tax_collection",
		Description:   fmt.Sprintf("Tax collection for payment request %d", paymentRequest.ID),
		Metadata:      metadataJSON,
	}
	if err := p.balanceSnapshotRepo.Save(ctx, newTaxBalanceSnapshot); err != nil {
		return err
	}

	// Create transaction record for tax wallet
	taxBalanceBefore, err := taxBalance.GetBalanceMap()
	if err != nil {
		return err
	}
	taxBalanceAfter, err := newTaxBalanceSnapshot.GetBalanceMap()
	if err != nil {
		return err
	}
	taxTransaction := &models.Transaction{
		UUID:              uuid.New(),
		CorrelationID:     paymentRequest.CorrelationID,
		Type:              models.TransactionTypeLock,
		Status:            models.TransactionStatusCompleted,
		Amount:            taxSystemShare,
		Currency:          utils.TomanCurrency,
		WalletID:          taxWallet.ID,
		CustomerID:        taxWallet.CustomerID,
		BalanceBefore:     taxBalanceBefore,
		BalanceAfter:      taxBalanceAfter,
		ExternalReference: atipayRequest.ReferenceNumber,
		ExternalTrace:     atipayRequest.TraceNumber,
		ExternalRRN:       atipayRequest.RRN,
		ExternalMaskedPAN: atipayRequest.MaskedPAN,
		Description:       fmt.Sprintf("Tax collection for payment request %d", paymentRequest.ID),
		Metadata:          metadataJSON,
	}
	if err := p.transactionRepo.Save(ctx, taxTransaction); err != nil {
		return err
	}

	// Update system wallet balance
	newSystemLockedBalance := systemBalance.LockedBalance + realSystemShare
	metadata["source"] = "payment_callback_increase_system_locked_(real_system_share)"
	metadata["operation"] = "increase_system_locked"
	metadataJSON, err = json.Marshal(metadata)
	if err != nil {
		return err
	}
	newSystemBalanceSnapshot := &models.BalanceSnapshot{
		UUID:          uuid.New(),
		CorrelationID: paymentRequest.CorrelationID,
		WalletID:      systemWallet.ID,
		CustomerID:    systemWallet.CustomerID,
		FreeBalance:   systemBalance.FreeBalance,
		FrozenBalance: systemBalance.FrozenBalance,
		LockedBalance: newSystemLockedBalance,
		CreditBalance: systemBalance.CreditBalance,
		TotalBalance:  systemBalance.FreeBalance + systemBalance.FrozenBalance + newSystemLockedBalance + systemBalance.CreditBalance,
		Reason:        "real_system_share",
		Description:   fmt.Sprintf("System share for payment request %d", paymentRequest.ID),
		Metadata:      metadataJSON,
	}
	if err := p.balanceSnapshotRepo.Save(ctx, newSystemBalanceSnapshot); err != nil {
		return err
	}

	// Create transaction record for system wallet
	systemBalanceBefore, err := systemBalance.GetBalanceMap()
	if err != nil {
		return err
	}
	systemBalanceAfter, err := newSystemBalanceSnapshot.GetBalanceMap()
	if err != nil {
		return err
	}
	systemTransaction := &models.Transaction{
		UUID:              uuid.New(),
		CorrelationID:     paymentRequest.CorrelationID,
		Type:              models.TransactionTypeLock,
		Status:            models.TransactionStatusCompleted,
		Amount:            realSystemShare,
		Currency:          utils.TomanCurrency,
		WalletID:          systemWallet.ID,
		CustomerID:        systemWallet.CustomerID,
		BalanceBefore:     systemBalanceBefore,
		BalanceAfter:      systemBalanceAfter,
		ExternalReference: atipayRequest.ReferenceNumber,
		ExternalTrace:     atipayRequest.TraceNumber,
		ExternalRRN:       atipayRequest.RRN,
		ExternalMaskedPAN: atipayRequest.MaskedPAN,
		Description:       fmt.Sprintf("Real system share for payment request %d", paymentRequest.ID),
		Metadata:          metadataJSON,
	}
	if err := p.transactionRepo.Save(ctx, systemTransaction); err != nil {
		return err
	}

	return nil
}

// generatePaymentResultHTML generates HTML response based on payment status
func (p *PaymentFlowImpl) generatePaymentResultHTML(
	ctx context.Context,
	paymentRequest *models.PaymentRequest,
	atipayRequest *dto.AtipayRequest,
	mapping PaymentStatusMapping,
) (string, error) {
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

	var m map[string]any
	if err := json.Unmarshal(paymentRequest.Metadata, &m); err != nil {
		return "", err
	}

	realWithTax := uint64(m["amount_with_tax"].(float64))
	agencyDiscountID := uint(m["agency_discount_id"].(float64))

	agencyDiscount, err := p.agencyDiscountRepo.ByID(ctx, agencyDiscountID)
	if err != nil {
		return "", err
	}
	if agencyDiscount == nil {
		return "", ErrAgencyDiscountNotFound
	}

	real := uint64(realWithTax * 10 / 11)
	tax := realWithTax - real
	customerCredit := uint64(float64(real)/(1-agencyDiscount.DiscountRate)) - real

	// Prepare template data
	data := map[string]any{
		"Status":          mapping.Status,
		"Message":         mapping.Message,
		"TotalAmount":     realWithTax,
		"TaxAmount":       tax,
		"NetAmount":       real,
		"CreditAmount":    customerCredit,
		"ReferenceNumber": atipayRequest.ReferenceNumber,
		"TraceNumber":     atipayRequest.TraceNumber,
		"RRN":             atipayRequest.RRN,
		"MaskedPAN":       atipayRequest.MaskedPAN,
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
	AmountIRR float64 `json:"amount"`
}

// verifyPaymentWithAtipay calls Atipay's verify-payment API to finalize the transaction
func (p *PaymentFlowImpl) verifyPaymentWithAtipay(ctx context.Context, referenceNumber string) (*AtipayVerificationResponse, error) {
	// Prepare Atipay verification request payload
	verificationPayload := map[string]any{
		"referenceNumber": referenceNumber,
		"apiKey":          p.atipayCfg.APIKey,
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
	customer, err := getCustomer(ctx, p.customerRepo, req.CustomerID)
	if err != nil {
		return nil, err
	}

	// Get customer's wallet
	wallet, err := getWallet(ctx, p.walletRepo, req.CustomerID)
	if err != nil {
		return nil, err
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
	transactions, err := p.transactionRepo.ByFilter(ctx, filter, "id DESC", int(req.PageSize), int(offset))
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
	_ = createAuditLog(ctx, p.auditRepo, &customer, models.AuditActionTransactionHistoryRetrieved, msg, true, nil, metadata)

	response = &dto.TransactionHistoryResponse{
		Items:      items,
		Pagination: pagination,
	}

	return response, nil
}

// validateGetTransactionHistoryRequest validates the transaction history request
func (p *PaymentFlowImpl) validateGetTransactionHistoryRequest(req *dto.GetTransactionHistoryRequest) error {
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

// GetWalletBalance retrieves the current wallet balance for a customer
func (p *PaymentFlowImpl) GetWalletBalance(ctx context.Context, req *dto.GetWalletBalanceRequest, metadata *ClientMetadata) (*dto.GetWalletBalanceResponse, error) {
	var err error
	defer func() {
		if err != nil {
			err = NewBusinessError("GET_WALLET_BALANCE_FAILED", "Get wallet balance failed", err)
		}
	}()

	// Verify customer exists and is active
	_, err = getCustomer(ctx, p.customerRepo, req.CustomerID)
	if err != nil {
		return nil, err
	}

	// Find wallet
	wallet, err := getWallet(ctx, p.walletRepo, req.CustomerID)
	if err != nil {
		return nil, err
	}

	// Latest balance latestSnapshot
	latestSnapshot, err := getLatestBalanceSnapshot(ctx, p.walletRepo, wallet.ID)
	if err != nil {
		return nil, err
	}

	resp := &dto.GetWalletBalanceResponse{
		Message:             "Wallet balance retrieved successfully",
		Free:                latestSnapshot.FreeBalance,
		Locked:              latestSnapshot.LockedBalance,
		Frozen:              latestSnapshot.FrozenBalance,
		Credit:              latestSnapshot.CreditBalance,
		Total:               latestSnapshot.TotalBalance,
		Currency:            utils.TomanCurrency,
		LastUpdated:         latestSnapshot.CreatedAt.Format(time.RFC3339),
		PendingTransactions: 0,
		MinimumBalance:      0,
		BalanceStatus:       "active",
	}

	return resp, nil
}
