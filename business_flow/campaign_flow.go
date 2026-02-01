// Package businessflow contains the core business logic and use cases for campaign workflows
package businessflow

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
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

// CampaignFlow handles the campaign business logic
type CampaignFlow interface {
	CreateCampaign(ctx context.Context, req *dto.CreateCampaignRequest, metadata *ClientMetadata) (*dto.CreateCampaignResponse, error)
	UpdateCampaign(ctx context.Context, req *dto.UpdateCampaignRequest, metadata *ClientMetadata) (*dto.UpdateCampaignResponse, error)
	CalculateCampaignCapacity(ctx context.Context, req *dto.CalculateCampaignCapacityRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCapacityResponse, error)
	CalculateCampaignCost(ctx context.Context, req *dto.CalculateCampaignCostRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCostResponse, error)
	ListCampaigns(ctx context.Context, req *dto.ListCampaignsRequest, metadata *ClientMetadata) (*dto.ListCampaignsResponse, error)
	ListAudienceSpec(ctx context.Context) (*dto.ListAudienceSpecResponse, error)
	GetApprovedRunningSummary(ctx context.Context, customerID uint) (*dto.CampaignsSummaryResponse, error)
	CancelCampaign(ctx context.Context, req *dto.CancelCampaignRequest, metadata *ClientMetadata) (*dto.CancelCampaignResponse, error)
}

// CampaignFlowImpl implements the campaign business flow
type CampaignFlowImpl struct {
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
	cacheConfig         *config.CacheConfig
	rc                  *redis.Client
	db                  *gorm.DB
}

// NewCampaignFlow creates a new campaign flow instance
func NewCampaignFlow(
	campaignRepo repository.CampaignRepository,
	customerRepo repository.CustomerRepository,
	walletRepo repository.WalletRepository,
	balanceSnapshotRepo repository.BalanceSnapshotRepository,
	transactionRepo repository.TransactionRepository,
	auditRepo repository.AuditLogRepository,
	lineNumberRepo repository.LineNumberRepository,
	segmentPriceRepo repository.SegmentPriceFactorRepository,
	db *gorm.DB,
	rc *redis.Client,
	notifier services.NotificationService,
	adminConfig config.AdminConfig,
	cacheConfig *config.CacheConfig,
) CampaignFlow {
	return &CampaignFlowImpl{
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
		cacheConfig:         cacheConfig,
		rc:                  rc,
		db:                  db,
	}
}

// CreateCampaign handles the complete campaign creation process
func (s *CampaignFlowImpl) CreateCampaign(ctx context.Context, req *dto.CreateCampaignRequest, metadata *ClientMetadata) (*dto.CreateCampaignResponse, error) {
	// Validate business rules
	if err := s.validateCreateCampaignRequest(req); err != nil {
		return nil, NewBusinessError("CAMPAIGN_VALIDATION_FAILED", "Campaign validation failed", err)
	}

	customer, err := getCustomer(ctx, s.customerRepo, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CUSTOMER_LOOKUP_FAILED", "Failed to lookup customer", err)
	}

	// Use transaction for atomicity
	var campaign *models.Campaign

	err = repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		var err error
		campaign, err = s.createCampaign(txCtx, req, &customer)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Campaign creation failed: %s", err.Error())
		_ = s.createAuditLog(ctx, &customer, models.AuditActionCampaignCreationFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("CAMPAIGN_CREATION_FAILED", "Campaign creation failed", err)
	}

	// Log successful creation
	msg := fmt.Sprintf("Campaign created successfully: %s", campaign.UUID.String())
	_ = s.createAuditLog(ctx, &customer, models.AuditActionCampaignCreated, msg, true, nil, metadata)

	// Build resp
	resp := &dto.CreateCampaignResponse{
		Message:   "Campaign created successfully",
		ID:        campaign.ID,
		UUID:      campaign.UUID.String(),
		Status:    string(campaign.Status),
		CreatedAt: campaign.CreatedAt.Format(time.RFC3339),
	}

	return resp, nil
}

