// Package businessflow contains the core business logic and use cases for campaign workflows
package businessflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/app/services"
	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AdminCampaignFlow handles the campaign business logic
type AdminCampaignFlow interface {
	ListCampaigns(ctx context.Context, filter dto.AdminListCampaignsFilter) (*dto.AdminListCampaignsResponse, error)
	ApproveCampaign(ctx context.Context, req *dto.AdminApproveCampaignRequest) (*dto.AdminApproveCampaignResponse, error)
	RejectCampaign(ctx context.Context, req *dto.AdminRejectCampaignRequest) (*dto.AdminRejectCampaignResponse, error)
}

// AdminCampaignFlowImpl implements the campaign business flow
type AdminCampaignFlowImpl struct {
	campaignRepo        repository.CampaignRepository
	customerRepo        repository.CustomerRepository
	walletRepo          repository.WalletRepository
	balanceSnapshotRepo repository.BalanceSnapshotRepository
	transactionRepo     repository.TransactionRepository
	auditRepo           repository.AuditLogRepository
	notifier            services.NotificationService
	adminConfig         config.AdminConfig
	db                  *gorm.DB
}

// NewAdminCampaignFlow creates a new campaign flow instance
func NewAdminCampaignFlow(
	campaignRepo repository.CampaignRepository,
	customerRepo repository.CustomerRepository,
	walletRepo repository.WalletRepository,
	balanceSnapshotRepo repository.BalanceSnapshotRepository,
	transactionRepo repository.TransactionRepository,
	auditRepo repository.AuditLogRepository,
	db *gorm.DB,
	notifier services.NotificationService,
	adminConfig config.AdminConfig,
) AdminCampaignFlow {
	return &AdminCampaignFlowImpl{
		campaignRepo:        campaignRepo,
		customerRepo:        customerRepo,
		walletRepo:          walletRepo,
		balanceSnapshotRepo: balanceSnapshotRepo,
		transactionRepo:     transactionRepo,
		auditRepo:           auditRepo,
		notifier:            notifier,
		adminConfig:         adminConfig,
		db:                  db,
	}
}

// ListCampaigns retrieves campaigns for admin using optional filters: title (name), status, start/end dates
func (s *AdminCampaignFlowImpl) ListCampaigns(ctx context.Context, filter dto.AdminListCampaignsFilter) (*dto.AdminListCampaignsResponse, error) {
	cf := models.CampaignFilter{}
	if filter.Title != nil && *filter.Title != "" {
		cf.Title = filter.Title
	}
	if filter.Status != nil && *filter.Status != "" {
		st := models.CampaignStatus(*filter.Status)
		if st.Valid() {
			cf.Status = &st
		}
	}
	if filter.StartDate != nil {
		cf.CreatedAfter = filter.StartDate
	}
	if filter.EndDate != nil {
		cf.CreatedBefore = filter.EndDate
	}

	if filter.StartDate != nil && filter.EndDate != nil {
		if filter.EndDate.Before(*filter.StartDate) {
			return nil, NewBusinessError("ADMIN_LIST_CAMPAIGNS_FAILED", "End date must be after start date", nil)
		}
	}

	rows, err := s.campaignRepo.ByFilter(ctx, cf, "created_at DESC", 0, 0)
	if err != nil {
		return nil, NewBusinessError("ADMIN_LIST_CAMPAIGNS_FAILED", "Failed to list campaigns", err)
	}
	items := make([]dto.AdminGetCampaignResponse, 0, len(rows))
	for _, c := range rows {
		items = append(items, dto.AdminGetCampaignResponse{
			UUID:       c.UUID.String(),
			Status:     c.Status.String(),
			CreatedAt:  c.CreatedAt,
			UpdatedAt:  c.UpdatedAt,
			Title:      c.Spec.Title,
			Segment:    c.Spec.Segment,
			Subsegment: c.Spec.Subsegment,
			Sex:        c.Spec.Sex,
			City:       c.Spec.City,
			AdLink:     c.Spec.AdLink,
			Content:    c.Spec.Content,
			ScheduleAt: c.Spec.ScheduleAt,
			LineNumber: c.Spec.LineNumber,
			Budget:     c.Spec.Budget,
			Comment:    c.Comment,
		})
	}

	return &dto.AdminListCampaignsResponse{
		Message: "Campaigns retrieved successfully",
		Items:   items,
	}, nil
}

