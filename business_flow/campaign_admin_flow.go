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
	GetCampaign(ctx context.Context, id uint) (*dto.AdminGetCampaignResponse, error)
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
	lineNumberRepo      repository.LineNumberRepository
	segmentPriceRepo    repository.SegmentPriceFactorRepository
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
	lineNumberRepo repository.LineNumberRepository,
	segmentPriceRepo repository.SegmentPriceFactorRepository,
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
		lineNumberRepo:      lineNumberRepo,
		segmentPriceRepo:    segmentPriceRepo,
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
	// Click counts and stats
	campaignIDs := make([]uint, 0, len(rows))
	for _, c := range rows {
		campaignIDs = append(campaignIDs, c.ID)
	}
	clickCounts, _ := s.campaignRepo.AggregateClickCountsByCampaignIDs(ctx, campaignIDs)

	items := make([]dto.AdminGetCampaignResponse, 0, len(rows))
	for _, c := range rows {
		var stats map[string]any
		if len(c.Statistics) > 0 {
			_ = json.Unmarshal(c.Statistics, &stats)
		}
		clicks := clickCounts[c.ID]
		var clickRate *float64
		totalSent := float64(0)
		if v, ok := stats["aggregatedTotalSent"]; ok {
			switch n := v.(type) {
			case float64:
				totalSent = n
			case int64:
				totalSent = float64(n)
			case json.Number:
				if f, e := n.Float64(); e == nil {
					totalSent = f
				}
			}
		}
		if totalSent > 0 {
			val := float64(clicks) / totalSent
			clickRate = &val
		}
		totalClicks := clicks

		items = append(items, dto.AdminGetCampaignResponse{
			ID:              c.ID,
			UUID:            c.UUID.String(),
			Status:          c.Status.String(),
			CreatedAt:       c.CreatedAt,
			UpdatedAt:       c.UpdatedAt,
			Title:           c.Spec.Title,
			Level1:          c.Spec.Level1,
			Level2s:         c.Spec.Level2s,
			Level3s:         c.Spec.Level3s,
			Tags:            c.Spec.Tags,
			Sex:             c.Spec.Sex,
			City:            c.Spec.City,
			AdLink:          c.Spec.AdLink,
			Content:         c.Spec.Content,
			ShortLinkDomain: c.Spec.ShortLinkDomain,
			Category:        c.Spec.Category,
			Job:             c.Spec.Job,
			ScheduleAt:      c.Spec.ScheduleAt,
			LineNumber:      c.Spec.LineNumber,
			Budget:          c.Spec.Budget,
			Comment:         c.Comment,
			Statistics:      stats,
			TotalClicks:     &totalClicks,
			ClickRate:       clickRate,
		})
	}

	return &dto.AdminListCampaignsResponse{
		Message: "Campaigns retrieved successfully",
		Items:   items,
	}, nil
}

