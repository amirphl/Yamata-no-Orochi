// Package businessflow contains the core business logic and use cases for SMS campaign workflows
package businessflow

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SMSCampaignFlow handles the SMS campaign business logic
type SMSCampaignFlow interface {
	CreateCampaign(ctx context.Context, req *dto.CreateSMSCampaignRequest, metadata *ClientMetadata) (*dto.CreateSMSCampaignResponse, error)
	UpdateCampaign(ctx context.Context, req *dto.UpdateSMSCampaignRequest, metadata *ClientMetadata) (*dto.UpdateSMSCampaignResponse, error)
	CalculateCampaignCapacity(ctx context.Context, req *dto.CalculateCampaignCapacityRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCapacityResponse, error)
	CalculateCampaignCost(ctx context.Context, req *dto.CalculateCampaignCostRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCostResponse, error)
	GetWalletBalance(ctx context.Context, req *dto.GetWalletBalanceRequest, metadata *ClientMetadata) (*dto.GetWalletBalanceResponse, error)
	ListCampaigns(ctx context.Context, req *dto.ListSMSCampaignsRequest, metadata *ClientMetadata) (*dto.ListSMSCampaignsResponse, error)
}

// SMSCampaignFlowImpl implements the SMS campaign business flow
type SMSCampaignFlowImpl struct {
	campaignRepo repository.SMSCampaignRepository
	customerRepo repository.CustomerRepository
	auditRepo    repository.AuditLogRepository
	db           *gorm.DB
}

// NewSMSCampaignFlow creates a new SMS campaign flow instance
func NewSMSCampaignFlow(
	campaignRepo repository.SMSCampaignRepository,
	customerRepo repository.CustomerRepository,
	auditRepo repository.AuditLogRepository,
	db *gorm.DB,
) SMSCampaignFlow {
	return &SMSCampaignFlowImpl{
		campaignRepo: campaignRepo,
		customerRepo: customerRepo,
		auditRepo:    auditRepo,
		db:           db,
	}
}

// CreateCampaign handles the complete SMS campaign creation process
func (s *SMSCampaignFlowImpl) CreateCampaign(ctx context.Context, req *dto.CreateSMSCampaignRequest, metadata *ClientMetadata) (*dto.CreateSMSCampaignResponse, error) {
	// Validate business rules
	if err := s.validateCreateCampaignRequest(ctx, req); err != nil {
		return nil, NewBusinessError("CAMPAIGN_VALIDATION_FAILED", "Campaign validation failed", err)
	}

	// Verify customer exists and is active
	customer, err := s.customerRepo.ByID(ctx, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CUSTOMER_LOOKUP_FAILED", "Failed to lookup customer", err)
	}
	if customer == nil {
		return nil, NewBusinessError("CUSTOMER_NOT_FOUND", "Customer not found", ErrCustomerNotFound)
	}
	if !utils.IsTrue(customer.IsActive) {
		return nil, NewBusinessError("CUSTOMER_INACTIVE", "Customer account is inactive", ErrAccountInactive)
	}

	// Use transaction for atomicity
	var campaign *models.SMSCampaign

	err = repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		// Create campaign
		var err error
		campaign, err = s.createCampaign(txCtx, req, customer)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Campaign creation failed: %s", err.Error())
		_ = s.createAuditLog(ctx, customer, models.AuditActionCampaignCreationFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("CAMPAIGN_CREATION_FAILED", "Campaign creation failed", err)
	}

	// Log successful creation
	msg := fmt.Sprintf("SMS campaign created successfully: %s", campaign.UUID.String())
	_ = s.createAuditLog(ctx, customer, models.AuditActionCampaignCreated, msg, true, nil, metadata)

	// Build response
	response := &dto.CreateSMSCampaignResponse{
		Message:   "SMS campaign created successfully",
		UUID:      campaign.UUID.String(),
		Status:    string(campaign.Status),
		CreatedAt: campaign.CreatedAt.Format(time.RFC3339),
	}

	return response, nil
}

