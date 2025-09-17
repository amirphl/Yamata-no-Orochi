// Package handlers contains HTTP request handlers and presentation layer logic for the API endpoints
package handlers

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
)

// CampaignHandlerInterface defines the contract for campaign handlers
type CampaignHandlerInterface interface {
	CreateCampaign(c fiber.Ctx) error
	UpdateCampaign(c fiber.Ctx) error
	CalculateCampaignCapacity(c fiber.Ctx) error
	CalculateCampaignCost(c fiber.Ctx) error
	ListCampaigns(c fiber.Ctx) error
}

// CampaignHandler handles campaign-related HTTP requests
type CampaignHandler struct {
	campaignFlow businessflow.CampaignFlow
	validator    *validator.Validate
}

func (h *CampaignHandler) ErrorResponse(c fiber.Ctx, statusCode int, message, errorCode string, details any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: false,
		Message: message,
		Error: dto.ErrorDetail{
			Code:    errorCode,
			Details: details,
		},
	})
}

func (h *CampaignHandler) SuccessResponse(c fiber.Ctx, statusCode int, message string, data any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// NewCampaignHandler creates a new campaign handler
func NewCampaignHandler(campaignFlow businessflow.CampaignFlow) *CampaignHandler {
	handler := &CampaignHandler{
		campaignFlow: campaignFlow,
		validator:    validator.New(),
	}

	// Setup custom validations
	handler.setupCustomValidations()

	return handler
}

// CreateCampaign handles the campaign creation process
// @Summary Create Campaign
// @Description Create a new campaign with the specified parameters
// @Tags Campaigns
// @Accept json
// @Produce json
// @Param request body dto.CreateCampaignRequest true "Campaign creation data"
// @Success 201 {object} dto.APIResponse{data=dto.CreateCampaignResponse} "Campaign created successfully"
// @Failure 400 {object} dto.APIResponse "Validation error or invalid request"
// @Failure 401 {object} dto.APIResponse "Unauthorized - customer not found or inactive"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/campaigns [post]
func (h *CampaignHandler) CreateCampaign(c fiber.Ctx) error {
	var req dto.CreateCampaignRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	// Validate request
	if err := h.validator.Struct(&req); err != nil {
		var validationErrors []string
		for _, err := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, getValidationErrorMessage(err))
		}
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", validationErrors)
	}

	// Get client information
	metadata := businessflow.NewClientMetadata(c.IP(), c.Get("User-Agent"))

	// Get authenticated customer ID from context
	customerID, ok := c.Locals("customer_id").(uint)
	if !ok {
		return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer ID not found in context", "MISSING_CUSTOMER_ID", nil)
	}

	// Set the authenticated customer ID in the request
	req.CustomerID = customerID

	// Call business logic with proper context
	result, err := h.campaignFlow.CreateCampaign(h.createRequestContext(c, "/api/v1/campaigns"), &req, metadata)
	if err != nil {
		// Handle specific business errors
		if businessflow.IsCustomerNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer not found", "CUSTOMER_NOT_FOUND", nil)
		}
		if businessflow.IsAccountInactive(err) {
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer account is inactive", "ACCOUNT_INACTIVE", nil)
		}

		log.Println("Campaign creation failed", err)
		// Handle generic business errors
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Campaign creation failed", "CAMPAIGN_CREATION_FAILED", nil)
	}

	// Successful campaign creation
	return h.SuccessResponse(c, fiber.StatusCreated, "Campaign created successfully", fiber.Map{
		"message":    result.Message,
		"uuid":       result.UUID,
		"status":     result.Status,
		"created_at": result.CreatedAt,
	})
}