// GetCampaign retrieves a single campaign by ID for admin
func (s *AdminCampaignFlowImpl) GetCampaign(ctx context.Context, id uint) (*dto.AdminGetCampaignResponse, error) {
	c, err := s.campaignRepo.ByID(ctx, id)
	if err != nil {
		return nil, NewBusinessError("ADMIN_GET_CAMPAIGN_FAILED", "Failed to get campaign", err)
	}
	if c == nil {
		return nil, ErrCampaignNotFound
	}
	var stats map[string]any
	if len(c.Statistics) > 0 {
		_ = json.Unmarshal(c.Statistics, &stats)
	}
	clickCounts, _ := s.campaignRepo.AggregateClickCountsByCampaignIDs(ctx, []uint{id})
	clicks := clickCounts[id]
	var clickRate *float64
	totalSent := float64(0)
	if v, ok := stats["aggregatedTotalSent"]; ok {
		switch n := v.(type) {
		case float64:
			totalSent = n
		case int64:
			totalSent = float64(n)
		case json.Number:
			if f, e := n.Float64(); e == nil {
				totalSent = f
			}
		}
	}
	if totalSent > 0 {
		val := float64(clicks) / totalSent
		clickRate = &val
	}
	totalClicks := clicks
	metaSegment, metaLine, err := s.readCampaignPriceFactorsFromMetadata(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	segmentPriceFactor := float64(-1)
	if metaSegment != nil {
		segmentPriceFactor = *metaSegment
	}
	// else if len(c.Spec.Level3s) > 0 {
	// 	factors, err := s.segmentPriceRepo.LatestByLevel3s(ctx, c.Spec.Level3s)
	// 	if err != nil {
	// 		return nil, NewBusinessError("ADMIN_GET_CAMPAIGN_FAILED", "Failed to get segment price factor", err)
	// 	}
	// 	maxFactor := float64(0)
	// 	for _, l3 := range c.Spec.Level3s {
	// 		if f, ok := factors[l3]; ok && f > maxFactor {
	// 			maxFactor = f
	// 		}
	// 	}
	// 	if maxFactor > 0 {
	// 		segmentPriceFactor = maxFactor
	// 	}
	// }
	lineNumberPriceFactor := float64(-1)
	if metaLine != nil {
		lineNumberPriceFactor = *metaLine
	}
	// else if c.Spec.LineNumber != nil {
	// 	lineNumber, err := s.lineNumberRepo.ByValue(ctx, *c.Spec.LineNumber)
	// 	if err != nil {
	// 		return nil, NewBusinessError("ADMIN_GET_CAMPAIGN_FAILED", "Failed to get line number price factor", err)
	// 	}
	// 	if lineNumber != nil {
	// 		lineNumberPriceFactor = lineNumber.PriceFactor
	// 	}
	// }

	resp := &dto.AdminGetCampaignResponse{
		ID:                    c.ID,
		UUID:                  c.UUID.String(),
		Status:                c.Status.String(),
		CreatedAt:             c.CreatedAt,
		UpdatedAt:             c.UpdatedAt,
		Title:                 c.Spec.Title,
		Level1:                c.Spec.Level1,
		Level2s:               c.Spec.Level2s,
		Level3s:               c.Spec.Level3s,
		Tags:                  c.Spec.Tags,
		Sex:                   c.Spec.Sex,
		City:                  c.Spec.City,
		AdLink:                c.Spec.AdLink,
		Content:               c.Spec.Content,
		ShortLinkDomain:       c.Spec.ShortLinkDomain,
		Category:              c.Spec.Category,
		Job:                   c.Spec.Job,
		ScheduleAt:            c.Spec.ScheduleAt,
		LineNumber:            c.Spec.LineNumber,
		Budget:                c.Spec.Budget,
		Comment:               c.Comment,
		SegmentPriceFactor:    segmentPriceFactor,
		LineNumberPriceFactor: lineNumberPriceFactor,
		Statistics:            stats,
		TotalClicks:           &totalClicks,
		ClickRate:             clickRate,
	}
	return resp, nil
}

func (s *AdminCampaignFlowImpl) readCampaignPriceFactorsFromMetadata(ctx context.Context, campaignID uint) (*float64, *float64, error) {
	source := "campaign_update"
	operation := "reserve_budget"
	txs, err := s.transactionRepo.ByFilter(ctx, models.TransactionFilter{
		CampaignID: &campaignID,
		Source:     &source,
		Operation:  &operation,
	}, "id DESC", 1, 0)
	if err != nil {
		return nil, nil, NewBusinessError("ADMIN_GET_CAMPAIGN_FAILED", "Failed to get campaign metadata", err)
	}
	if len(txs) == 0 || len(txs[0].Metadata) == 0 {
		return nil, nil, nil
	}

	var meta map[string]any
	if err := json.Unmarshal(txs[0].Metadata, &meta); err != nil {
		return nil, nil, nil
	}

	segmentPriceFactor := parseMetadataFloat(meta["segment_price_factor"])
	lineNumberPriceFactor := parseMetadataFloat(meta["line_number_price_factor"])
	return segmentPriceFactor, lineNumberPriceFactor, nil
}

func parseMetadataFloat(value any) *float64 {
	switch v := value.(type) {
	case float64:
		return &v
	case int64:
		f := float64(v)
		return &f
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return &f
		}
	}
	return nil
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
			Type:       utils.ToPtr(models.TransactionTypeFreeze),
			Status:     utils.ToPtr(models.TransactionStatusCompleted),
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

		newFrozen := latestBalance.FrozenBalance - amount
		newSpentOnCampaign := latestBalance.SpentOnCampaign + amount

		newSnap := &models.BalanceSnapshot{
			UUID:               uuid.New(),
			CorrelationID:      freezeTx.CorrelationID,
			WalletID:           wallet.ID,
			CustomerID:         customer.ID,
			FreeBalance:        latestBalance.FreeBalance,
			FrozenBalance:      newFrozen,
			LockedBalance:      latestBalance.LockedBalance,
			CreditBalance:      latestBalance.CreditBalance,
			SpentOnCampaign:    newSpentOnCampaign,
			AgencyShareWithTax: latestBalance.AgencyShareWithTax,
			TotalBalance:       latestBalance.FreeBalance + newFrozen + latestBalance.LockedBalance + latestBalance.CreditBalance + newSpentOnCampaign + latestBalance.AgencyShareWithTax,
			Reason:             "campaign_approved_budget_spent_on_campaign",
			Description:        fmt.Sprintf("Budget spent on approved campaign %d", campaign.ID),
			Metadata:           metaBytes,
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

		feeTx := &models.Transaction{
			UUID:          uuid.New(),
			CorrelationID: freezeTx.CorrelationID,
			Type:          models.TransactionTypeFee,
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
		if err := s.transactionRepo.Save(txCtx, feeTx); err != nil {
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
			Type:       utils.ToPtr(models.TransactionTypeFreeze),
			Status:     utils.ToPtr(models.TransactionStatusCompleted),
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
			UUID:               uuid.New(),
			CorrelationID:      freezeTx.CorrelationID,
			WalletID:           wallet.ID,
			CustomerID:         customer.ID,
			FreeBalance:        latestBalance.FreeBalance,
			FrozenBalance:      newFrozen,
			LockedBalance:      latestBalance.LockedBalance,
			CreditBalance:      newCredit,
			SpentOnCampaign:    latestBalance.SpentOnCampaign,
			AgencyShareWithTax: latestBalance.AgencyShareWithTax,
			TotalBalance:       latestBalance.FreeBalance + newFrozen + latestBalance.LockedBalance + newCredit + latestBalance.SpentOnCampaign + latestBalance.AgencyShareWithTax,
			Reason:             "campaign_rejected_budget_refund",
			Description:        fmt.Sprintf("Refund reserved budget for rejected campaign %d", campaign.ID),
			Metadata:           metaBytes,
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
			Type:          models.TransactionTypeRefund,
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

	return &dto.AdminRejectCampaignResponse{
		Message: "Campaign rejected and budget refunded successfully",
	}, nil
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
