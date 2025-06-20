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
	"gorm.io/gorm"
)

// CampaignFlow handles the campaign business logic
type CampaignFlow interface {
	CreateCampaign(ctx context.Context, req *dto.CreateCampaignRequest, metadata *ClientMetadata) (*dto.CreateCampaignResponse, error)
	UpdateCampaign(ctx context.Context, req *dto.UpdateCampaignRequest, metadata *ClientMetadata) (*dto.UpdateCampaignResponse, error)
	CalculateCampaignCapacity(ctx context.Context, req *dto.CalculateCampaignCapacityRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCapacityResponse, error)
	CalculateCampaignCost(ctx context.Context, req *dto.CalculateCampaignCostRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCostResponse, error)
	ListCampaigns(ctx context.Context, req *dto.ListCampaignsRequest, metadata *ClientMetadata) (*dto.ListCampaignsResponse, error)
}

// CampaignFlowImpl implements the campaign business flow
type CampaignFlowImpl struct {
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

// NewCampaignFlow creates a new campaign flow instance
func NewCampaignFlow(
	campaignRepo repository.CampaignRepository,
	customerRepo repository.CustomerRepository,
	walletRepo repository.WalletRepository,
	balanceSnapshotRepo repository.BalanceSnapshotRepository,
	transactionRepo repository.TransactionRepository,
	auditRepo repository.AuditLogRepository,
	db *gorm.DB,
	notifier services.NotificationService,
	adminConfig config.AdminConfig,
) CampaignFlow {
	return &CampaignFlowImpl{
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
			// title (len), sex (choice), segment (db, len), subsegment (db, len), city (db, len), adlink (len), content (len), line number (db, len), budget (min, max)
			// TODO: ------------------- Line number must be valid (db)

			// retrieve campaign again (last state)
			campaign, err := getCampaign(txCtx, s.campaignRepo, req.UUID, req.CustomerID)
			if err != nil {
				return err
			}

			if err := s.canFinalizeCampaign(campaign); err != nil {
				return err
			}

			campaign.Status = models.CampaignStatusWaitingForApproval
			campaign.UpdatedAt = utils.ToPtr(utils.UTCNow())
			if err := s.campaignRepo.Update(txCtx, campaign); err != nil {
				return err
			}

			cost, err := s.CalculateCampaignCost(txCtx, &dto.CalculateCampaignCostRequest{
				Title:      title,
				Segment:    segment,
				Subsegment: subsegment,
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

			// Fetch wallet free balance
			wallet, err := s.walletRepo.ByCustomerID(txCtx, req.CustomerID)
			if err != nil {
				return err
			}
			if wallet == nil {
				wallet, err := createWalletWithInitialSnapshot(txCtx, s.walletRepo, req.CustomerID, "campaign_update")
				if err != nil {
					return err
				}

				// Update customer with wallet reference
				customer.Wallet = &wallet
			}

			latestBalance, err := getLatestBalanceSnapshot(txCtx, s.walletRepo, wallet.ID)
			if err != nil {
				return err
			}

			availableBalance := latestBalance.FreeBalance + latestBalance.CreditBalance
			if availableBalance < cost.Total {
				return ErrInsufficientFunds
			}

			newCreditBalance := uint64(0)
			newFreeBalance := uint64(0)
			if latestBalance.CreditBalance <= cost.Total {
				newCreditBalance = 0
				newFreeBalance = latestBalance.FreeBalance - (cost.Total - latestBalance.CreditBalance)
			} else {
				newCreditBalance = latestBalance.CreditBalance - cost.Total
				newFreeBalance = latestBalance.FreeBalance
			}
			newFrozenBalance := latestBalance.FrozenBalance + cost.Total

			// -------------------
			// -------------------
			// TODO: ALSO add formula for calculating capacity and cost (I mean, parameters)
			// -------------------
			// -------------------

			// Build metadata with full campaign spec
			meta := map[string]any{
				"source":        "campaign_update",
				"operation":     "reserve_budget",
				"campaign_id":   campaign.ID,
				"amount":        cost.Total,
				"currency":      utils.TomanCurrency,
				"campaign_spec": campaign.Spec,
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
				Amount:        cost.Total,
				Currency:      utils.TomanCurrency,
				WalletID:      wallet.ID,
				CustomerID:    customer.ID,
				BalanceBefore: beforeMap,
				BalanceAfter:  afterMap,
				Description:   fmt.Sprintf("Campaign budget reserved: %d Tomans for campaign %d", cost.Total, campaign.ID),
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
	// For now, just return a fixed capacity of 11000
	// In the future, this could be enhanced with:
	// - Target audience analysis based on segment/subsegment
	// - Geographic reach calculation based on cities
	// - Budget-based capacity estimation
	// - Historical campaign performance data
	// - Seasonal factors and market conditions

	response := &dto.CalculateCampaignCapacityResponse{
		Message:  "Campaign capacity calculated successfully",
		Capacity: 501,
	}

	return response, nil
}

// CalculateCampaignCost handles the campaign cost calculation process
func (s *CampaignFlowImpl) CalculateCampaignCost(ctx context.Context, req *dto.CalculateCampaignCostRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCostResponse, error) {
	// Calculate the number of parts based on content length
	numPages := s.calculateSMSParts(req.Content)

	// Pricing constants
	basePrice := uint64(140)
	lineFactor := uint64(20)
	segmentFactor := float64(2.2)

	// Calculate price per message
	pricePerMsg := uint64(200*numPages) + basePrice*uint64(float64(lineFactor)*segmentFactor)

	// Calculate campaign capacity (target audience size)
	capacityResp, err := s.CalculateCampaignCapacity(ctx, &dto.CalculateCampaignCapacityRequest{
		Title:      req.Title,
		Segment:    req.Segment,
		Subsegment: req.Subsegment,
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
	msgTarget := availableCapacity
	if req.Budget != nil {
		msgTarget = uint64(math.Min(float64(availableCapacity), float64(*req.Budget)/float64(pricePerMsg)))
	}

	total := pricePerMsg * msgTarget
	response := &dto.CalculateCampaignCostResponse{
		Message:      "Campaign cost calculated successfully",
		Total:        total,
		MsgTarget:    msgTarget,
		MaxMsgTarget: availableCapacity,
	}

	return response, nil
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

	// Fetch items
	items, err := s.campaignRepo.ByFilter(ctx, filter, orderBy, limit, offset)
	if err != nil {
		return nil, err
	}

	// Map to response items
	respItems := make([]dto.GetCampaignResponse, 0, len(items))
	for _, c := range items {
		respItems = append(respItems, dto.GetCampaignResponse{
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

	// Build pagination
	totalPages := int((total64 + int64(limit) - 1) / int64(limit))

	return &dto.ListCampaignsResponse{
		Message: "Campaigns retrieved successfully",
		Items:   respItems,
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
	hasUpdateFields := req.Title != nil || req.Segment != nil || len(req.Subsegment) > 0 ||
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