// UpdateCampaign handles the campaign update process
func (s *CampaignFlowImpl) UpdateCampaign(ctx context.Context, req *dto.UpdateCampaignRequest, metadata *ClientMetadata) (*dto.UpdateCampaignResponse, error) {
	// Validate business rules
	if err := s.validateUpdateCampaignRequest(req); err != nil {
		return nil, NewBusinessError("CAMPAIGN_UPDATE_VALIDATION_FAILED", "Campaign update validation failed", err)
	}

	// Get customer
	customer, err := getCustomer(ctx, s.customerRepo, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CUSTOMER_LOOKUP_FAILED", "Failed to lookup customer", err)
	}

	// Get existing campaign
	campaign, err := getCampaign(ctx, s.campaignRepo, req.UUID, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_LOOKUP_FAILED", "Failed to lookup campaign", err)
	}

	// Check if campaign can be updated (only initiated campaigns can be updated)
	if !canUpdateCampaign(campaign.Status) {
		return nil, NewBusinessError("CAMPAIGN_UPDATE_NOT_ALLOWED", "Campaign cannot be updated in current status", ErrCampaignUpdateNotAllowed)
	}

	if req.ScheduleAt == nil && campaign.Spec.ScheduleAt == nil {
		req.ScheduleAt = utils.ToPtr(utils.UTCNow().Add(time.Hour))
		campaign.Spec.ScheduleAt = req.ScheduleAt
	}

	// Validate schedule time must be at least 10 minutes in the future
	scheduleTime := req.ScheduleAt
	if scheduleTime == nil {
		scheduleTime = campaign.Spec.ScheduleAt
	}
	if scheduleTime != nil && !scheduleTime.IsZero() {
		if scheduleTime.Before(utils.UTCNow().Add(10 * time.Minute)) {
			return nil, NewBusinessError("INVALID_SCHEDULE_TIME", "Schedule time must be at least 10 minutes in the future", ErrScheduleTimeTooSoon)
		}
	}

	var (
		title      = req.Title
		level1     = req.Level1
		level2s    = req.Level2s
		level3s    = req.Level3s
		tags       = req.Tags
		sex        = req.Sex
		city       = req.City
		adLink     = req.AdLink
		content    = req.Content
		scheduleAt = req.ScheduleAt
		lineNumber = req.LineNumber
		budget     = req.Budget
	)
	if req.Title == nil {
		title = campaign.Spec.Title
	}
	if req.Level1 == nil {
		level1 = campaign.Spec.Level1
	}
	if req.Level2s == nil {
		level2s = campaign.Spec.Level2s
	}
	if req.Level3s == nil {
		level3s = campaign.Spec.Level3s
	}
	if req.Tags == nil {
		tags = campaign.Spec.Tags
	}
	if req.Sex == nil {
		sex = campaign.Spec.Sex
	}
	if req.City == nil {
		city = campaign.Spec.City
	}
	if req.AdLink == nil {
		adLink = campaign.Spec.AdLink
	}
	if req.Content == nil {
		content = campaign.Spec.Content
	}
	if req.ScheduleAt == nil {
		scheduleAt = campaign.Spec.ScheduleAt
	}
	if req.LineNumber == nil {
		lineNumber = campaign.Spec.LineNumber
	}
	if req.Budget == nil {
		budget = campaign.Spec.Budget
	}

	capacity, err := s.CalculateCampaignCapacity(ctx, &dto.CalculateCampaignCapacityRequest{
		Title:      title,
		Level1:     level1,
		Level2s:    level2s,
		Level3s:    level3s,
		Tags:       tags,
		Sex:        sex,
		City:       city,
		AdLink:     adLink,
		Content:    content,
		ScheduleAt: scheduleAt,
		LineNumber: lineNumber,
		Budget:     budget,
	}, metadata)
	if err != nil {
		return nil, NewBusinessError("CAPACITY_CALCULATION_FAILED", "Failed to calculate campaign capacity", err)
	}
	if capacity.Capacity < utils.MinAcceptableCampaignCapacity {
		return nil, NewBusinessError("INSUFFICIENT_CAPACITY", "Insufficient campaign capacity", ErrInsufficientCampaignCapacity)
	}

	// TODO: validate content, part, link, line number, etc

	// Use transaction for atomicity
	err = repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		// Update campaign
		if err := s.updateCampaign(txCtx, req, &campaign); err != nil {
			return err
		}

		if req.Finalize != nil && *req.Finalize {
			// TODO: validate all required fields are present like line number, budget
			// schedule time must be present and be in future
			// title (len), sex (choice), level1 level2s level3s (db, len), tags (db, len), city (db, len), adlink (len), content (len), line number (db, len), budget (min, max)

			// retrieve campaign again (last state)
			campaign, err := getCampaign(txCtx, s.campaignRepo, req.UUID, req.CustomerID)
			if err != nil {
				return err
			}

			if err := s.canFinalizeCampaign(campaign); err != nil {
				return err
			}

			if req.LineNumber != nil {
				// query line number exists
				lineNumber, err := s.lineNumberRepo.ByValue(txCtx, *req.LineNumber)
				if err != nil {
					return err
				}
				if lineNumber == nil {
					return ErrLineNumberNotFound
				}
				if !*lineNumber.IsActive {
					return ErrLineNumberNotActive
				}
			}

			lineNumberPriceFactor, err := s.fetchLineNumberPriceFactor(txCtx, *req.LineNumber)
			if err != nil {
				return err
			}

			cost, err := s.CalculateCampaignCost(txCtx, &dto.CalculateCampaignCostRequest{
				Title:      title,
				Level1:     level1,
				Level2s:    level2s,
				Level3s:    level3s,
				Tags:       tags,
				Sex:        sex,
				City:       city,
				AdLink:     adLink,
				Content:    content,
				ScheduleAt: scheduleAt,
				LineNumber: lineNumber,
				Budget:     budget,
			}, metadata)
			if err != nil {
				return err
			}

			campaign.Status = models.CampaignStatusWaitingForApproval
			campaign.NumAudience = utils.ToPtr(cost.NumTargetAudience)
			campaign.UpdatedAt = utils.ToPtr(utils.UTCNow())
			if err := s.campaignRepo.Update(txCtx, campaign); err != nil {
				return err
			}

			// Fetch wallet free balance
			wallet, err := getWallet(txCtx, s.walletRepo, req.CustomerID)
			if err != nil {
				return err
			}
			// Update customer with wallet reference
			customer.Wallet = &wallet

			latestBalance, err := getLatestBalanceSnapshot(txCtx, s.walletRepo, wallet.ID)
			if err != nil {
				return err
			}

			availableBalance := latestBalance.FreeBalance + latestBalance.CreditBalance
			if availableBalance < cost.TotalCost {
				return ErrInsufficientFunds
			}

			newFreeBalance := latestBalance.FreeBalance
			newCreditBalance := latestBalance.CreditBalance
			remaining := cost.TotalCost

			if remaining <= newFreeBalance {
				newFreeBalance -= remaining
			} else {
				remaining -= newFreeBalance
				newFreeBalance = 0
				newCreditBalance -= remaining
			}
			newFrozenBalance := latestBalance.FrozenBalance + cost.TotalCost

			// -------------------
			// -------------------
			// TODO: ALSO add formula for calculating capacity and cost (I mean, parameters)
			// -------------------
			// -------------------

			// Build metadata with full campaign spec
			meta := map[string]any{
				"source":                   "campaign_update",
				"operation":                "reserve_budget",
				"campaign_id":              campaign.ID,
				"amount":                   cost.TotalCost,
				"currency":                 utils.TomanCurrency,
				"campaign_spec":            campaign.Spec,
				"line_number_price_factor": lineNumberPriceFactor,
			}
			metaBytes, _ := json.Marshal(meta)

			corrID := uuid.New()

			newSnapshot := &models.BalanceSnapshot{
				UUID:               uuid.New(),
				CorrelationID:      corrID,
				WalletID:           wallet.ID,
				CustomerID:         customer.ID,
				FreeBalance:        newFreeBalance,
				FrozenBalance:      newFrozenBalance,
				CreditBalance:      newCreditBalance,
				LockedBalance:      latestBalance.LockedBalance,
				SpentOnCampaign:    latestBalance.SpentOnCampaign,
				AgencyShareWithTax: latestBalance.AgencyShareWithTax,
				TotalBalance:       newFreeBalance + newFrozenBalance + newCreditBalance + latestBalance.LockedBalance + latestBalance.SpentOnCampaign + latestBalance.AgencyShareWithTax,
				Reason:             "campaign_budget_reserved_waiting_for_approval",
				Description:        fmt.Sprintf("Budget reserved for campaign %d", campaign.ID),
				Metadata:           metaBytes,
				CreatedAt:          utils.UTCNow(),
				UpdatedAt:          utils.UTCNow(),
			}
			if err := s.balanceSnapshotRepo.Save(txCtx, newSnapshot); err != nil {
				return err
			}

			beforeMap, err := latestBalance.GetBalanceMap()
			if err != nil {
				return err
			}
			afterMap, err := newSnapshot.GetBalanceMap()
			if err != nil {
				return err
			}

			freezeTx := &models.Transaction{
				UUID:          uuid.New(),
				CorrelationID: corrID,
				Type:          models.TransactionTypeFreeze,
				Status:        models.TransactionStatusCompleted,
				Amount:        cost.TotalCost,
				Currency:      utils.TomanCurrency,
				WalletID:      wallet.ID,
				CustomerID:    customer.ID,
				BalanceBefore: beforeMap,
				BalanceAfter:  afterMap,
				Description:   fmt.Sprintf("Campaign budget reserved: %d Tomans for campaign %d", cost.TotalCost, campaign.ID),
				Metadata:      metaBytes,
				CreatedAt:     utils.UTCNow(),
				UpdatedAt:     utils.UTCNow(),
			}
			if err := s.transactionRepo.Save(txCtx, freezeTx); err != nil {
				return err
			}

			// Notify admin about new campaign awaiting approval
			if s.notifier != nil && s.adminConfig.Mobile != "" {
				subject := campaign.UUID.String()
				if campaign.Spec.Title != nil {
					subject = *campaign.Spec.Title
				}
				msg := fmt.Sprintf("New campaign pending approval:\n%s", subject)
				_ = s.notifier.SendSMS(txCtx, s.adminConfig.Mobile, msg, nil)
				// TODO: Resend?
			}
		}

		return nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Campaign update failed for campaign %d: %s", campaign.ID, err.Error())
		_ = s.createAuditLog(ctx, &customer, models.AuditActionCampaignUpdateFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("CAMPAIGN_UPDATE_FAILED", "Campaign update failed", err)
	}

	// Log successful update
	msg := fmt.Sprintf("Campaign updated successfully: %d", campaign.ID)
	_ = s.createAuditLog(ctx, &customer, models.AuditActionCampaignUpdated, msg, true, nil, metadata)

	// Build resp
	resp := &dto.UpdateCampaignResponse{
		Message: "Campaign updated successfully",
	}

	return resp, nil
}

