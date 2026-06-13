// Package businessflow contains the core business logic and use cases for campaign workflows
package businessflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/app/services"
	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// AdminCampaignFlow handles the campaign business logic
type AdminCampaignFlow interface {
	ListCampaigns(ctx context.Context, filter dto.AdminListCampaignsFilter) (*dto.AdminListCampaignsResponse, error)
	GetCampaign(ctx context.Context, id uint) (*dto.AdminGetCampaignResponse, error)
	ApproveCampaign(ctx context.Context, req *dto.AdminApproveCampaignRequest) (*dto.AdminApproveCampaignResponse, error)
	RejectCampaign(ctx context.Context, req *dto.AdminRejectCampaignRequest) (*dto.AdminRejectCampaignResponse, error)
	CancelCampaign(ctx context.Context, req *dto.AdminCancelCampaignRequest) (*dto.AdminCancelCampaignResponse, error)
	RemoveAudienceSpec(ctx context.Context, platform *string) (*dto.AdminRemoveAudienceSpecResponse, error)
	RescheduleCampaign(ctx context.Context, req *dto.AdminRescheduleCampaignRequest) (*dto.AdminRescheduleCampaignResponse, error)
}

// AdminCampaignFlowImpl implements the campaign business flow
type AdminCampaignFlowImpl struct {
	campaignRepo         repository.CampaignRepository
	customerRepo         repository.CustomerRepository
	walletRepo           repository.WalletRepository
	balanceSnapshotRepo  repository.BalanceSnapshotRepository
	transactionRepo      repository.TransactionRepository
	auditRepo            repository.AuditLogRepository
	platformSettingsRepo repository.PlatformSettingsRepository
	lineNumberRepo       repository.LineNumberRepository
	segmentPriceRepo     repository.SegmentPriceFactorRepository
	notifier             services.NotificationService
	adminConfig          config.AdminConfig
	messageConfig        config.MessageConfig
	cacheConfig          config.CacheConfig
	rc                   *redis.Client
	db                   *gorm.DB
}

var adminTehranLoc *time.Location = time.FixedZone("Asia/Tehran", 3*3600+1800)