// UpdateCampaign handles the campaign update process
// @Summary Update Campaign
// @Description Update an existing campaign with the specified parameters
// @Tags Campaigns
// @Accept json
// @Produce json
// @Param uuid path string true "Campaign UUID"
// @Param request body dto.UpdateCampaignRequest true "Campaign update data"
// @Success 200 {object} dto.APIResponse{data=dto.UpdateCampaignResponse} "Campaign updated successfully"
// @Failure 400 {object} dto.APIResponse "Validation error or invalid request"
// @Failure 401 {object} dto.APIResponse "Unauthorized - customer not found or inactive"
// @Failure 403 {object} dto.APIResponse "Forbidden - campaign access denied or update not allowed"
// @Failure 404 {object} dto.APIResponse "Campaign not found"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/campaigns/{uuid} [put]
func (h *CampaignHandler) UpdateCampaign(c fiber.Ctx) error {
	// Get campaign UUID from path parameter
	campaignUUID := c.Params("uuid")
	if campaignUUID == "" {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Campaign UUID is required", "MISSING_CAMPAIGN_UUID", nil)
	}

	var req dto.UpdateCampaignRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	// Set the campaign UUID and customer ID in the request
	req.UUID = campaignUUID

	// Get authenticated customer ID from context
	customerID, ok := c.Locals("customer_id").(uint)
	if !ok {
		return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer ID not found in context", "MISSING_CUSTOMER_ID", nil)
	}
	req.CustomerID = customerID

	// Validate request
	if err := h.validator.Struct(&req); err != nil {
		var validationErrors []string
		for _, err := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, getValidationErrorMessage(err))
		}
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", validationErrors)
	}

	// Get client information
	metadata := businessflow.NewClientMetadata(c.IP(), c.Get("User-Agent"))

	// Call business logic with proper context
	result, err := h.campaignFlow.UpdateCampaign(h.createRequestContext(c, "/api/v1/campaigns/"+campaignUUID), &req, metadata)
	if err != nil {
		// Handle specific business errors
		if businessflow.IsCustomerNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer not found", "CUSTOMER_NOT_FOUND", nil)
		}
		if businessflow.IsAccountInactive(err) {
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer account is inactive", "ACCOUNT_INACTIVE", nil)
		}
		if businessflow.IsCampaignNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Campaign not found", "CAMPAIGN_NOT_FOUND", nil)
		}
		if businessflow.IsCampaignAccessDenied(err) {
			return h.ErrorResponse(c, fiber.StatusForbidden, "Access denied: campaign belongs to another customer", "CAMPAIGN_ACCESS_DENIED", nil)
		}
		if businessflow.IsCampaignUpdateNotAllowed(err) {
			return h.ErrorResponse(c, fiber.StatusForbidden, "Campaign cannot be updated in current status", "CAMPAIGN_UPDATE_NOT_ALLOWED", nil)
		}
		if businessflow.IsScheduleTimeTooSoon(err) {
			return h.ErrorResponse(c, fiber.StatusForbidden, "Schedule time is too soon", "SCHEDULE_TIME_TOO_SOON", nil)
		}
		if businessflow.IsInsufficientCampaignCapacity(err) {
			return h.ErrorResponse(c, fiber.StatusForbidden, "Insufficient campaign capacity", "INSUFFICIENT_CAMPAIGN_CAPACITY", nil)
		}

		if businessflow.IsCampaignTitleRequired(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Campaign title is required", "CAMPAIGN_TITLE_REQUIRED", nil)
		}
		if businessflow.IsCampaignSegmentRequired(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Campaign segment is required", "CAMPAIGN_SEGMENT_REQUIRED", nil)
		}
		if businessflow.IsCampaignSubsegmentRequired(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Campaign subsegment is required", "CAMPAIGN_SUBSEGMENT_REQUIRED", nil)
		}
		if businessflow.IsCampaignContentRequired(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Campaign content is required", "CAMPAIGN_CONTENT_REQUIRED", nil)
		}
		if businessflow.IsScheduleTimeNotPresent(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Schedule time is not present", "SCHEDULE_TIME_NOT_PRESENT", nil)
		}
		if businessflow.IsScheduleTimeTooSoon(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Schedule time is too soon", "SCHEDULE_TIME_TOO_SOON", nil)
		}
		if businessflow.IsCampaignLineNumberRequired(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Campaign line number is required", "CAMPAIGN_LINE_NUMBER_REQUIRED", nil)
		}
		if businessflow.IsCampaignBudgetRequired(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Campaign budget is required", "CAMPAIGN_BUDGET_REQUIRED", nil)
		}

		log.Println("Campaign update failed", err)
		// Handle generic business errors
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Campaign update failed", "CAMPAIGN_UPDATE_FAILED", nil)
	}

	// Successful campaign update
	return h.SuccessResponse(c, fiber.StatusOK, "Campaign updated successfully", fiber.Map{
		"message": result.Message,
	})
}

