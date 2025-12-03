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
		segment    = req.Segment
		subsegment = req.Subsegment
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
	if req.Segment == nil {
		segment = campaign.Spec.Segment
	}
	if req.Subsegment == nil {
		subsegment = campaign.Spec.Subsegment
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
		Segment:    segment,
		Subsegment: subsegment,
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
			// TODO: -------------------
			// TODO: validate all required fields are present like line number, budget
			// schedule time must be present and be in future
			// title (len), sex (choice), segment (db, len), subsegment (db, len), tags (db, len), city (db, len), adlink (len), content (len), line number (db, len), budget (min, max)
			// TODO: ------------------- Line number must be valid (db)

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
				Segment:    segment,
				Subsegment: subsegment,
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

			newCreditBalance := uint64(0)
			newFreeBalance := uint64(0)
			if latestBalance.CreditBalance <= cost.TotalCost {
				newCreditBalance = 0
				newFreeBalance = latestBalance.FreeBalance - (cost.TotalCost - latestBalance.CreditBalance)
			} else {
				newCreditBalance = latestBalance.CreditBalance - cost.TotalCost
				newFreeBalance = latestBalance.FreeBalance
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
				UUID:          uuid.New(),
				CorrelationID: corrID,
				WalletID:      wallet.ID,
				CustomerID:    customer.ID,
				FreeBalance:   newFreeBalance,
				FrozenBalance: newFrozenBalance,
				CreditBalance: newCreditBalance,
				LockedBalance: latestBalance.LockedBalance,
				TotalBalance:  newFreeBalance + newFrozenBalance + latestBalance.LockedBalance + newCreditBalance,
				Reason:        "campaign_budget_reserved_waiting_for_approval",
				Description:   fmt.Sprintf("Budget reserved for campaign %d", campaign.ID),
				Metadata:      metaBytes,
				CreatedAt:     utils.UTCNow(),
				UpdatedAt:     utils.UTCNow(),
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

			transaction := &models.Transaction{
				UUID:          uuid.New(),
				CorrelationID: corrID,
				Type:          models.TransactionTypeLaunchCampaign,
				Status:        models.TransactionStatusPending,
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
			if err := s.transactionRepo.Save(txCtx, transaction); err != nil {
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

// CalculateCampaignCapacity handles the campaign capacity calculation process
func (s *CampaignFlowImpl) CalculateCampaignCapacity(ctx context.Context, req *dto.CalculateCampaignCapacityRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCapacityResponse, error) {
	if req.Segment == nil {
		return nil, NewBusinessError("SEGMENT_REQUIRED", "Segment is required", ErrCampaignSegmentRequired)
	}
	if req.Subsegment == nil {
		return nil, NewBusinessError("SUBSEGMENT_REQUIRED", "Subsegment is required", ErrCampaignSubsegmentRequired)
	}
	if req.Tags == nil {
		return nil, NewBusinessError("TAGS_REQUIRED", "Tags is required", ErrCampaignTagsRequired)
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

	// Sum available audience where at least one tag matches across all levels
	for _, lvl2 := range specResp.Spec {
		for _, lvl3 := range lvl2 {
			for _, item := range lvl3 {
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
	basePrice := uint64(150) // 600
	lineFactor, err := s.fetchLineNumberPriceFactor(ctx, *req.LineNumber)
	if err != nil {
		return nil, NewBusinessError("LINE_NUMBER_PRICE_FACTOR_FETCH_FAILED", "Failed to fetch line number price factor", err)
	}
	segmentFactor := float64(1)

	// Calculate price per message
	pricePerMsg := uint64(200*numPages) + basePrice*uint64(float64(lineFactor)*segmentFactor)

	// TODO: Fix it
	pricePerMsg = 2

	// Calculate campaign capacity (target audience size)
	capacityResp, err := s.CalculateCampaignCapacity(ctx, &dto.CalculateCampaignCapacityRequest{
		Title:      req.Title,
		Segment:    req.Segment,
		Subsegment: req.Subsegment,
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
	page := req.Page
	if page < 1 {
		page = 1
	}
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

	// Map to response items
	items := make([]dto.GetCampaignResponse, 0, len(rows))
	for _, c := range rows {
		items = append(items, dto.GetCampaignResponse{
			UUID:       c.UUID.String(),
			Status:     c.Status.String(),
			CreatedAt:  c.CreatedAt,
			UpdatedAt:  c.UpdatedAt,
			Title:      c.Spec.Title,
			Segment:    c.Spec.Segment,
			Subsegment: c.Spec.Subsegment,
			Tags:       c.Spec.Tags,
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
	if req.Title != nil && *req.Title == "" {
		return ErrCampaignTitleRequired
	}
	if req.Content != nil && *req.Content == "" {
		return ErrCampaignContentRequired
	}
	if req.Segment != nil && *req.Segment == "" {
		return ErrCampaignSegmentRequired
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

	if len(req.Subsegment) > 0 {
		if slices.Contains(req.Subsegment, "") {
			return ErrCampaignSubsegmentRequired
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
	if req.Segment != nil && *req.Segment != "" {
		spec.Segment = req.Segment
	}
	if len(req.Subsegment) > 0 {
		spec.Subsegment = req.Subsegment
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
	hasUpdateFields := req.Title != nil || req.Segment != nil || len(req.Subsegment) > 0 || len(req.Tags) > 0 ||
		req.Sex != nil || len(req.City) > 0 || req.AdLink != nil || req.Content != nil ||
		req.ScheduleAt != nil || req.LineNumber != nil || req.Budget != nil

	if !hasUpdateFields {
		return ErrCampaignUpdateRequired
	}

	return nil
}

func (s *CampaignFlowImpl) canFinalizeCampaign(campaign models.Campaign) error {
	if campaign.Spec.Title == nil || *campaign.Spec.Title == "" {
		return ErrCampaignTitleRequired
	}
	if campaign.Spec.Segment == nil || *campaign.Spec.Segment == "" {
		return ErrCampaignSegmentRequired
	}
	if campaign.Spec.Subsegment == nil {
		return ErrCampaignSubsegmentRequired
	}
	if campaign.Spec.Tags == nil {
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
	if req.Segment != nil && *req.Segment != "" {
		spec.Segment = req.Segment
	}
	if len(req.Subsegment) > 0 {
		spec.Subsegment = req.Subsegment
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

	// Read existing JSON file (if any)
	current, err := readAudienceSpecFile(filePath)
	if err != nil {
		return nil, NewBusinessError("LIST_AUDIENCE_SPEC_READ_FAILED", "Failed to read audience spec file", err)
	}

	// Marshal and write atomically (tmp + rename)
	bytes, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return nil, NewBusinessError("LIST_AUDIENCE_SPEC_MARSHAL_FAILED", "Failed to marshal merged spec", err)
	}

	// Update Redis cache
	_ = s.rc.Set(ctx, cacheKey, bytes, 0).Err()

	out := make(dto.AudienceSpec)
	for l1, l2 := range current {
		for l2k, l3 := range l2 {
			for l3k, item := range l3 {
				if item.AvailableAudience > 0 {
					if _, ok := out[l1]; !ok {
						out[l1] = make(map[string]map[string]dto.AudienceSpecItem)
					}
					if _, ok := out[l1][l2k]; !ok {
						out[l1][l2k] = make(map[string]dto.AudienceSpecItem)
					}
					out[l1][l2k][l3k] = dto.AudienceSpecItem{
						Tags:              item.Tags,
						AvailableAudience: item.AvailableAudience,
					}
				}
			}
		}
	}

	return &dto.ListAudienceSpecResponse{
		Message: "Audience spec retrieved",
		Spec:    out,
	}, nil
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