// NewAdminCampaignFlow creates a new campaign flow instance
func NewAdminCampaignFlow(
	campaignRepo repository.CampaignRepository,
	customerRepo repository.CustomerRepository,
	walletRepo repository.WalletRepository,
	balanceSnapshotRepo repository.BalanceSnapshotRepository,
	transactionRepo repository.TransactionRepository,
	auditRepo repository.AuditLogRepository,
	platformSettingsRepo repository.PlatformSettingsRepository,
	lineNumberRepo repository.LineNumberRepository,
	segmentPriceRepo repository.SegmentPriceFactorRepository,
	db *gorm.DB,
	rc *redis.Client,
	notifier services.NotificationService,
	adminConfig config.AdminConfig,
	messageConfig config.MessageConfig,
	cacheConfig config.CacheConfig,
) AdminCampaignFlow {
	return &AdminCampaignFlowImpl{
		campaignRepo:         campaignRepo,
		customerRepo:         customerRepo,
		walletRepo:           walletRepo,
		balanceSnapshotRepo:  balanceSnapshotRepo,
		transactionRepo:      transactionRepo,
		auditRepo:            auditRepo,
		platformSettingsRepo: platformSettingsRepo,
		lineNumberRepo:       lineNumberRepo,
		segmentPriceRepo:     segmentPriceRepo,
		notifier:             notifier,
		adminConfig:          adminConfig,
		messageConfig:        messageConfig,
		cacheConfig:          cacheConfig,
		rc:                   rc,
		db:                   db,
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
			return nil, NewBusinessError("ADMIN_LIST_CAMPAIGNS_FAILED", "End date must be after start date", ErrStartDateAfterEndDate)
		}
	}

	rows, err := s.campaignRepo.ByFilter(ctx, cf, "updated_at DESC", 0, 0)
	if err != nil {
		logAdminAction(ctx, s.auditRepo, models.AuditActionAdminCampaignList, "Admin listed campaigns", false, nil, map[string]any{
			"status": filter.Status,
		}, err)
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
			ID:                 c.ID,
			UUID:               c.UUID.String(),
			Status:             c.Status.String(),
			CreatedAt:          c.CreatedAt,
			UpdatedAt:          c.UpdatedAt,
			Title:              c.Spec.Title,
			Level1:             c.Spec.Level1,
			Level2s:            c.Spec.Level2s,
			Level3s:            c.Spec.Level3s,
			Tags:               c.Spec.Tags,
			Sex:                c.Spec.Sex,
			City:               c.Spec.City,
			AdLink:             c.Spec.AdLink,
			Content:            c.Spec.Content,
			ShortLinkDomain:    c.Spec.ShortLinkDomain,
			Category:           c.Spec.Category,
			Job:                c.Spec.Job,
			ScheduleAt:         c.Spec.ScheduleAt,
			LineNumber:         c.Spec.LineNumber,
			MediaUUID:          c.Spec.MediaUUID,
			PlatformSettingsID: c.Spec.PlatformSettingsID,
			Platform:           c.Spec.Platform,
			Budget:             c.Spec.Budget,
			Comment:            c.Comment,
			Statistics:         stats,
			TotalClicks:        &totalClicks,
			ClickRate:          clickRate,
		})
	}

	resp := &dto.AdminListCampaignsResponse{
		Message: "Campaigns retrieved successfully",
		Items:   items,
	}
	logAdminAction(ctx, s.auditRepo, models.AuditActionAdminCampaignList, "Admin listed campaigns", true, nil, map[string]any{
		"items": len(items),
	}, nil)
	return resp, nil
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
		MediaUUID:             c.Spec.MediaUUID,
		PlatformSettingsID:    c.Spec.PlatformSettingsID,
		Platform:              c.Spec.Platform,
		Budget:                c.Spec.Budget,
		Comment:               c.Comment,
		SegmentPriceFactor:    segmentPriceFactor,
		LineNumberPriceFactor: lineNumberPriceFactor,
		Statistics:            stats,
		TotalClicks:           &totalClicks,
		ClickRate:             clickRate,
	}
	logAdminAction(ctx, s.auditRepo, models.AuditActionAdminCampaignGet, "Admin fetched campaign", true, &c.CustomerID, map[string]any{
		"campaign_id": c.ID,
	}, nil)
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
		if err := s.validateApprovalPlatformSettings(txCtx, campaign); err != nil {
			return err
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
		logAdminAction(ctx, s.auditRepo, models.AuditActionAdminCampaignApproved, "Admin approved campaign", false, nil, map[string]any{
			"campaign_id": req.CampaignID,
		}, err)
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

	logAdminAction(ctx, s.auditRepo, models.AuditActionAdminCampaignApproved, "Admin approved campaign", true, &customer.ID, map[string]any{
		"campaign_id": campaign.ID,
		"comment":     req.Comment,
	}, nil)
	return &dto.AdminApproveCampaignResponse{Message: "Campaign approved successfully"}, nil
}

func (s *AdminCampaignFlowImpl) validateApprovalPlatformSettings(ctx context.Context, campaign *models.Campaign) error {
	if campaign == nil {
		return ErrCampaignNotFound
	}

	platform := strings.ToLower(strings.TrimSpace(campaign.Spec.Platform))
	switch platform {
	case models.CampaignPlatformBale, models.CampaignPlatformRubika, models.CampaignPlatformSPlus:
	default:
		return nil
	}

	if campaign.Spec.PlatformSettingsID == nil || *campaign.Spec.PlatformSettingsID == 0 {
		return ErrCampaignPlatformSettingRequired
	}

	settings, err := s.platformSettingsRepo.ByID(ctx, *campaign.Spec.PlatformSettingsID)
	if err != nil {
		return err
	}
	if settings == nil {
		return ErrCampaignPlatformSettingNotFound
	}
	if settings.CustomerID != campaign.CustomerID || strings.ToLower(strings.TrimSpace(settings.Platform)) != platform {
		return ErrCampaignPlatformSettingNotFound
	}

	switch platform {
	case models.CampaignPlatformBale:
		if _, err := parsePositiveIntMetadata(settings.Metadata, "bale_bot_id"); err != nil {
			return fmt.Errorf("campaign platform_settings.metadata.bale_bot_id is required for bale campaigns: %w", err)
		}
	case models.CampaignPlatformRubika:
		if _, err := parseStringMetadata(settings.Metadata, "rubika_service_id"); err != nil {
			return fmt.Errorf("campaign platform_settings.metadata.rubika_service_id is required for rubika campaigns: %w", err)
		}
	case models.CampaignPlatformSPlus:
		if _, err := parseStringMetadata(settings.Metadata, "splus_bot_id"); err != nil {
			return fmt.Errorf("campaign platform_settings.metadata.splus_bot_id is required for splus campaigns: %w", err)
		}
	}

	return nil
}