// CalculateCampaignCapacity handles the campaign capacity calculation process
// @Summary Calculate Campaign Capacity
// @Description Calculate the potential reach and capacity of an campaign based on parameters
// @Tags Campaigns
// @Accept json
// @Produce json
// @Param request body dto.CalculateCampaignCapacityRequest true "Campaign parameters for capacity calculation"
// @Success 200 {object} dto.APIResponse{data=dto.CalculateCampaignCapacityResponse} "Capacity calculated successfully"
// @Failure 400 {object} dto.APIResponse "Validation error or invalid request"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/campaigns/calculate-capacity [post]
func (h *CampaignHandler) CalculateCampaignCapacity(c fiber.Ctx) error {
	var req dto.CalculateCampaignCapacityRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	// Validate request
	if err := h.validator.Struct(&req); err != nil {
		var validationErrors []string
		for _, err := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, getValidationErrorMessage(err))
		}
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", validationErrors)
	}

	// Get client information
	metadata := businessflow.NewClientMetadata(c.IP(), c.Get("User-Agent"))

	// Call business logic with proper context
	result, err := h.campaignFlow.CalculateCampaignCapacity(h.createRequestContext(c, "/api/v1/campaigns/calculate-capacity"), &req, metadata)
	if err != nil {
		log.Println("Campaign capacity calculation failed", err)
		// Handle generic business errors
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Campaign capacity calculation failed", "CAPACITY_CALCULATION_FAILED", nil)
	}

	// Successful capacity calculation
	return h.SuccessResponse(c, fiber.StatusOK, "Campaign capacity calculated successfully", fiber.Map{
		"message":  result.Message,
		"capacity": result.Capacity,
	})
}

// CalculateCampaignCost handles the campaign cost calculation process
// @Summary Calculate Campaign Cost
// @Description Calculate the total cost of an campaign based on content, target audience, and pricing factors
// @Tags Campaigns
// @Accept json
// @Produce json
// @Param request body dto.CalculateCampaignCostRequest true "Campaign parameters for cost calculation"
// @Success 200 {object} dto.APIResponse{data=dto.CalculateCampaignCostResponse} "Cost calculated successfully"
// @Failure 400 {object} dto.APIResponse "Validation error or invalid request"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/campaigns/calculate-cost [post]
func (h *CampaignHandler) CalculateCampaignCost(c fiber.Ctx) error {
	var req dto.CalculateCampaignCostRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	// Validate request
	if err := h.validator.Struct(&req); err != nil {
		var validationErrors []string
		for _, err := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, getValidationErrorMessage(err))
		}
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", validationErrors)
	}

	// Get client information
	metadata := businessflow.NewClientMetadata(c.IP(), c.Get("User-Agent"))

	// Call business logic with proper context
	result, err := h.campaignFlow.CalculateCampaignCost(h.createRequestContext(c, "/api/v1/campaigns/calculate-cost"), &req, metadata)
	if err != nil {
		log.Println("Campaign cost calculation failed", err)
		// Handle generic business errors
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Campaign cost calculation failed", "COST_CALCULATION_FAILED", nil)
	}

	// Successful cost calculation
	return h.SuccessResponse(c, fiber.StatusOK, "Campaign cost calculated successfully", fiber.Map{
		"message":        result.Message,
		"total":          result.Total,
		"msg_target":     result.MsgTarget,
		"max_msg_target": result.MaxMsgTarget,
	})
}