// UpdateCampaign handles the SMS campaign update process
func (s *SMSCampaignFlowImpl) UpdateCampaign(ctx context.Context, req *dto.UpdateSMSCampaignRequest, metadata *ClientMetadata) (*dto.UpdateSMSCampaignResponse, error) {
	// Validate business rules
	if err := s.validateUpdateCampaignRequest(ctx, req); err != nil {
		return nil, NewBusinessError("CAMPAIGN_UPDATE_VALIDATION_FAILED", "Campaign update validation failed", err)
	}

	// Verify customer exists and is active
	customer, err := s.customerRepo.ByID(ctx, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CUSTOMER_LOOKUP_FAILED", "Failed to lookup customer", err)
	}
	if customer == nil {
		return nil, NewBusinessError("CUSTOMER_NOT_FOUND", "Customer not found", ErrCustomerNotFound)
	}
	if !utils.IsTrue(customer.IsActive) {
		return nil, NewBusinessError("CUSTOMER_INACTIVE", "Customer account is inactive", ErrAccountInactive)
	}

	// Get existing campaign and verify ownership
	existingCampaign, err := s.campaignRepo.ByUUID(ctx, req.UUID)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_LOOKUP_FAILED", "Failed to lookup campaign", err)
	}
	if existingCampaign == nil {
		return nil, NewBusinessError("CAMPAIGN_NOT_FOUND", "Campaign not found", ErrCampaignNotFound)
	}

	// Verify ownership
	if existingCampaign.CustomerID != req.CustomerID {
		return nil, NewBusinessError("CAMPAIGN_ACCESS_DENIED", "Access denied: campaign belongs to another customer", ErrCampaignAccessDenied)
	}

	// Check if campaign can be updated (only initiated campaigns can be updated)
	if !s.canUpdateCampaign(existingCampaign.Status) {
		return nil, NewBusinessError("CAMPAIGN_UPDATE_NOT_ALLOWED", "Campaign cannot be updated in current status", ErrCampaignUpdateNotAllowed)
	}

	capacity, err := s.CalculateCampaignCapacity(ctx, &dto.CalculateCampaignCapacityRequest{
		Title:      existingCampaign.Spec.Title,
		Segment:    existingCampaign.Spec.Segment,
		Subsegment: existingCampaign.Spec.Subsegment,
		Sex:        existingCampaign.Spec.Sex,
		City:       existingCampaign.Spec.City,
		AdLink:     existingCampaign.Spec.AdLink,
		Content:    existingCampaign.Spec.Content,
		ScheduleAt: existingCampaign.Spec.ScheduleAt,
		LineNumber: existingCampaign.Spec.LineNumber,
		Budget:     existingCampaign.Spec.Budget,
	}, metadata)

	if err != nil {
		return nil, NewBusinessError("CAPACITY_CALCULATION_FAILED", "Failed to calculate campaign capacity", err)
	}

	if capacity.Capacity < 500 {
		return nil, NewBusinessError("INSUFFICIENT_CAPACITY", "Insufficient campaign capacity", ErrInsufficientCampaignCapacity)
	}

	// TODO: ------------------------------------
	// TODO: ------------------------------------
	// TODO: ------------------------------------
	// TODO: validate datetime > now + 10 minutes
	// TODO: ------------------------------------
	// TODO: ------------------------------------
	// TODO: ------------------------------------

	// TODO: validate content, part, link, etc

	// TODO: validate budget against wallet

	// Use transaction for atomicity
	err = repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		// Update campaign
		return s.updateCampaign(txCtx, req, existingCampaign)
	})

	if err != nil {
		errMsg := fmt.Sprintf("Campaign update failed: %s", err.Error())
		_ = s.createAuditLog(ctx, customer, models.AuditActionCampaignUpdateFailed, errMsg, false, &errMsg, metadata)

		// TODO: ------------------------------------
		// TODO: ------------------------------------
		// TODO: ------------------------------------
		// TODO: reduce from wallet (if not enough, return error)
		// TODO: ------------------------------------
		// TODO: ------------------------------------
		// TODO: ------------------------------------
		// TODO: Store parameters used for calculating cost, capacity, etc (we can revert from balance if needed)
		// TODO: ------------------------------------
		// TODO: ------------------------------------
		// TODO: ------------------------------------
		return nil, NewBusinessError("CAMPAIGN_UPDATE_FAILED", "Campaign update failed", err)
	}

	// Log successful update
	msg := fmt.Sprintf("SMS campaign updated successfully: %s", existingCampaign.UUID.String())
	_ = s.createAuditLog(ctx, customer, models.AuditActionCampaignUpdated, msg, true, nil, metadata)

	// Build response
	response := &dto.UpdateSMSCampaignResponse{
		Message: "SMS campaign updated successfully",
	}

	return response, nil
}