func parsePositiveIntMetadata(metadata map[string]any, key string) (int64, error) {
	if metadata == nil {
		return 0, fmt.Errorf("metadata is missing")
	}
	raw, ok := metadata[key]
	if !ok {
		return 0, fmt.Errorf("%s is missing", key)
	}

	switch v := raw.(type) {
	case int:
		if v <= 0 {
			return 0, fmt.Errorf("%s must be positive", key)
		}
		return int64(v), nil
	case int64:
		if v <= 0 {
			return 0, fmt.Errorf("%s must be positive", key)
		}
		return v, nil
	case float64:
		if v <= 0 || v != float64(int64(v)) {
			return 0, fmt.Errorf("%s must be a positive integer", key)
		}
		return int64(v), nil
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, fmt.Errorf("%s must not be empty", key)
		}
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer", key)
		}
		return id, nil
	case json.Number:
		id, err := v.Int64()
		if err != nil || id <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer", key)
		}
		return id, nil
	default:
		return 0, fmt.Errorf("%s has unsupported type %T", key, raw)
	}
}

func parseStringMetadata(metadata map[string]any, key string) (string, error) {
	if metadata == nil {
		return "", fmt.Errorf("metadata is missing")
	}
	raw, ok := metadata[key]
	if !ok {
		return "", fmt.Errorf("%s is missing", key)
	}

	switch v := raw.(type) {
	case string:
		out := strings.TrimSpace(v)
		if out == "" {
			return "", fmt.Errorf("%s must not be empty", key)
		}
		return out, nil
	case int:
		if v <= 0 {
			return "", fmt.Errorf("%s must be positive", key)
		}
		return strconv.Itoa(v), nil
	case int64:
		if v <= 0 {
			return "", fmt.Errorf("%s must be positive", key)
		}
		return strconv.FormatInt(v, 10), nil
	case float64:
		if v <= 0 {
			return "", fmt.Errorf("%s must be positive", key)
		}
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case json.Number:
		out := strings.TrimSpace(v.String())
		if out == "" {
			return "", fmt.Errorf("%s must not be empty", key)
		}
		return out, nil
	default:
		return "", fmt.Errorf("%s has unsupported type %T", key, raw)
	}
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
		logAdminAction(ctx, s.auditRepo, models.AuditActionAdminCampaignRejected, "Admin rejected campaign", false, nil, map[string]any{
			"campaign_id": req.CampaignID,
			"comment":     req.Comment,
		}, err)
		return nil, NewBusinessError("ADMIN_REJECT_CAMPAIGN_FAILED", "Failed to reject campaign", err)
	}

	// Notify customer and admin (best-effort, outside transaction)
	if s.notifier != nil {
		title := campaign.UUID.String()
		if campaign.Spec.Title != nil && *campaign.Spec.Title != "" {
			title = *campaign.Spec.Title
		}
		customerMobile := normalizeIranMobile(customer.RepresentativeMobile)
		msgCustomer := strings.TrimSpace(s.messageConfig.CampaignRejectedTemplate)
		if msgCustomer == "" {
			msgCustomer = "Your campaign '%s' has been rejected."
		}
		if strings.Contains(msgCustomer, "%") {
			msgCustomer = fmt.Sprintf(msgCustomer, title)
		}
		id64 := int64(customer.ID)
		smsCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.notifier.SendSMS(smsCtx, customerMobile, msgCustomer, &id64)
		adminMsg := fmt.Sprintf("Campaign rejected:\n%s", title)
		for _, mobile := range s.adminConfig.ActiveMobiles() {
			_ = s.notifier.SendSMS(smsCtx, mobile, adminMsg, nil)
		}
	}

	logAdminAction(ctx, s.auditRepo, models.AuditActionAdminCampaignRejected, "Admin rejected campaign", true, &customer.ID, map[string]any{
		"campaign_id": campaign.ID,
		"comment":     req.Comment,
	}, nil)
	return &dto.AdminRejectCampaignResponse{
		Message: "Campaign rejected and budget refunded successfully",
	}, nil
}

