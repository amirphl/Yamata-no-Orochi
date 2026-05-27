package businessflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PaymentAdminFlow handles admin-only payment business logic.
type PaymentAdminFlow interface {
	AdminChargeWallet(ctx context.Context, req *dto.AdminChargeWalletRequest, metadata *ClientMetadata, adminID uint) (*dto.AdminChargeWalletResponse, error)
	AdminListDepositReceipts(ctx context.Context, f models.DepositReceiptFilter, limit, offset int, order string) (*dto.ListDepositReceiptsResponse, error)
	AdminGetDepositReceiptFile(ctx context.Context, receiptUUID string) ([]byte, string, string, error)
	AdminUpdateDepositReceiptStatus(ctx context.Context, req *dto.AdminUpdateDepositReceiptStatusRequest, adminID uint, metadata *ClientMetadata) (*dto.SubmitDepositReceiptResponse, error)
}

// NewPaymentAdminFlow creates a new admin payment flow instance.
func NewPaymentAdminFlow(
	paymentRequestRepo repository.PaymentRequestRepository,
	walletRepo repository.WalletRepository,
	customerRepo repository.CustomerRepository,
	auditRepo repository.AuditLogRepository,
	balanceSnapshotRepo repository.BalanceSnapshotRepository,
	transactionRepo repository.TransactionRepository,
	agencyDiscountRepo repository.AgencyDiscountRepository,
	depositReceiptRepo repository.DepositReceiptRepository,
	db *gorm.DB,
	atipayCfg config.AtipayConfig,
	sysCfg config.SystemConfig,
	deploymentCfg config.DeploymentConfig,
) PaymentAdminFlow {
	return &PaymentFlowImpl{
		paymentRequestRepo:  paymentRequestRepo,
		walletRepo:          walletRepo,
		customerRepo:        customerRepo,
		auditRepo:           auditRepo,
		balanceSnapshotRepo: balanceSnapshotRepo,
		transactionRepo:     transactionRepo,
		agencyDiscountRepo:  agencyDiscountRepo,
		depositReceiptRepo:  depositReceiptRepo,
		db:                  db,
		atipayCfg:           atipayCfg,
		sysCfg:              sysCfg,
		deploymentCfg:       deploymentCfg,
	}
}

