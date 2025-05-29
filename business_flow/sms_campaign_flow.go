// Package businessflow contains the core business logic and use cases for SMS campaign workflows
package businessflow

import (
	"context"
	"fmt"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"gorm.io/gorm"
)

// SMSCampaignFlow handles the SMS campaign business logic
type SMSCampaignFlow interface {
	CreateCampaign(ctx context.Context, req *dto.CreateSMSCampaignRequest, metadata *ClientMetadata) (*dto.CreateSMSCampaignResponse, error)
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

// validateCreateCampaignRequest validates the campaign creation request
func (s *SMSCampaignFlowImpl) validateCreateCampaignRequest(ctx context.Context, req *dto.CreateSMSCampaignRequest) error {
	if req.CustomerID == 0 {
		return fmt.Errorf("customer ID is required")
	}

	// Validate required fields
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
		spec.Title = *req.Title
	}
	if req.Segment != nil && *req.Segment != "" {
		spec.Segment = *req.Segment
	}
	if len(req.Subsegment) > 0 {
		spec.Subsegment = req.Subsegment
	}
	if req.Sex != nil && *req.Sex != "" {
		spec.Sex = *req.Sex
	}
	if len(req.City) > 0 {
		spec.City = req.City
	}
	if req.AdLink != nil && *req.AdLink != "" {
		spec.AdLink = *req.AdLink
	}
	if req.Content != nil && *req.Content != "" {
		spec.Content = *req.Content
	}
	if req.ScheduleAt != nil {
		spec.ScheduleAt = req.ScheduleAt
	}
	if req.LineNumber != nil && *req.LineNumber != "" {
		spec.LineNumber = *req.LineNumber
	}
	if req.Budget != nil && *req.Budget != 0 {
		spec.Budget = *req.Budget
	}

	// Create campaign model
	campaign := models.SMSCampaign{
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
	createdCampaign, err := s.campaignRepo.ByID(ctx, campaign.ID)
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