// ApproveCampaign approves a campaign: ensure schedule_at > now, change status to approved, and reduce frozen to locked spend
func (s *AdminCampaignFlowImpl) ApproveCampaign(ctx context.Context, req *dto.AdminApproveCampaignRequest) (*dto.AdminApproveCampaignResponse, error) {
	if req == nil || req.CampaignID == 0 {
		return nil, NewBusinessError("ADMIN_APPROVE_CAMPAIGN_FAILED", "campaign_id is required", nil)
	}

	var campaign *models.Campaign
	var customer models.Customer

	err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		var err error
		campaign, err = s.campaignRepo.ByID(txCtx, req.CampaignID)
		if err != nil {
			return err
		}
		if campaign == nil {
			return ErrCampaignNotFound
		}
		if campaign.Status != models.CampaignStatusWaitingForApproval {
			return ErrCampaignNotWaitingForApproval
		}
		if campaign.Spec.ScheduleAt == nil || campaign.Spec.ScheduleAt.Before(utils.UTCNow()) {
			return ErrScheduleTimeTooSoon
		}

		customer, err = getCustomer(txCtx, s.customerRepo, campaign.CustomerID)
		if err != nil {
			return err
		}

		// Find the frozen reservation transaction created during finalize
		freezeTxs, err := s.transactionRepo.ByFilter(txCtx, models.TransactionFilter{
			CustomerID: &campaign.CustomerID,
			CampaignID: &campaign.ID,
			Source:     utils.ToPtr("campaign_update"),
			Operation:  utils.ToPtr("reserve_budget"),
			Type:       utils.ToPtr(models.TransactionTypeLaunchCampaign),
			Status:     utils.ToPtr(models.TransactionStatusPending),
		}, "id DESC", 0, 0)
		if err != nil {
			return err
		}
		if len(freezeTxs) == 0 {
			return ErrFreezeTransactionNotFound
		}
		if len(freezeTxs) > 1 {
			return ErrMultipleFreezeTransactionsFound
		}
		freezeTx := freezeTxs[0]

		// Load wallet and current balance
		wallet, err := getWallet(txCtx, s.walletRepo, campaign.CustomerID)
		if err != nil {
			return err
		}
		latestBalance, err := getLatestBalanceSnapshot(txCtx, s.walletRepo, wallet.ID)
		if err != nil {
			return err
		}

		amount := freezeTx.Amount
		if latestBalance.FrozenBalance < amount {
			return ErrInsufficientFunds
		}

		meta := map[string]any{
			"source":      "admin_campaign_approve",
			"operation":   "approve_campaign_budget_consume",
			"campaign_id": campaign.ID,
		}
		if req.Comment != nil {
			meta["comment"] = *req.Comment
		}
		metaBytes, _ := json.Marshal(meta)

		// Move from frozen to locked (spend reserved budget)
		newFrozen := latestBalance.FrozenBalance - amount
		newLocked := latestBalance.LockedBalance + amount

		newSnap := &models.BalanceSnapshot{
			UUID:          uuid.New(),
			CorrelationID: freezeTx.CorrelationID,
			WalletID:      wallet.ID,
			CustomerID:    customer.ID,
			FreeBalance:   latestBalance.FreeBalance,
			FrozenBalance: newFrozen,
			LockedBalance: newLocked,
			CreditBalance: latestBalance.CreditBalance,
			TotalBalance:  latestBalance.FreeBalance + newFrozen + newLocked + latestBalance.CreditBalance,
			Reason:        "campaign_approved_budget_locked",
			Description:   fmt.Sprintf("Budget locked for approved campaign %d", campaign.ID),
			Metadata:      metaBytes,
		}
		if err := s.balanceSnapshotRepo.Save(txCtx, newSnap); err != nil {
			return err
		}

		beforeMap, err := latestBalance.GetBalanceMap()
		if err != nil {
			return err
		}
		afterMap, err := newSnap.GetBalanceMap()
		if err != nil {
			return err
		}

		spendTx := &models.Transaction{
			UUID:          uuid.New(),
			CorrelationID: freezeTx.CorrelationID,
			Type:          models.TransactionTypeLock,
			Status:        models.TransactionStatusCompleted,
			Amount:        amount,
			Currency:      utils.TomanCurrency,
			WalletID:      wallet.ID,
			CustomerID:    customer.ID,
			BalanceBefore: beforeMap,
			BalanceAfter:  afterMap,
			Description:   fmt.Sprintf("Budget locked for approved campaign %d", campaign.ID),
			Metadata:      metaBytes,
		}
		if err := s.transactionRepo.Save(txCtx, spendTx); err != nil {
			return err
		}

		campaign.Status = models.CampaignStatusApproved
		campaign.Comment = req.Comment
		campaign.UpdatedAt = utils.ToPtr(utils.UTCNow())
		if err := s.campaignRepo.Update(txCtx, *campaign); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, NewBusinessError("ADMIN_APPROVE_CAMPAIGN_FAILED", "Failed to approve campaign", err)
	}

	// Notify customer (best-effort, outside transaction)
	if s.notifier != nil {
		title := campaign.UUID.String()
		if campaign.Spec.Title != nil && *campaign.Spec.Title != "" {
			title = *campaign.Spec.Title
		}
		customerMobile := normalizeIranMobile(customer.RepresentativeMobile)
		msgCustomer := fmt.Sprintf("Your campaign '%s' has been approved.", title)
		id64 := int64(customer.ID)
		smsCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.notifier.SendSMS(smsCtx, customerMobile, msgCustomer, &id64)
	}

	return &dto.AdminApproveCampaignResponse{Message: "Campaign approved successfully"}, nil
}