// CancelCampaign allows a customer to cancel their own campaign waiting for approval and refunds the reserved budget.
func (s *CampaignFlowImpl) CancelCampaign(ctx context.Context, req *dto.CancelCampaignRequest, metadata *ClientMetadata) (*dto.CancelCampaignResponse, error) {
	if req == nil || req.CampaignID == 0 {
		return nil, NewBusinessError("CANCEL_CAMPAIGN_VALIDATION_FAILED", "campaign_id is required", ErrCampaignNotFound)
	}

	var campaign *models.Campaign

	err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		var err error
		campaign, err = s.campaignRepo.ByID(txCtx, req.CampaignID)
		if err != nil {
			return err
		}
		if campaign == nil {
			return ErrCampaignNotFound
		}
		if campaign.CustomerID != req.CustomerID {
			return ErrCampaignAccessDenied
		}
		if campaign.Status != models.CampaignStatusWaitingForApproval {
			return ErrCampaignNotWaitingForApproval
		}

		customer, err := getCustomer(txCtx, s.customerRepo, campaign.CustomerID)
		if err != nil {
			return err
		}

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
			"source":      "campaign_cancel",
			"operation":   "cancel_campaign_refund_frozen",
			"campaign_id": campaign.ID,
			"comment":     req.Comment,
		}
		metaBytes, _ := json.Marshal(meta)

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
			Reason:             "campaign_cancelled_budget_refund",
			Description:        fmt.Sprintf("Refund reserved budget for cancelled campaign %d", campaign.ID),
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
			Description:   fmt.Sprintf("Refund reserved budget for cancelled campaign %d", campaign.ID),
			Metadata:      metaBytes,
		}
		if err := s.transactionRepo.Save(txCtx, refundTx); err != nil {
			return err
		}

		campaign.Status = models.CampaignStatusCancelled
		if req.Comment != nil && strings.TrimSpace(*req.Comment) != "" {
			comment := strings.TrimSpace(*req.Comment)
			campaign.Comment = &comment
		}
		campaign.UpdatedAt = utils.ToPtr(utils.UTCNow())
		if err := s.campaignRepo.Update(txCtx, *campaign); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, NewBusinessError("CANCEL_CAMPAIGN_FAILED", "Failed to cancel campaign", err)
	}

	return &dto.CancelCampaignResponse{
		Message: "Campaign cancelled successfully",
	}, nil
}

