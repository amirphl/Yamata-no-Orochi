package businessflow

import (
	"context"
	"encoding/json"
	"fmt"

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
	ChargeWalletByAdmin(ctx context.Context, req *dto.ChargeWalletByAdminRequest, metadata *ClientMetadata, adminID uint) (*dto.ChargeWalletByAdminResponse, error)
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
		db:                  db,
		atipayCfg:           atipayCfg,
		sysCfg:              sysCfg,
		deploymentCfg:       deploymentCfg,
	}
}

// ChargeWalletByAdmin directly charges a customer's wallet without redirecting to Atipay.
func (p *PaymentFlowImpl) ChargeWalletByAdmin(
	ctx context.Context,
	req *dto.ChargeWalletByAdminRequest,
	metadata *ClientMetadata,
	adminID uint,
) (*dto.ChargeWalletByAdminResponse, error) {
	if req == nil {
		return nil, NewBusinessError("CHARGE_WALLET_BY_ADMIN_FAILED", "Charge wallet by admin failed", fmt.Errorf("request is nil"))
	}
	if req.CustomerID == 0 {
		return nil, NewBusinessError("CHARGE_WALLET_BY_ADMIN_FAILED", "Charge wallet by admin failed", fmt.Errorf("customer_id is required"))
	}
	if err := p.validateChargeWalletRequest(&dto.ChargeWalletRequest{AmountWithTax: req.AmountWithTax}); err != nil {
		return nil, NewBusinessError("CHARGE_WALLET_BY_ADMIN_FAILED", "Charge wallet by admin failed", err)
	}

	var customer models.Customer
	var paymentRequest *models.PaymentRequest

	err := repository.WithTransaction(ctx, p.db, func(txCtx context.Context) error {
		var err error
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

		paymentRequest, err = p.createPaymentRequest(txCtx, customer, req.AmountWithTax)
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
		metadataJSON, err := json.Marshal(m)
		if err != nil {
			return err
		}

		syntheticReference := fmt.Sprintf("SYNTHETIC_ADMIN-%s", uuid.New().String())
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
	_ = createAuditLog(ctx, p.auditRepo, &customer, models.AuditActionWalletChargeCompleted, msg, true, nil, metadata)
	_ = createAuditLog(ctx, p.auditRepo, &customer, models.AuditActionPaymentCallbackProcessed, msg, true, nil, metadata)

	return &dto.ChargeWalletByAdminResponse{
		Message:          "Wallet charged successfully by admin",
		Success:          true,
		PaymentRequestID: paymentRequest.ID,
		InvoiceNumber:    paymentRequest.InvoiceNumber,
		ReferenceNumber:  paymentRequest.PaymentReference,
		CustomerID:       req.CustomerID,
		AdminID:          adminID,
		AmountWithTax:    req.AmountWithTax,
	}, nil
}