// RescheduleCampaign updates the scheduled time for eligible campaigns (admin action).
func (s *AdminCampaignFlowImpl) RescheduleCampaign(ctx context.Context, req *dto.AdminRescheduleCampaignRequest) (*dto.AdminRescheduleCampaignResponse, error) {
	if req == nil || req.CampaignID == 0 {
		return nil, NewBusinessError("ADMIN_RESCHEDULE_CAMPAIGN_FAILED", "campaign_id is required", ErrCampaignNotFound)
	}

	campaign, err := s.campaignRepo.ByID(ctx, req.CampaignID)
	if err != nil {
		return nil, NewBusinessError("ADMIN_RESCHEDULE_CAMPAIGN_FAILED", "Failed to fetch campaign", err)
	}
	if campaign == nil {
		return nil, ErrCampaignNotFound
	}
	if !isAdminReschedulable(campaign.Status) {
		return nil, ErrCampaignRescheduleNotAllowed
	}

	scheduleUTC := toUTCFromTehran(req.ScheduleAt)
	if scheduleUTC.Before(utils.UTCNow().Add(15 * time.Minute)) {
		return nil, ErrScheduleTimeTooSoon
	}

	tehranTime := scheduleUTC.In(tehranLocation())
	if !isWithinRescheduleWindow(tehranTime) {
		return nil, NewBusinessError("SCHEDULE_TIME_OUTSIDE_WINDOW", "Schedule time must be between 08:00 and 21:00 Asia/Tehran", ErrScheduleTimeOutsideWindow)
	}

	campaign.Spec.ScheduleAt = utils.ToPtr(scheduleUTC)
	campaign.UpdatedAt = utils.ToPtr(utils.UTCNow())

	if err := s.campaignRepo.Update(ctx, *campaign); err != nil {
		logAdminAction(ctx, s.auditRepo, models.AuditActionAdminCampaignRescheduled, "Admin rescheduled campaign", false, &campaign.CustomerID, map[string]any{
			"campaign_id": req.CampaignID,
			"schedule_at": scheduleUTC,
		}, err)
		return nil, NewBusinessError("ADMIN_RESCHEDULE_CAMPAIGN_FAILED", "Failed to reschedule campaign", err)
	}

	logAdminAction(ctx, s.auditRepo, models.AuditActionAdminCampaignRescheduled, "Admin rescheduled campaign", true, &campaign.CustomerID, map[string]any{
		"campaign_id": req.CampaignID,
		"schedule_at": scheduleUTC,
	}, nil)
	return &dto.AdminRescheduleCampaignResponse{
		Message: "Campaign rescheduled successfully",
	}, nil
}