// AdminChargeWallet directly charges a customer's wallet without redirecting to Atipay.
func (p *PaymentFlowImpl) AdminChargeWallet(
	ctx context.Context,
	req *dto.AdminChargeWalletRequest,
	metadata *ClientMetadata,
	adminID uint,
) (*dto.AdminChargeWalletResponse, error) {
	if req == nil {
		return nil, NewBusinessError("CHARGE_WALLET_BY_ADMIN_FAILED", "Charge wallet by admin failed", fmt.Errorf("request is nil"))
	}
	if req.CustomerID == 0 {
		return nil, NewBusinessError("CHARGE_WALLET_BY_ADMIN_FAILED", "Charge wallet by admin failed", fmt.Errorf("customer_id is required"))
	}
	if err := p.validateChargeWalletRequest(&dto.ChargeWalletRequest{AmountWithTax: req.AmountWithTax}); err != nil {
		return nil, NewBusinessError("CHARGE_WALLET_BY_ADMIN_FAILED", "Charge wallet by admin failed", err)
	}
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey == "" && metadata != nil {
		idempotencyKey = strings.TrimSpace(metadata.RequestID)
	}
	if idempotencyKey == "" {
		return nil, NewBusinessError("IDEMPOTENCY_KEY_REQUIRED", "Idempotency key is required", fmt.Errorf("idempotency_key is required"))
	}

	var customer models.Customer
	var paymentRequest *models.PaymentRequest
	isIdempotentReplay := false

	err := repository.WithTransaction(ctx, p.db, func(txCtx context.Context) error {
		var err error

		lockName := "charge_wallet_by_admin:" + idempotencyKey
		if err := p.db.WithContext(txCtx).Exec("SELECT pg_advisory_xact_lock(hashtext(?))", lockName).Error; err != nil {
			return err
		}

		existing, err := p.findAdminChargeByIdempotencyKey(txCtx, idempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.CustomerID != req.CustomerID || existing.Amount != req.AmountWithTax {
				return fmt.Errorf("idempotency key already used with different request payload")
			}
			existingAdminID := paymentMetadataAdminID(existing.Metadata)
			if existingAdminID != 0 && existingAdminID != adminID {
				return fmt.Errorf("idempotency key already used by a different admin")
			}
			if existing.Status != models.PaymentRequestStatusCompleted {
				return fmt.Errorf("idempotency key exists but payment is not completed yet")
			}
			customer = models.Customer{ID: req.CustomerID}
			paymentRequest = existing
			isIdempotentReplay = true
			return nil
		}

		customer, err = getCustomer(txCtx, p.customerRepo, req.CustomerID)
		if err != nil {
			return err
		}

		// Ensure customer wallet exists and attach it to customer for payment request creation.
		wallet, err := p.walletRepo.ByCustomerID(txCtx, customer.ID)
		if err != nil {
			return err
		}
		customer.Wallet = wallet

		paymentRequest, err = p.createPaymentRequest(txCtx, customer, req.AmountWithTax, "EN")
		if err != nil {
			return err
		}

		// Mark origin metadata so downstream snapshots/transactions clearly show admin direct charge details.
		var m map[string]any
		if err := json.Unmarshal(paymentRequest.Metadata, &m); err != nil {
			return err
		}
		m["source"] = "wallet_recharge_admin"
		m["admin_id"] = adminID
		m["charged_by"] = "admin"
		m["payment_channel"] = "admin_direct_charge"
		m["admin_charge"] = true
		m["idempotency_key"] = idempotencyKey
		metadataJSON, err := json.Marshal(m)
		if err != nil {
			return err
		}

		syntheticReference := fmt.Sprintf("SYNTHETIC_REF-%s", uuid.New().String())
		syntheticTrace := fmt.Sprintf("SYNTHETIC_TRACE-%s", uuid.New().String())
		syntheticRRN := fmt.Sprintf("SYNTHETIC_RRN-%s", uuid.New().String())

		paymentRequest.Metadata = metadataJSON
		paymentRequest.Description = "charge wallet by admin"
		paymentRequest.RedirectURL = fmt.Sprintf("https://%s/admin/payments/direct-charge", p.deploymentCfg.Domain)
		paymentRequest.AtipayToken = "ADMIN_DIRECT_CHARGE"
		paymentRequest.AtipayStatus = "OK"
		paymentRequest.Status = models.PaymentRequestStatusPending
		paymentRequest.StatusReason = "payment request pending for admin direct charge"
		paymentRequest.UpdatedAt = utils.UTCNow()
		if err := p.paymentRequestRepo.Update(txCtx, paymentRequest); err != nil {
			return err
		}

		callbackReq := &dto.AtipayRequest{
			State:             "OK",
			Status:            "2",
			ReferenceNumber:   syntheticReference,
			ReservationNumber: paymentRequest.InvoiceNumber,
			TerminalID:        "ADMIN_PANEL",
			TraceNumber:       syntheticTrace,
			MaskedPAN:         "ADMIN-DIRECT",
			RRN:               syntheticRRN,
		}
		mapping := p.getPaymentStatusMapping(callbackReq.Status, callbackReq.State)
		mapping.Description = "Payment completed successfully via admin direct charge"
		if err := p.updatePaymentRequest(txCtx, paymentRequest, callbackReq, mapping); err != nil {
			return err
		}

		if err := p.updateBalances(txCtx, paymentRequest, callbackReq); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		errMsg := fmt.Sprintf("Charge wallet by admin failed for customer %d by admin %d: %s", req.CustomerID, adminID, err.Error())
		_ = createAuditLog(ctx, p.auditRepo, &customer, models.AuditActionWalletChargeFailed, errMsg, false, &errMsg, metadata)
		return nil, NewBusinessError("CHARGE_WALLET_BY_ADMIN_FAILED", "Failed to charge wallet by admin", err)
	}

	msg := fmt.Sprintf("Wallet charged by admin %d for customer %d (payment request %d)", adminID, req.CustomerID, paymentRequest.ID)
	if isIdempotentReplay {
		msg = fmt.Sprintf("Wallet charge idempotency replay by admin %d for customer %d (payment request %d)", adminID, req.CustomerID, paymentRequest.ID)
	}
	_ = createAuditLog(ctx, p.auditRepo, &customer, models.AuditActionWalletChargeCompleted, msg, true, nil, metadata)
	_ = createAuditLog(ctx, p.auditRepo, &customer, models.AuditActionPaymentCallbackProcessed, msg, true, nil, metadata)

	return &dto.AdminChargeWalletResponse{
		Message: func() string {
			if isIdempotentReplay {
				return "Wallet charge already processed (idempotent replay)"
			}
			return "Wallet charged successfully by admin"
		}(),
		Success:          true,
		PaymentRequestID: paymentRequest.ID,
		InvoiceNumber:    paymentRequest.InvoiceNumber,
		ReferenceNumber:  paymentRequest.PaymentReference,
		CustomerID:       req.CustomerID,
		AdminID:          adminID,
		AmountWithTax:    req.AmountWithTax,
	}, nil
}

// AdminListDepositReceipts lists receipts with optional filters.
func (p *PaymentFlowImpl) AdminListDepositReceipts(ctx context.Context, f models.DepositReceiptFilter, limit, offset int, order string) (*dto.ListDepositReceiptsResponse, error) {
	items, err := p.depositReceiptRepo.List(ctx, f, limit, offset, order)
	if err != nil {
		return nil, NewBusinessError("ADMIN_LIST_DEPOSIT_RECEIPTS_FAILED", "Failed to list deposit receipts", err)
	}
	resp := &dto.ListDepositReceiptsResponse{Items: make([]dto.DepositReceiptItem, 0, len(items))}
	for _, r := range items {
		preview, previewType := buildReceiptPreview(r.FileData, r.ContentType)
		resp.Items = append(resp.Items, dto.DepositReceiptItem{
			UUID:          r.UUID.String(),
			CustomerID:    r.CustomerID,
			Amount:        r.Amount,
			Currency:      r.Currency,
			Status:        string(r.Status),
			StatusReason:  r.StatusReason,
			RejectionNote: r.RejectionNote,
			Lang:          r.Lang,
			FileName:      r.FileName,
			ContentType:   r.ContentType,
			FileSize:      r.FileSize,
			PreviewBase64: preview,
			PreviewType:   previewType,
			CreatedAt:     r.CreatedAt,
		})
	}
	return resp, nil
}

// AdminGetDepositReceiptFile returns raw file bytes and metadata.
func (p *PaymentFlowImpl) AdminGetDepositReceiptFile(ctx context.Context, receiptUUID string) ([]byte, string, string, error) {
	rec, err := p.depositReceiptRepo.ByUUID(ctx, receiptUUID)
	if err != nil {
		return nil, "", "", err
	}
	if rec == nil {
		return nil, "", "", ErrDepositReceiptNotFound
	}
	return rec.FileData, rec.FileName, rec.ContentType, nil
}

// AdminUpdateDepositReceiptStatus approves or rejects a receipt and on approval credits wallet like normal flow.
func (p *PaymentFlowImpl) AdminUpdateDepositReceiptStatus(ctx context.Context, req *dto.AdminUpdateDepositReceiptStatusRequest, adminID uint, metadata *ClientMetadata) (*dto.SubmitDepositReceiptResponse, error) {
	if req == nil {
		return nil, NewBusinessError("ADMIN_RECEIPT_UPDATE_INVALID", "Invalid request", fmt.Errorf("nil request"))
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != "approve" && action != "reject" {
		return nil, ErrDepositReceiptInvalidStatus
	}

	var customer models.Customer
	var receipt *models.DepositReceipt
	err := repository.WithTransaction(ctx, p.db, func(txCtx context.Context) error {
		var err error
		receipt, err = p.depositReceiptRepo.ByUUID(txCtx, req.ReceiptUUID)
		if err != nil {
			return err
		}
		if receipt == nil {
			return ErrDepositReceiptNotFound
		}
		if receipt.Status == models.DepositReceiptStatusApproved {
			return ErrDepositReceiptAlreadyApproved
		}
		if receipt.Status == models.DepositReceiptStatusRejected {
			return ErrDepositReceiptAlreadyRejected
		}

		if action == "reject" {
			receipt.Status = models.DepositReceiptStatusRejected
			receipt.StatusReason = "Rejected by admin"
			receipt.ReviewerID = &adminID
			if req.Reason != "" {
				receipt.RejectionNote = &req.Reason
			}
			if err := p.depositReceiptRepo.Update(txCtx, receipt); err != nil {
				return err
			}
			desc := fmt.Sprintf("Deposit receipt %s rejected by admin %d", req.ReceiptUUID, adminID)
			_ = createAuditLog(txCtx, p.auditRepo, nil, models.AuditActionAdminDepositReceiptReviewed, desc, true, nil, metadata)
			return nil
		}

		// Approve path mirrors AdminChargeWallet + PaymentCallback success.
		customer, err = getCustomer(txCtx, p.customerRepo, receipt.CustomerID)
		if err != nil {
			return err
		}
		wallet, err := p.walletRepo.ByCustomerID(txCtx, customer.ID)
		if err != nil {
			return err
		}
		customer.Wallet = wallet

		paymentRequest, err := p.createPaymentRequest(txCtx, customer, receipt.Amount, receipt.Lang)
		if err != nil {
			return err
		}

		// Mark metadata
		var m map[string]any
		if err := json.Unmarshal(paymentRequest.Metadata, &m); err != nil {
			return err
		}
		m["source"] = "deposit_receipt"
		m["deposit_receipt_uuid"] = receipt.UUID.String()
		m["deposit_invoice_number"] = receipt.InvoiceNumber
		m["admin_id"] = adminID
		m["payment_channel"] = "deposit_receipt_manual"
		metaJSON, err := json.Marshal(m)
		if err != nil {
			return err
		}

		paymentRequest.Metadata = metaJSON
		paymentRequest.Description = "wallet charge via deposit receipt"
		paymentRequest.RedirectURL = fmt.Sprintf("https://%s/api/v1/payments/deposit-receipt", p.deploymentCfg.Domain)
		paymentRequest.AtipayToken = "ADMIN_DEPOSIT_RECEIPT"
		paymentRequest.AtipayStatus = "OK"
		paymentRequest.Status = models.PaymentRequestStatusPending
		paymentRequest.StatusReason = "payment request pending for deposit receipt approval"
		paymentRequest.UpdatedAt = utils.UTCNow()
		if err := p.paymentRequestRepo.Update(txCtx, paymentRequest); err != nil {
			return err
		}

		callbackReq := &dto.AtipayRequest{
			State:             "OK",
			Status:            "2",
			ReferenceNumber:   fmt.Sprintf("DEPOSIT_RECEIPT-REF-%s", uuid.New().String()),
			ReservationNumber: paymentRequest.InvoiceNumber,
			TerminalID:        "DEPOSIT_RECEIPT",
			TraceNumber:       fmt.Sprintf("DEPOSIT_RECEIPT-TRACE-%s", uuid.New().String()),
			MaskedPAN:         "DEPOSIT_RECEIPT",
			RRN:               fmt.Sprintf("DEPOSIT_RECEIPT-RRN-%s", uuid.New().String()),
		}
		mapping := p.getPaymentStatusMapping(callbackReq.Status, callbackReq.State)
		mapping.Description = "Payment completed via deposit receipt approval"
		if err := p.updatePaymentRequest(txCtx, paymentRequest, callbackReq, mapping); err != nil {
			return err
		}
		if err := p.updateBalances(txCtx, paymentRequest, callbackReq); err != nil {
			return err
		}

		receipt.Status = models.DepositReceiptStatusApproved
		receipt.StatusReason = "Approved and credited"
		receipt.ReviewerID = &adminID
		if err := p.depositReceiptRepo.Update(txCtx, receipt); err != nil {
			return err
		}

		desc := fmt.Sprintf("Deposit receipt %s approved by admin %d and credited", req.ReceiptUUID, adminID)
		_ = createAuditLog(txCtx, p.auditRepo, &customer, models.AuditActionAdminDepositReceiptReviewed, desc, true, nil, metadata)
		return nil
	})
	if err != nil {
		errMsg := fmt.Sprintf("Admin %d failed to update receipt %s: %v", adminID, req.ReceiptUUID, err)
		_ = createAuditLog(ctx, p.auditRepo, nil, models.AuditActionAdminDepositReceiptReviewed, errMsg, false, &errMsg, metadata)
		return nil, NewBusinessError("ADMIN_RECEIPT_UPDATE_FAILED", "Failed to update deposit receipt", err)
	}

	msg := fmt.Sprintf("Admin %d updated receipt %s to %s", adminID, req.ReceiptUUID, req.Action)
	_ = createAuditLog(ctx, p.auditRepo, &customer, models.AuditActionAdminDepositReceiptReviewed, msg, true, nil, metadata)

	return &dto.SubmitDepositReceiptResponse{
		Success:     true,
		Message:     fmt.Sprintf("Receipt %s %sed", req.ReceiptUUID, req.Action),
		ReceiptUUID: req.ReceiptUUID,
		Status:      string(receipt.Status),
	}, nil
}

func (p *PaymentFlowImpl) findAdminChargeByIdempotencyKey(ctx context.Context, idempotencyKey string) (*models.PaymentRequest, error) {
	var req models.PaymentRequest
	err := p.db.WithContext(ctx).
		Where("metadata ->> 'source' = ?", "wallet_recharge_admin").
		Where("metadata ->> 'idempotency_key' = ?", idempotencyKey).
		Order("id DESC").
		First(&req).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &req, nil
}

func paymentMetadataAdminID(raw json.RawMessage) uint {
	if len(raw) == 0 {
		return 0
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return 0
	}
	v, ok := m["admin_id"]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case float64:
		if x <= 0 {
			return 0
		}
		return uint(x)
	case int:
		if x <= 0 {
			return 0
		}
		return uint(x)
	case string:
		i, err := strconv.ParseUint(strings.TrimSpace(x), 10, 64)
		if err != nil || i == 0 {
			return 0
		}
		return uint(i)
	default:
		return 0
	}
}