// CalculateCampaignCapacity handles the SMS campaign capacity calculation process
func (s *SMSCampaignFlowImpl) CalculateCampaignCapacity(ctx context.Context, req *dto.CalculateCampaignCapacityRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCapacityResponse, error) {
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

// CalculateCampaignCost handles the SMS campaign cost calculation process
func (s *SMSCampaignFlowImpl) CalculateCampaignCost(ctx context.Context, req *dto.CalculateCampaignCostRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCostResponse, error) {
	// For now, just return a fixed cost of 14000
	// In the future, this could be enhanced with:
	// - Target audience analysis based on segment/subsegment
	// - Geographic reach calculation based on cities
	// - Budget-based cost estimation
	// - Historical campaign performance data
	// - Seasonal factors and market conditions
	// Calculate the number of SMS parts based on content length
	numPages := s.calculateSMSParts(req.Content)

	// Pricing constants
	basePrice := uint64(140)
	lineFactor := uint64(21)
	segmentFactor := float64(2.2)

	// Calculate price per message
	pricePerMsg := uint64(200*numPages) + basePrice*uint64(float64(lineFactor)*segmentFactor)

	// Calculate campaign capacity (target audience size)
	capacityReq := &dto.CalculateCampaignCapacityRequest{
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
	}

	capacityResp, err := s.CalculateCampaignCapacity(ctx, capacityReq, metadata)
	if err != nil {
		return nil, NewBusinessError("CAPACITY_CALCULATION_FAILED", "Failed to calculate campaign capacity", err)
	}

	taxCoefficient := 0.1

	availableCapacity := capacityResp.Capacity
	msgTarget := availableCapacity
	if req.Budget != nil {
		msgTarget = uint64(math.Min(float64(availableCapacity), float64(*req.Budget)*(1-taxCoefficient)/float64(pricePerMsg)))
	}

	// Calculate subtotal
	subtotal := pricePerMsg * msgTarget

	// Calculate tax (10%)
	tax := uint64(float64(subtotal) * taxCoefficient)

	// Calculate total
	total := subtotal + tax

	response := &dto.CalculateCampaignCostResponse{
		Message:      "Campaign cost calculated successfully",
		SubTotal:     subtotal,
		Tax:          tax,
		Total:        total,
		MsgTarget:    msgTarget,
		MaxMsgTarget: availableCapacity,
	}

	return response, nil
}

// calculateSMSParts calculates the number of SMS parts based on character count
// This implements the same logic as the frontend calculateSMSParts function
func (s *SMSCampaignFlowImpl) calculateSMSParts(content *string) uint64 {
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
func (s *SMSCampaignFlowImpl) countCharacters(text string) uint64 {
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

// validateCreateCampaignRequest validates the campaign creation request
func (s *SMSCampaignFlowImpl) validateCreateCampaignRequest(ctx context.Context, req *dto.CreateSMSCampaignRequest) error {
	// TODO: ------------------------------------
	// TODO: ------------------------------------
	if req.CustomerID == 0 {
		return fmt.Errorf("customer ID is required")
	}

	if req.Title != nil && *req.Title == "" {
		return fmt.Errorf("campaign title is required")
	}
	if req.Content != nil && *req.Content == "" {
		return fmt.Errorf("campaign content is required")
	}
	if req.Segment != nil && *req.Segment == "" {
		return fmt.Errorf("target segment is required")
	}
	if req.LineNumber != nil && *req.LineNumber == "" {
		return fmt.Errorf("line number is required")
	}
	if req.Budget != nil && *req.Budget <= 0 {
		return fmt.Errorf("budget is required and must be greater than 0")
	}
	if req.Sex != nil && *req.Sex == "" {
		return fmt.Errorf("sex is required")
	}
	if req.AdLink != nil && *req.AdLink == "" {
		return fmt.Errorf("ad link is required")
	}

	// Validate optional fields if provided
	if req.ScheduleAt != nil {
		if req.ScheduleAt.Before(time.Now().UTC()) {
			return fmt.Errorf("schedule time cannot be in the past")
		}
	}

	if len(req.City) > 0 {
		for _, city := range req.City {
			if city == "" {
				return fmt.Errorf("city names cannot be empty")
			}
		}
	}

	if len(req.Subsegment) > 0 {
		for _, subsegment := range req.Subsegment {
			if subsegment == "" {
				return fmt.Errorf("subsegment names cannot be empty")
			}
		}
	}

	return nil
}

// createCampaign creates the SMS campaign in the database
func (s *SMSCampaignFlowImpl) createCampaign(ctx context.Context, req *dto.CreateSMSCampaignRequest, customer *models.Customer) (*models.SMSCampaign, error) {
	// Build campaign spec

	spec := models.SMSCampaignSpec{}

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
	campaign := models.SMSCampaign{
		UUID:       uuid.New(),
		CustomerID: customer.ID,
		Status:     models.SMSCampaignStatusInitiated,
		Spec:       spec,
	}

	// Save to database
	err := s.campaignRepo.Create(ctx, campaign)
	if err != nil {
		return nil, fmt.Errorf("failed to save campaign to database: %w", err)
	}

	// Get the created campaign with ID
	createdCampaign, err := s.campaignRepo.ByUUID(ctx, campaign.UUID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve created campaign: %w", err)
	}

	return createdCampaign, nil
}

// createAuditLog creates an audit log entry for the campaign operation
func (s *SMSCampaignFlowImpl) createAuditLog(ctx context.Context, customer *models.Customer, action, description string, success bool, errorMsg *string, metadata *ClientMetadata) error {
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
	requestID := ctx.Value(RequestIDKey)
	if requestID != nil {
		requestIDStr, ok := requestID.(string)
		if ok {
			audit.RequestID = &requestIDStr
		}
	}

	return s.auditRepo.Save(ctx, audit)
}

// validateUpdateCampaignRequest validates the campaign update request
func (s *SMSCampaignFlowImpl) validateUpdateCampaignRequest(ctx context.Context, req *dto.UpdateSMSCampaignRequest) error {
	// TODO: ------------------------------------
	// TODO: ------------------------------------
	if req.UUID == "" {
		return fmt.Errorf("campaign UUID is required")
	}

	if req.CustomerID == 0 {
		return fmt.Errorf("customer ID is required")
	}

	// At least one field should be provided for update
	hasUpdateFields := req.Title != nil || req.Segment != nil || len(req.Subsegment) > 0 ||
		req.Sex != nil || len(req.City) > 0 || req.AdLink != nil || req.Content != nil ||
		req.ScheduleAt != nil || req.LineNumber != nil || req.Budget != nil

	if !hasUpdateFields {
		return fmt.Errorf("at least one field must be provided for update")
	}

	return nil
}

// canUpdateCampaign checks if a campaign can be updated based on its current status
func (s *SMSCampaignFlowImpl) canUpdateCampaign(status models.SMSCampaignStatus) bool {
	// Only campaigns with 'initiated' or 'in-progress' status can be updated
	// Campaigns with 'waiting-for-approval', 'approved', or 'rejected' status cannot be updated
	return status == models.SMSCampaignStatusInitiated || status == models.SMSCampaignStatusInProgress
}

// updateCampaign updates the SMS campaign in the database
func (s *SMSCampaignFlowImpl) updateCampaign(ctx context.Context, req *dto.UpdateSMSCampaignRequest, existingCampaign *models.SMSCampaign) error {
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
	existingCampaign.Status = models.SMSCampaignStatusWaitingForApproval

	// Save to database
	err := s.campaignRepo.Update(ctx, *existingCampaign)
	if err != nil {
		return fmt.Errorf("failed to update campaign in database: %w", err)
	}

	return nil
}

// GetWalletBalance handles the user wallet balance retrieval process
func (s *SMSCampaignFlowImpl) GetWalletBalance(ctx context.Context, req *dto.GetWalletBalanceRequest, metadata *ClientMetadata) (*dto.GetWalletBalanceResponse, error) {
	// Verify customer exists and is active
	customer, err := s.customerRepo.ByID(ctx, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CUSTOMER_LOOKUP_FAILED", "Failed to lookup customer", err)
	}
	if customer == nil {
		return nil, NewBusinessError("CUSTOMER_NOT_FOUND", "Customer not found", ErrCustomerNotFound)
	}
	if !utils.IsTrue(customer.IsActive) {
		return nil, NewBusinessError("CUSTOMER_INACTIVE", "Customer account is inactive", ErrAccountInactive)
	}

	// TODO: In the future, this will query the actual wallet database
	// For now, return mock data based on customer ID for demonstration
	mockBalance := s.generateMockWalletBalance(req.CustomerID)

	response := &dto.GetWalletBalanceResponse{
		Message:             "Wallet balance retrieved successfully",
		Free:                mockBalance.Free,
		Locked:              mockBalance.Locked,
		Frozen:              mockBalance.Frozen,
		Total:               mockBalance.Total,
		Currency:            mockBalance.Currency,
		LastUpdated:         mockBalance.LastUpdated,
		PendingTransactions: mockBalance.PendingTransactions,
		MinimumBalance:      mockBalance.MinimumBalance,
		CreditLimit:         mockBalance.CreditLimit,
		BalanceStatus:       mockBalance.BalanceStatus,
	}

	return response, nil
}

// generateMockWalletBalance generates mock wallet balance data for demonstration
// In the future, this will be replaced with actual database queries
func (s *SMSCampaignFlowImpl) generateMockWalletBalance(customerID uint) *dto.GetWalletBalanceResponse {
	// Generate different mock data based on customer ID for variety
	seed := customerID % 1000

	// Base values with some variation
	baseFree := uint64(1000000 + int(seed)*50000)  // 1M to 1.5M
	baseLocked := uint64(200000 + int(seed)*10000) // 200K to 300K
	baseFrozen := uint64(seed % 100000)            // 0 to 100K

	// Calculate totals
	total := baseFree + baseLocked + baseFrozen

	// Generate realistic mock data
	return &dto.GetWalletBalanceResponse{
		Free:                baseFree,
		Locked:              baseLocked,
		Frozen:              baseFrozen,
		Total:               total,
		Currency:            "IRR",
		LastUpdated:         time.Now().UTC().Format(time.RFC3339),
		PendingTransactions: uint64(seed % 5),                   // 0 to 4
		MinimumBalance:      100000,                             // 100K minimum
		CreditLimit:         uint64(5000000 + int(seed)*100000), // 5M to 6M
		BalanceStatus:       "active",
	}
}

// ListCampaigns retrieves user's campaigns with pagination, ordering and filters
func (s *SMSCampaignFlowImpl) ListCampaigns(ctx context.Context, req *dto.ListSMSCampaignsRequest, metadata *ClientMetadata) (*dto.ListSMSCampaignsResponse, error) {
	// Validate customer
	customer, err := s.customerRepo.ByID(ctx, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CUSTOMER_LOOKUP_FAILED", "Failed to lookup customer", err)
	}
	if customer == nil {
		return nil, NewBusinessError("CUSTOMER_NOT_FOUND", "Customer not found", ErrCustomerNotFound)
	}
	if !utils.IsTrue(customer.IsActive) {
		return nil, NewBusinessError("ACCOUNT_INACTIVE", "Customer account is inactive", ErrAccountInactive)
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
	filter := models.SMSCampaignFilter{CustomerID: &req.CustomerID}
	if req.Filter != nil {
		if req.Filter.Title != nil && *req.Filter.Title != "" {
			filter.Title = req.Filter.Title
		}
		if req.Filter.Status != nil && *req.Filter.Status != "" {
			status := models.SMSCampaignStatus(*req.Filter.Status)
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
		return nil, fmt.Errorf("failed to count campaigns: %w", err)
	}

	// Fetch items
	items, err := s.campaignRepo.ByFilter(ctx, filter, orderBy, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list campaigns: %w", err)
	}

	// Map to response items
	respItems := make([]dto.GetSMSCampaignResponse, 0, len(items))
	for _, c := range items {
		respItems = append(respItems, dto.GetSMSCampaignResponse{
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

	return &dto.ListSMSCampaignsResponse{
		Message:    "Campaigns retrieved successfully",
		Items:      respItems,
		Pagination: dto.PaginationInfo{Total: total64, Page: page, Limit: limit, TotalPages: totalPages},
	}, nil
}