// CancelCampaign cancels an approved campaign: change status to cancelled-by-admin and refund spent budget to customer credit balance.
func (s *AdminCampaignFlowImpl) CancelCampaign(ctx context.Context, req *dto.AdminCancelCampaignRequest) (*dto.AdminCancelCampaignResponse, error) {
	if req == nil || req.CampaignID == 0 || strings.TrimSpace(req.Comment) == "" {
		return nil, NewBusinessError("ADMIN_CANCEL_CAMPAIGN_FAILED", "campaign_id and comment are required", nil)
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
		if campaign.Status != models.CampaignStatusApproved {
			return ErrCampaignNotApproved
		}

		customer, err = getCustomer(txCtx, s.customerRepo, campaign.CustomerID)
		if err != nil {
			return err
		}

		// Find the debit (fee) transaction created when campaign was approved.
		debitTxs, err := s.transactionRepo.ByFilter(txCtx, models.TransactionFilter{
			CustomerID: &campaign.CustomerID,
			CampaignID: &campaign.ID,
			Source:     utils.ToPtr("admin_campaign_approve"),
			Operation:  utils.ToPtr("approve_campaign_budget_consume"),
			Type:       utils.ToPtr(models.TransactionTypeFee),
			Status:     utils.ToPtr(models.TransactionStatusCompleted),
		}, "id DESC", 0, 0)
		if err != nil {
			return err
		}
		if len(debitTxs) == 0 {
			return ErrCampaignDebitTransactionNotFound
		}
		if len(debitTxs) > 1 {
			return ErrMultipleCampaignDebitTransactionsFound
		}
		debitTx := debitTxs[0]

		wallet, err := getWallet(txCtx, s.walletRepo, campaign.CustomerID)
		if err != nil {
			return err
		}
		latestBalance, err := getLatestBalanceSnapshot(txCtx, s.walletRepo, wallet.ID)
		if err != nil {
			return err
		}

		amount := debitTx.Amount
		if latestBalance.SpentOnCampaign < amount {
			return ErrInsufficientFunds
		}

		meta := map[string]any{
			"source":      "admin_campaign_cancel",
			"operation":   "cancel_campaign_refund_spent",
			"campaign_id": campaign.ID,
			"comment":     req.Comment,
		}
		metaBytes, _ := json.Marshal(meta)

		// Move spent amount back to customer credit.
		newCredit := latestBalance.CreditBalance + amount
		newSpentOnCampaign := latestBalance.SpentOnCampaign - amount

		newSnap := &models.BalanceSnapshot{
			UUID:               uuid.New(),
			CorrelationID:      debitTx.CorrelationID,
			WalletID:           wallet.ID,
			CustomerID:         customer.ID,
			FreeBalance:        latestBalance.FreeBalance,
			FrozenBalance:      latestBalance.FrozenBalance,
			LockedBalance:      latestBalance.LockedBalance,
			CreditBalance:      newCredit,
			SpentOnCampaign:    newSpentOnCampaign,
			AgencyShareWithTax: latestBalance.AgencyShareWithTax,
			TotalBalance:       latestBalance.FreeBalance + latestBalance.FrozenBalance + latestBalance.LockedBalance + newCredit + newSpentOnCampaign + latestBalance.AgencyShareWithTax,
			Reason:             "campaign_cancelled_by_admin_budget_refund",
			Description:        fmt.Sprintf("Refund spent budget for admin-cancelled campaign %d", campaign.ID),
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
			CorrelationID: debitTx.CorrelationID,
			Type:          models.TransactionTypeRefund,
			Status:        models.TransactionStatusCompleted,
			Amount:        amount,
			Currency:      utils.TomanCurrency,
			WalletID:      wallet.ID,
			CustomerID:    customer.ID,
			BalanceBefore: beforeMap,
			BalanceAfter:  afterMap,
			Description:   fmt.Sprintf("Refund spent budget for admin-cancelled campaign %d", campaign.ID),
			Metadata:      metaBytes,
		}
		if err := s.transactionRepo.Save(txCtx, refundTx); err != nil {
			return err
		}

		campaign.Status = models.CampaignStatusCancelledByAdmin
		campaign.Comment = &req.Comment
		campaign.UpdatedAt = utils.ToPtr(utils.UTCNow())
		if err := s.campaignRepo.Update(txCtx, *campaign); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		logAdminAction(ctx, s.auditRepo, models.AuditActionAdminCampaignCancelled, "Admin cancelled campaign", false, nil, map[string]any{
			"campaign_id": req.CampaignID,
			"comment":     req.Comment,
		}, err)
		return nil, NewBusinessError("ADMIN_CANCEL_CAMPAIGN_FAILED", "Failed to cancel campaign", err)
	}

	// Notify customer/admin (best effort)
	if s.notifier != nil {
		title := campaign.UUID.String()
		if campaign.Spec.Title != nil && *campaign.Spec.Title != "" {
			title = *campaign.Spec.Title
		}
		customerMobile := normalizeIranMobile(customer.RepresentativeMobile)
		msgCustomer := fmt.Sprintf("Your campaign '%s' has been cancelled by admin.", title)
		id64 := int64(customer.ID)
		smsCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.notifier.SendSMS(smsCtx, customerMobile, msgCustomer, &id64)
		adminMsg := fmt.Sprintf("Campaign cancelled by admin:\n%s", title)
		for _, mobile := range s.adminConfig.ActiveMobiles() {
			_ = s.notifier.SendSMS(smsCtx, mobile, adminMsg, nil)
		}
	}

	logAdminAction(ctx, s.auditRepo, models.AuditActionAdminCampaignCancelled, "Admin cancelled campaign", true, &customer.ID, map[string]any{
		"campaign_id": campaign.ID,
		"comment":     req.Comment,
	}, nil)
	return &dto.AdminCancelCampaignResponse{
		Message: "Campaign cancelled and budget refunded successfully",
	}, nil
}