// CalculateCampaignCapacity handles the campaign capacity calculation process
func (s *CampaignFlowImpl) CalculateCampaignCapacity(ctx context.Context, req *dto.CalculateCampaignCapacityRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCapacityResponse, error) {
	if err := s.validateCalculateCampaignCapacityRequest(req); err != nil {
		return nil, NewBusinessError("CALCULATE_CAMPAIGN_CAPACITY_VALIDATION_FAILED", "Campaign capacity calculation validation failed", err)
	}

	// Fetch audience spec (from cache or file)
	specResp, err := s.ListAudienceSpec(ctx)
	if err != nil {
		return nil, NewBusinessError("LIST_AUDIENCE_SPEC_FAILED", "Failed to load audience spec", err)
	}

	var capacity uint64

	// Build a set of requested tags for quick lookup
	tagSet := make(map[string]struct{}, len(req.Tags))
	for _, t := range req.Tags {
		if t != "" {
			tagSet[t] = struct{}{}
		}
	}

	// Sum available audience only for requested Level1/Level2/Level3 keys
	// Respect provided tags: if tags set is empty, count all items; otherwise only items with matching tags.
	if req.Level1 != nil {
		l1k := *req.Level1
		l1map, ok := specResp.Spec[l1k]
		if ok {
			// prepare lookups for requested level2s and level3s
			level2Set := make(map[string]struct{}, len(req.Level2s))
			for _, l2 := range req.Level2s {
				if l2 != "" {
					level2Set[l2] = struct{}{}
				}
			}
			level3Set := make(map[string]struct{}, len(req.Level3s))
			for _, l3 := range req.Level3s {
				if l3 != "" {
					level3Set[l3] = struct{}{}
				}
			}

			for l2k, node := range l1map {
				// skip level2s not requested
				if len(level2Set) > 0 {
					if _, ok := level2Set[l2k]; !ok {
						continue
					}
				}
				if len(node.Items) == 0 && len(node.Metadata) == 0 {
					continue
				}
				for l3k, item := range node.Items {
					// skip level3s not requested
					if len(level3Set) > 0 {
						if _, ok := level3Set[l3k]; !ok {
							continue
						}
					}
					if len(tagSet) == 0 {
						capacity += uint64(item.AvailableAudience)
						continue
					}
					matched := false
					for _, it := range item.Tags {
						if _, ok := tagSet[it]; ok {
							matched = true
							break
						}
					}
					if matched {
						capacity += uint64(item.AvailableAudience)
					}
				}
			}
		}
	}

	return &dto.CalculateCampaignCapacityResponse{
		Message:  "Campaign capacity calculated successfully",
		Capacity: capacity,
	}, nil
}