// RejectCampaign rejects a campaign: change status to rejected and refund frozen to free
func (s *AdminCampaignFlowImpl) RejectCampaign(ctx context.Context, req *dto.AdminRejectCampaignRequest) (*dto.AdminRejectCampaignResponse, error) {
	if req == nil || req.CampaignID == 0 || strings.TrimSpace(req.Comment) == "" {
		return nil, NewBusinessError("ADMIN_REJECT_CAMPAIGN_FAILED", "campaign_id and comment are required", nil)
	}

	var campaign *models.Campaign
	var customer models.Customer

	err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		var err error
		campaign, err = s.campaignRepo.ByID(txCtx, req.CampaignID)
		if err != nil {
			return err
		}
		if campaign == nil {
			return ErrCampaignNotFound
		}
		if campaign.Status != models.CampaignStatusWaitingForApproval {
			return ErrCampaignNotWaitingForApproval
		}

		customer, err = getCustomer(txCtx, s.customerRepo, campaign.CustomerID)
		if err != nil {
			return err
		}

		// Find the frozen reservation transaction created during finalize
		freezeTxs, err := s.transactionRepo.ByFilter(txCtx, models.TransactionFilter{
			CustomerID: &campaign.CustomerID,
			CampaignID: &campaign.ID,
			Source:     utils.ToPtr("campaign_update"),
			Operation:  utils.ToPtr("reserve_budget"),
			Type:       utils.ToPtr(models.TransactionTypeLaunchCampaign),
			Status:     utils.ToPtr(models.TransactionStatusPending),
		}, "id DESC", 0, 0)
		if err != nil {
			return err
		}
		if len(freezeTxs) == 0 {
			return ErrFreezeTransactionNotFound
		}
		if len(freezeTxs) > 1 {
			return ErrMultipleFreezeTransactionsFound
		}
		freezeTx := freezeTxs[0]

		wallet, err := getWallet(txCtx, s.walletRepo, campaign.CustomerID)
		if err != nil {
			return err
		}
		latestBalance, err := getLatestBalanceSnapshot(txCtx, s.walletRepo, wallet.ID)
		if err != nil {
			return err
		}

		amount := freezeTx.Amount
		if latestBalance.FrozenBalance < amount {
			return ErrInsufficientFunds
		}

		meta := map[string]any{
			"source":      "admin_campaign_reject",
			"operation":   "reject_campaign_refund_frozen",
			"campaign_id": campaign.ID,
			"comment":     req.Comment,
		}
		metaBytes, _ := json.Marshal(meta)

		// Move from frozen back to free
		newFrozen := latestBalance.FrozenBalance - amount
		newCredit := latestBalance.CreditBalance + amount

		newSnap := &models.BalanceSnapshot{
			UUID:          uuid.New(),
			CorrelationID: freezeTx.CorrelationID,
			WalletID:      wallet.ID,
			CustomerID:    customer.ID,
			FreeBalance:   latestBalance.FreeBalance,
			FrozenBalance: newFrozen,
			LockedBalance: latestBalance.LockedBalance,
			CreditBalance: newCredit,
			TotalBalance:  latestBalance.FreeBalance + newFrozen + latestBalance.LockedBalance + newCredit,
			Reason:        "campaign_rejected_budget_refund",
			Description:   fmt.Sprintf("Refund reserved budget for rejected campaign %d", campaign.ID),
			Metadata:      metaBytes,
		}
		if err := s.balanceSnapshotRepo.Save(txCtx, newSnap); err != nil {
			return err
		}

		beforeMap, err := latestBalance.GetBalanceMap()
		if err != nil {
			return err
		}
		afterMap, err := newSnap.GetBalanceMap()
		if err != nil {
			return err
		}

		refundTx := &models.Transaction{
			UUID:          uuid.New(),
			CorrelationID: freezeTx.CorrelationID,
			Type:          models.TransactionTypeUnfreeze,
			Status:        models.TransactionStatusCompleted,
			Amount:        amount,
			Currency:      utils.TomanCurrency,
			WalletID:      wallet.ID,
			CustomerID:    customer.ID,
			BalanceBefore: beforeMap,
			BalanceAfter:  afterMap,
			Description:   fmt.Sprintf("Refund reserved budget for rejected campaign %d", campaign.ID),
			Metadata:      metaBytes,
		}
		if err := s.transactionRepo.Save(txCtx, refundTx); err != nil {
			return err
		}

		campaign.Status = models.CampaignStatusRejected
		campaign.Comment = &req.Comment
		campaign.UpdatedAt = utils.ToPtr(utils.UTCNow())
		if err := s.campaignRepo.Update(txCtx, *campaign); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, NewBusinessError("ADMIN_REJECT_CAMPAIGN_FAILED", "Failed to reject campaign", err)
	}

	// Notify customer and admin (best-effort, outside transaction)
	if s.notifier != nil {
		title := campaign.UUID.String()
		if campaign.Spec.Title != nil && *campaign.Spec.Title != "" {
			title = *campaign.Spec.Title
		}
		customerMobile := normalizeIranMobile(customer.RepresentativeMobile)
		msgCustomer := fmt.Sprintf("Your campaign '%s' has been rejected.", title)
		id64 := int64(customer.ID)
		smsCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.notifier.SendSMS(smsCtx, customerMobile, msgCustomer, &id64)
	}

	return &dto.AdminRejectCampaignResponse{Message: "Campaign rejected and budget refunded successfully"}, nil
}

func normalizeIranMobile(m string) string {
	if m == "" {
		return m
	}
	if strings.HasPrefix(m, "+") {
		return m[1:]
	}
	if strings.HasPrefix(m, "0") && len(m) == 11 {
		return "98" + m[1:]
	}
	return m
}