// ListCampaigns returns user's campaigns with filters and pagination
// @Summary List Campaigns
// @Description Retrieve the authenticated user's campaigns with pagination, ordering, and filters
// @Tags Campaigns
// @Accept json
// @Produce json
// @Param page query int true "Page number"
// @Param limit query int true "Items per page (max 100)"
// @Param orderby query string false "Order by (newest|oldest)" default(newest)
// @Param title query string false "Filter by title (contains)"
// @Param status query string false "Filter by status (initiated|in-progress|waiting-for-approval|approved|rejected)"
// @Success 200 {object} dto.APIResponse{data=dto.ListCampaignsResponse}
// @Failure 400 {object} dto.APIResponse "Validation error"
// @Failure 401 {object} dto.APIResponse "Unauthorized"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/campaigns [get]
func (h *CampaignHandler) ListCampaigns(c fiber.Ctx) error {
	// Parse query params
	pageStr := c.Query("page", "1")
	limitStr := c.Query("limit", "10")
	page := 1
	if v, err := strconv.Atoi(pageStr); err == nil && v > 0 {
		page = v
	}
	limit := 10
	if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
		limit = v
	}
	if limit > 100 {
		limit = 100
	}
	orderby := c.Query("orderby", "newest")
	title := c.Query("title")
	status := c.Query("status")

	// Get authenticated customer ID
	customerID, ok := c.Locals("customer_id").(uint)
	if !ok {
		return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer ID not found in context", "MISSING_CUSTOMER_ID", nil)
	}

	// Build request DTO
	var filter *dto.ListCampaignsFilter
	if title != "" || status != "" {
		filter = &dto.ListCampaignsFilter{}
		if title != "" {
			filter.Title = &title
		}
		if status != "" {
			filter.Status = &status
		}
	}
	req := &dto.ListCampaignsRequest{
		CustomerID: customerID,
		Page:       page,
		Limit:      limit,
		OrderBy:    orderby,
		Filter:     filter,
	}

	// Client metadata
	metadata := businessflow.NewClientMetadata(c.IP(), c.Get("User-Agent"))

	// Call business logic
	result, err := h.campaignFlow.ListCampaigns(h.createRequestContext(c, "/api/v1/campaigns"), req, metadata)
	if err != nil {
		if businessflow.IsCustomerNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer not found", "CUSTOMER_NOT_FOUND", nil)
		}
		if businessflow.IsAccountInactive(err) {
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer account is inactive", "ACCOUNT_INACTIVE", nil)
		}

		log.Println("List campaigns failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to list campaigns", "LIST_CAMPAIGNS_FAILED", nil)
	}

	return h.SuccessResponse(c, fiber.StatusOK, "Campaigns retrieved successfully", fiber.Map{
		"message":    result.Message,
		"items":      result.Items,
		"pagination": result.Pagination,
	})
}

// createRequestContext creates a context with request-scoped values for observability and timeout
func (h *CampaignHandler) createRequestContext(c fiber.Ctx, endpoint string) context.Context {
	return h.createRequestContextWithTimeout(c, endpoint, 30*time.Second)
}

// createRequestContextWithTimeout creates a context with custom timeout and request-scoped values
func (h *CampaignHandler) createRequestContextWithTimeout(c fiber.Ctx, endpoint string, timeout time.Duration) context.Context {
	// Create context with custom timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Add request-scoped values for observability
	ctx = context.WithValue(ctx, utils.RequestIDKey, c.Get("X-Request-ID"))
	ctx = context.WithValue(ctx, utils.UserAgentKey, c.Get("User-Agent"))
	ctx = context.WithValue(ctx, utils.IPAddressKey, c.IP())
	ctx = context.WithValue(ctx, utils.EndpointKey, endpoint)
	ctx = context.WithValue(ctx, utils.TimeoutKey, timeout)
	ctx = context.WithValue(ctx, utils.CancelFuncKey, cancel) // Store cancel function for cleanup

	return ctx
}

// setupCustomValidations sets up custom validation rules
func (h *CampaignHandler) setupCustomValidations() {
	// Add custom validation rules if needed
	// Example: h.validator.RegisterValidation("custom_rule", customValidationFunc)
}