// CalculateCampaignCost handles the campaign cost calculation process
func (s *CampaignFlowImpl) CalculateCampaignCost(ctx context.Context, req *dto.CalculateCampaignCostRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCostResponse, error) {
	// Calculate the number of parts based on content length
	numPages := s.calculateSMSParts(req.Content)

	if req.LineNumber == nil {
		return nil, NewBusinessError("LINE_NUMBER_REQUIRED", "Line number is required", ErrCampaignLineNumberRequired)
	}

	// Pricing constants
	basePrice := uint64(200)
	lineFactor, err := s.fetchLineNumberPriceFactor(ctx, *req.LineNumber)
	if err != nil {
		return nil, NewBusinessError("LINE_NUMBER_PRICE_FACTOR_FETCH_FAILED", "Failed to fetch line number price factor", err)
	}
	segmentPriceFactor := float64(1)
	if len(req.Level3s) > 0 {
		factors, err := s.segmentPriceRepo.LatestByLevel3s(ctx, req.Level3s)
		if err != nil {
			return nil, NewBusinessError("SEGMENT_PRICE_FACTOR_FETCH_FAILED", "Failed to fetch segment price factors", err)
		}
		maxFactor := float64(0)
		for _, l3 := range req.Level3s {
			if f, ok := factors[l3]; ok && f > maxFactor {
				maxFactor = f
			}
		}
		if maxFactor == 0 {
			s.notifyMissingSegmentPriceFactor(req.Level3s)
			return nil, NewBusinessError("SEGMENT_PRICE_FACTOR_NOT_FOUND", "Segment price factor not found for provided level3 options", ErrSegmentPriceFactorNotFound)
		}
		segmentPriceFactor = maxFactor
	}

	// Calculate price per message
	pricePerMsg := uint64(200*numPages) + basePrice*uint64(float64(lineFactor)*segmentPriceFactor)

	// Calculate campaign capacity (target audience size)
	capacityResp, err := s.CalculateCampaignCapacity(ctx, &dto.CalculateCampaignCapacityRequest{
		Title:      req.Title,
		Level1:     req.Level1,
		Level2s:    req.Level2s,
		Level3s:    req.Level3s,
		Tags:       req.Tags,
		Sex:        req.Sex,
		City:       req.City,
		AdLink:     req.AdLink,
		Content:    req.Content,
		ScheduleAt: req.ScheduleAt,
		LineNumber: req.LineNumber,
		Budget:     req.Budget,
	}, metadata)
	if err != nil {
		return nil, NewBusinessError("CAPACITY_CALCULATION_FAILED", "Failed to calculate campaign capacity", err)
	}

	availableCapacity := capacityResp.Capacity
	numTargetAudience := availableCapacity
	if req.Budget != nil {
		numTargetAudience = uint64(math.Min(float64(availableCapacity), float64(*req.Budget)/float64(pricePerMsg)))
	}

	total := pricePerMsg * numTargetAudience
	response := &dto.CalculateCampaignCostResponse{
		Message:           "Campaign cost calculated successfully",
		TotalCost:         total,
		NumTargetAudience: numTargetAudience,
		MaxTargetAudience: availableCapacity,
	}

	return response, nil
}

func (s *CampaignFlowImpl) fetchLineNumberPriceFactor(ctx context.Context, lineNumber string) (float64, error) {
	ln, err := s.lineNumberRepo.ByValue(ctx, lineNumber)
	if err != nil {
		return 0, err
	}
	if ln == nil {
		return 0, ErrLineNumberNotFound
	}
	if !*ln.IsActive {
		return 0, ErrLineNumberNotActive
	}

	return ln.PriceFactor, nil
}

func (s *CampaignFlowImpl) notifyMissingSegmentPriceFactor(level3s []string) {
	mobile := strings.TrimSpace(s.adminConfig.Mobile)
	if mobile == "" || s.notifier == nil {
		return
	}
	msg := fmt.Sprintf("Segment price factor missing for level3: %s", strings.Join(level3s, ","))
	go func() {
		_ = s.notifier.SendSMS(context.Background(), mobile, msg, nil)
	}()
}