func (s *AdminCampaignFlowImpl) RemoveAudienceSpec(ctx context.Context, platform *string) (*dto.AdminRemoveAudienceSpecResponse, error) {
	if s.rc == nil {
		return nil, NewBusinessError("ADMIN_REMOVE_AUDIENCE_SPEC_FAILED", "Cache config is not available", ErrCacheNotAvailable)
	}

	normalizedPlatform, err := normalizeAudienceSpecPlatformDefault(platform)
	if err != nil {
		return nil, NewBusinessError("ADMIN_REMOVE_AUDIENCE_SPEC_INVALID_PLATFORM", "Invalid platform", ErrAudienceSpecPlatformInvalid)
	}

	lockKey := audienceSpecPlatformLockKey(s.cacheConfig, normalizedPlatform)
	cacheKey := audienceSpecPlatformCacheKey(s.cacheConfig, normalizedPlatform)
	filePath := audienceSpecFilePath()

	ok, err := s.rc.SetNX(ctx, lockKey, "1", 10*time.Second).Result()
	if err != nil {
		return nil, NewBusinessError("ADMIN_REMOVE_AUDIENCE_SPEC_LOCK_FAILED", "Failed to acquire lock", err)
	}
	if !ok {
		return nil, NewBusinessError("ADMIN_REMOVE_AUDIENCE_SPEC_LOCK_BUSY", "Another worker is updating audience spec", errors.New("lock busy"))
	}
	defer func() {
		_ = s.rc.Del(context.Background(), lockKey).Err()
	}()

	currentByPlatform, err := readAudienceSpecFileByPlatform(filePath)
	if err != nil {
		return nil, NewBusinessError("ADMIN_REMOVE_AUDIENCE_SPEC_READ_FAILED", "Failed to read audience spec file", err)
	}
	delete(currentByPlatform, normalizedPlatform)

	bytes, err := json.MarshalIndent(currentByPlatform, "", "  ")
	if err != nil {
		return nil, NewBusinessError("ADMIN_REMOVE_AUDIENCE_SPEC_MARSHAL_FAILED", "Failed to marshal audience spec", err)
	}
	if err := atomicWrite(filePath, bytes, 0o644); err != nil {
		return nil, NewBusinessError("ADMIN_REMOVE_AUDIENCE_SPEC_WRITE_FAILED", "Failed to write audience spec file", err)
	}

	if err := s.rc.Del(ctx, cacheKey).Err(); err != nil {
		return nil, NewBusinessError("ADMIN_REMOVE_AUDIENCE_SPEC_CACHE_FAILED", "Failed to clear audience spec cache", err)
	}

	resp := &dto.AdminRemoveAudienceSpecResponse{
		Message:  "Audience spec removed successfully",
		Platform: normalizedPlatform,
	}
	logAdminAction(ctx, s.auditRepo, models.AuditActionAdminRemoveAudienceSpec, "Admin removed audience spec", true, nil, map[string]any{
		"platform": normalizedPlatform,
	}, nil)
	return resp, nil
}

func isAdminReschedulable(status models.CampaignStatus) bool {
	switch status {
	case models.CampaignStatusInProgress, models.CampaignStatusWaitingForApproval, models.CampaignStatusApproved:
		return true
	default:
		return false
	}
}

func toUTCFromTehran(t time.Time) time.Time {
	loc := tehranLocation()
	tehranTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc)
	return tehranTime.UTC()
}

func tehranLocation() *time.Location {
	if adminTehranLoc == nil || adminTehranLoc.String() != "Asia/Tehran" {
		if loaded, err := time.LoadLocation("Asia/Tehran"); err == nil {
			adminTehranLoc = loaded
		} else {
			adminTehranLoc = time.FixedZone("Asia/Tehran", 3*3600+1800)
		}
	}
	return adminTehranLoc
}

func isWithinRescheduleWindow(tehranTime time.Time) bool {
	hour := tehranTime.Hour()
	if hour < 8 {
		return false
	}
	if hour > 21 {
		return false
	}
	if hour == 21 && (tehranTime.Minute() > 0 || tehranTime.Second() > 0 || tehranTime.Nanosecond() > 0) {
		return false
	}
	return true
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