// ListCampaigns retrieves user's campaigns with pagination, ordering and filters
func (s *CampaignFlowImpl) ListCampaigns(ctx context.Context, req *dto.ListCampaignsRequest, metadata *ClientMetadata) (*dto.ListCampaignsResponse, error) {
	var err error
	defer func() {
		if err != nil {
			err = NewBusinessError("LIST_CAMPAIGNS_FAILED", "Failed to list campaigns", err)
		}
	}()

	// Validate customer
	_, err = getCustomer(ctx, s.customerRepo, req.CustomerID)
	if err != nil {
		return nil, err
	}

	// Normalize pagination
	page := max(1, req.Page)
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	// Build filter
	filter := models.CampaignFilter{CustomerID: &req.CustomerID}
	if req.Filter != nil {
		if req.Filter.Title != nil && *req.Filter.Title != "" {
			filter.Title = req.Filter.Title
		}
		if req.Filter.Status != nil && *req.Filter.Status != "" {
			status := models.CampaignStatus(*req.Filter.Status)
			if status.Valid() {
				filter.Status = &status
			}
		}
	}

	// Order by
	orderBy := "created_at DESC"
	switch req.OrderBy {
	case "oldest":
		orderBy = "created_at ASC"
	case "newest":
		orderBy = "created_at DESC"
	}

	// Count total
	total64, err := s.campaignRepo.Count(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Fetch rows
	rows, err := s.campaignRepo.ByFilter(ctx, filter, orderBy, limit, offset)
	if err != nil {
		return nil, err
	}

	// Precompute click counts per campaign
	campaignIDs := make([]uint, 0, len(rows))
	for _, c := range rows {
		campaignIDs = append(campaignIDs, c.ID)
	}
	clickCounts, err := s.campaignRepo.ClickCounts(ctx, campaignIDs)
	if err != nil {
		return nil, err
	}

	// Map to response items
	items := make([]dto.GetCampaignResponse, 0, len(rows))
	for _, c := range rows {
		var statsMap map[string]any
		if len(c.Statistics) > 0 {
			_ = json.Unmarshal(c.Statistics, &statsMap)
		}
		totalSent := float64(0)
		if v, ok := statsMap["totalSent"]; ok {
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
		clicks := clickCounts[c.ID]
		var clickRate *float64
		if totalSent > 0 {
			val := float64(clicks) / totalSent
			clickRate = &val
		}
		totalClicks := clicks

		var linePriceFactor *float64
		if c.Spec.LineNumber != nil {
			lineNumber, err := s.lineNumberRepo.ByValue(ctx, *c.Spec.LineNumber)
			if err != nil {
				return nil, err
			}
			if lineNumber != nil {
				linePriceFactor = &lineNumber.PriceFactor
			}
		}

		items = append(items, dto.GetCampaignResponse{
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
			ScheduleAt:      c.Spec.ScheduleAt,
			LineNumber:      c.Spec.LineNumber,
			LinePriceFactor: linePriceFactor,
			Budget:          c.Spec.Budget,
			NumAudience:     c.NumAudience,
			Comment:         c.Comment,
			Statistics:      statsMap,
			ClickRate:       clickRate,
			TotalClicks:     &totalClicks,
		})
	}

	// Build pagination
	totalPages := int((total64 + int64(limit) - 1) / int64(limit))

	return &dto.ListCampaignsResponse{
		Message: "Campaigns retrieved successfully",
		Items:   items,
		Pagination: dto.PaginationInfo{
			Total:      total64,
			Page:       page,
			Limit:      limit,
			TotalPages: totalPages,
		},
	}, nil
}

// validateCreateCampaignRequest validates the campaign creation request
func (s *CampaignFlowImpl) validateCreateCampaignRequest(req *dto.CreateCampaignRequest) error {
	if req.CustomerID == 0 {
		return ErrCustomerNotFound
	}
	if req.Title == nil || (req.Title != nil && *req.Title == "") {
		return ErrCampaignTitleRequired
	}
	if req.Content != nil && *req.Content == "" {
		return ErrCampaignContentRequired
	}
	if req.Level1 == nil || (req.Level1 != nil && *req.Level1 == "") {
		return ErrCampaignLevel1Required
	}
	if req.Level2s == nil || (req.Level2s != nil && len(req.Level2s) == 0) {
		return ErrCampaignLevel2sRequired
	}
	if req.Level3s == nil || (req.Level3s != nil && len(req.Level3s) == 0) {
		return ErrCampaignLevel3sRequired
	}
	if req.Tags == nil || (req.Tags != nil && len(req.Tags) == 0) {
		return ErrCampaignTagsRequired
	}
	if req.LineNumber != nil && *req.LineNumber == "" {
		return ErrCampaignLineNumberRequired
	}
	if req.Budget != nil && *req.Budget <= 0 {
		return ErrCampaignBudgetRequired
	}
	if req.Sex != nil && *req.Sex == "" {
		return ErrCampaignSexRequired
	}
	if req.City != nil && len(req.City) == 0 {
		return ErrCampaignCityRequired
	}
	if req.AdLink != nil && *req.AdLink == "" {
		return ErrCampaignAdLinkRequired
	}

	// Validate schedule time must be at least 10 minutes in the future
	scheduleTime := req.ScheduleAt
	if scheduleTime != nil && !scheduleTime.IsZero() {
		if scheduleTime.Before(utils.UTCNow().Add(10 * time.Minute)) {
			return ErrScheduleTimeTooSoon
		}
	}

	if len(req.City) > 0 {
		if slices.Contains(req.City, "") {
			return ErrCampaignCityRequired
		}
	}

	if len(req.Level2s) > 0 {
		if slices.Contains(req.Level2s, "") {
			return ErrCampaignLevel2sRequired
		}
	}

	if len(req.Level3s) > 0 {
		if slices.Contains(req.Level3s, "") {
			return ErrCampaignLevel3sRequired
		}
	}

	if len(req.Tags) > 0 {
		if slices.Contains(req.Tags, "") {
			return ErrCampaignTagsRequired
		}
	}

	return nil
}

// createCampaign creates the campaign in the database
func (s *CampaignFlowImpl) createCampaign(ctx context.Context, req *dto.CreateCampaignRequest, customer *models.Customer) (*models.Campaign, error) {
	// Build campaign spec
	spec := models.CampaignSpec{}

	if req.Title != nil && *req.Title != "" {
		spec.Title = req.Title
	}
	if req.Level1 != nil && *req.Level1 != "" {
		spec.Level1 = req.Level1
	}
	if len(req.Level2s) > 0 {
		spec.Level2s = req.Level2s
	}
	if len(req.Level3s) > 0 {
		spec.Level3s = req.Level3s
	}
	if len(req.Tags) > 0 {
		spec.Tags = req.Tags
	}
	if req.Sex != nil && *req.Sex != "" {
		spec.Sex = req.Sex
	}
	if len(req.City) > 0 {
		spec.City = req.City
	}
	if req.AdLink != nil && *req.AdLink != "" {
		spec.AdLink = req.AdLink
	}
	if req.Content != nil && *req.Content != "" {
		spec.Content = req.Content
	}
	if req.ScheduleAt != nil {
		spec.ScheduleAt = req.ScheduleAt
	}
	if req.LineNumber != nil && *req.LineNumber != "" {
		spec.LineNumber = req.LineNumber
	}
	if req.Budget != nil && *req.Budget != 0 {
		spec.Budget = req.Budget
	}

	// Create campaign model
	campaign := models.Campaign{
		UUID:       uuid.New(),
		CustomerID: customer.ID,
		Status:     models.CampaignStatusInitiated,
		Spec:       spec,
	}

	// Save to database
	err := s.campaignRepo.Save(ctx, &campaign)
	if err != nil {
		return nil, err
	}

	// Get the created campaign with ID
	createdCampaign, err := s.campaignRepo.ByUUID(ctx, campaign.UUID.String())
	if err != nil {
		return nil, err
	}

	return createdCampaign, nil
}

// validateUpdateCampaignRequest validates the campaign update request
func (s *CampaignFlowImpl) validateUpdateCampaignRequest(req *dto.UpdateCampaignRequest) error {
	if req.UUID == "" {
		return ErrCampaignUUIDRequired
	}

	if req.CustomerID == 0 {
		return ErrCustomerNotFound
	}

	// At least one field should be provided for update
	hasUpdateFields := req.Title != nil || req.Level1 != nil || len(req.Level2s) > 0 || len(req.Level3s) > 0 ||
		len(req.Tags) > 0 || req.Sex != nil || len(req.City) > 0 || req.AdLink != nil || req.Content != nil ||
		req.ScheduleAt != nil || req.LineNumber != nil || req.Budget != nil

	if !hasUpdateFields {
		return ErrCampaignUpdateRequired
	}

	return nil
}

// validateCalculateCampaignCapacityRequest validates the request
func (s *CampaignFlowImpl) validateCalculateCampaignCapacityRequest(req *dto.CalculateCampaignCapacityRequest) error {
	if req.Level1 == nil {
		return ErrCampaignLevel1Required
	}
	if req.Level2s == nil {
		return ErrCampaignLevel2sRequired
	}
	if len(req.Level2s) == 0 {
		return ErrCampaignLevel2sRequired
	}
	if req.Level3s == nil {
		return ErrCampaignLevel3sRequired
	}
	if len(req.Level3s) == 0 {
		return ErrCampaignLevel3sRequired
	}
	if req.Tags == nil {
		return ErrCampaignTagsRequired
	}
	if len(req.Tags) == 0 {
		return ErrCampaignTagsRequired
	}

	return nil
}

func (s *CampaignFlowImpl) canFinalizeCampaign(campaign models.Campaign) error {
	if campaign.Spec.Title == nil || *campaign.Spec.Title == "" {
		return ErrCampaignTitleRequired
	}
	if campaign.Spec.Level1 == nil || *campaign.Spec.Level1 == "" {
		return ErrCampaignLevel1Required
	}
	if campaign.Spec.Level2s == nil {
		return ErrCampaignLevel2sRequired
	}
	if len(campaign.Spec.Level2s) == 0 {
		return ErrCampaignLevel2sRequired
	}
	if campaign.Spec.Level3s == nil {
		return ErrCampaignLevel3sRequired
	}
	if len(campaign.Spec.Level3s) == 0 {
		return ErrCampaignLevel3sRequired
	}
	if campaign.Spec.Tags == nil {
		return ErrCampaignTagsRequired
	}
	if len(campaign.Spec.Tags) == 0 {
		return ErrCampaignTagsRequired
	}
	if campaign.Spec.Content == nil || *campaign.Spec.Content == "" {
		return ErrCampaignContentRequired
	}
	if campaign.Spec.ScheduleAt == nil || campaign.Spec.ScheduleAt.IsZero() {
		return ErrScheduleTimeNotPresent
	}
	if campaign.Spec.ScheduleAt.Before(utils.UTCNow().Add(10 * time.Minute)) {
		return ErrScheduleTimeTooSoon
	}
	if campaign.Spec.LineNumber == nil || *campaign.Spec.LineNumber == "" {
		return ErrCampaignLineNumberRequired
	}
	if campaign.Spec.Budget == nil || *campaign.Spec.Budget <= 0 {
		return ErrCampaignBudgetRequired
	}

	return nil
}

// updateCampaign updates the campaign in the database
func (s *CampaignFlowImpl) updateCampaign(ctx context.Context, req *dto.UpdateCampaignRequest, existingCampaign *models.Campaign) error {
	// Update campaign spec with new values
	spec := existingCampaign.Spec

	if req.Title != nil && *req.Title != "" {
		spec.Title = req.Title
	}
	if req.Level1 != nil && *req.Level1 != "" {
		spec.Level1 = req.Level1
	}
	if len(req.Level2s) > 0 {
		spec.Level2s = req.Level2s
	}
	if len(req.Level3s) > 0 {
		spec.Level3s = req.Level3s
	}
	if len(req.Tags) > 0 {
		spec.Tags = req.Tags
	}
	if req.Sex != nil && *req.Sex != "" {
		spec.Sex = req.Sex
	}
	if len(req.City) > 0 {
		spec.City = req.City
	}
	if req.AdLink != nil && *req.AdLink != "" {
		spec.AdLink = req.AdLink
	}
	if req.Content != nil && *req.Content != "" {
		spec.Content = req.Content
	}
	if req.ScheduleAt != nil {
		spec.ScheduleAt = req.ScheduleAt
	}
	if req.LineNumber != nil && *req.LineNumber != "" {
		spec.LineNumber = req.LineNumber
	}
	if req.Budget != nil && *req.Budget != 0 {
		spec.Budget = req.Budget
	}

	// Update the campaign spec
	existingCampaign.Spec = spec
	existingCampaign.Status = models.CampaignStatusInProgress
	existingCampaign.UpdatedAt = utils.ToPtr(utils.UTCNow())

	// Save to database
	err := s.campaignRepo.Update(ctx, *existingCampaign)
	if err != nil {
		return err
	}

	return nil
}

// ListAudienceSpec returns the current audience spec from cache or file
func (s *CampaignFlowImpl) ListAudienceSpec(ctx context.Context) (*dto.ListAudienceSpecResponse, error) {
	// derive redis key and file path consistent with bot audience spec flow

	cacheKey := redisKey(*s.cacheConfig, utils.AudienceSpecCacheKey)
	filePath := audienceSpecFilePath()

	// try redis first
	if bs, err := s.rc.Get(ctx, cacheKey).Bytes(); err == nil && len(bs) > 0 {
		var out dto.AudienceSpec
		if err := json.Unmarshal(bs, &out); err == nil {
			return &dto.ListAudienceSpecResponse{
				Message: "Audience spec retrieved from cache",
				Spec:    out,
			}, nil
		}
	}

	// Read existing JSON file (if any) using v2 reader
	current, err := readAudienceSpecFileV2(filePath)
	if err != nil {
		return nil, NewBusinessError("LIST_AUDIENCE_SPEC_READ_FAILED", "Failed to read audience spec file", err)
	}

	// Build DTO shape including level-2 metadata and only positive-availability items
	out := make(dto.AudienceSpec)
	for l1, l2map := range current {
		for l2k, node := range l2map {
			if node == nil {
				continue
			}
			// Collect items with AvailableAudience > 0
			items := make(map[string]dto.AudienceSpecItem)
			for l3k, leaf := range node.Items {
				if leaf.AvailableAudience > 0 {
					items[l3k] = dto.AudienceSpecItem{
						Tags:              leaf.Tags,
						AvailableAudience: leaf.AvailableAudience,
					}
				}
			}
			if len(items) == 0 && len(node.Metadata) == 0 {
				// Skip empty level2 without items and metadata
				continue
			}
			if _, ok := out[l1]; !ok {
				out[l1] = make(map[string]dto.AudienceSpecLevel2)
			}
			out[l1][l2k] = dto.AudienceSpecLevel2{
				Metadata: node.Metadata,
				Items:    items,
			}
		}
	}

	// Cache DTO JSON
	if bs, err := json.MarshalIndent(out, "", "  "); err == nil {
		_ = s.rc.Set(ctx, cacheKey, bs, 0).Err()
	}

	return &dto.ListAudienceSpecResponse{
		Message: "Audience spec retrieved",
		Spec:    out,
	}, nil
}

func (s *CampaignFlowImpl) GetApprovedRunningSummary(ctx context.Context, customerID uint) (*dto.CampaignsSummaryResponse, error) {
	if customerID == 0 {
		return nil, NewBusinessError("CUSTOMER_ID_REQUIRED", "customer_id must be greater than 0", ErrCustomerNotFound)
	}

	// Build counts using repository Count with combined filters
	custID := customerID
	statusApproved := models.CampaignStatusApproved
	statusRunning := models.CampaignStatusRunning

	approvedCount64, err := s.campaignRepo.Count(ctx, models.CampaignFilter{CustomerID: &custID, Status: &statusApproved})
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_COUNT_FAILED", "Failed to count approved campaigns", err)
	}
	runningCount64, err := s.campaignRepo.Count(ctx, models.CampaignFilter{CustomerID: &custID, Status: &statusRunning})
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_COUNT_FAILED", "Failed to count running campaigns", err)
	}

	approved := int(approvedCount64)
	running := int(runningCount64)
	resp := &dto.CampaignsSummaryResponse{
		Message:       "Campaigns summary retrieved",
		ApprovedCount: approved,
		RunningCount:  running,
		Total:         approved + running,
	}
	return resp, nil
}

// calculateSMSParts calculates the number of SMS parts based on character count
// This implements the same logic as the frontend calculateSMSParts function
func (s *CampaignFlowImpl) calculateSMSParts(content *string) uint64 {
	if content == nil || *content == "" {
		return 1
	}

	// Count characters with proper weighting (English=1, others=2)
	charCount := s.countCharacters(*content)

	// Calculate SMS parts based on character count
	if charCount <= 70 {
		return 1
	} else if charCount <= 132 {
		return 2
	} else if charCount <= 198 {
		return 3
	} else if charCount <= 264 {
		return 4
	} else if charCount <= 330 {
		return 5
	}
	return 6 // More than 330 characters
}

// countCharacters counts characters in text with proper weighting for different character types
// English characters and numbers = 1, others (Farsi, Arabic, etc.) = 2
// This implements the same logic as the frontend countCharacters function
func (s *CampaignFlowImpl) countCharacters(text string) uint64 {
	if text == "" {
		return 0
	}

	// Remove the link character (🔗) before counting
	textWithoutLinkChar := strings.ReplaceAll(text, "🔗", "")

	var count uint64
	for _, char := range textWithoutLinkChar {
		// Check if character is English (ASCII range 32-126)
		if char >= 32 && char <= 126 {
			count += 1 // English character
		} else {
			count += 2 // Non-English character (Farsi, Arabic, etc.)
		}
	}
	return count
}

// createAuditLog creates an audit log entry for the campaign operation
func (s *CampaignFlowImpl) createAuditLog(ctx context.Context, customer *models.Customer, action, description string, success bool, errorMsg *string, metadata *ClientMetadata) error {
	var customerID *uint
	if customer != nil {
		customerID = &customer.ID
	}

	ipAddress := ""
	userAgent := ""
	if metadata != nil {
		ipAddress = metadata.IPAddress
		userAgent = metadata.UserAgent
	}

	audit := &models.AuditLog{
		CustomerID:   customerID,
		Action:       action,
		Description:  &description,
		Success:      utils.ToPtr(success),
		IPAddress:    &ipAddress,
		UserAgent:    &userAgent,
		ErrorMessage: errorMsg,
	}

	// Extract request ID from context if available
	requestID := ctx.Value(utils.RequestIDKey)
	if requestID != nil {
		requestIDStr, ok := requestID.(string)
		if ok {
			audit.RequestID = &requestIDStr
		}
	}

	if err := s.auditRepo.Save(ctx, audit); err != nil {
		return err
	}

	return nil
}
