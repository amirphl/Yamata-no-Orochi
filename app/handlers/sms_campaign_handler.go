// Package handlers contains HTTP request handlers and presentation layer logic for the API endpoints
package handlers

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
)

// SMSCampaignHandlerInterface defines the contract for SMS campaign handlers
type SMSCampaignHandlerInterface interface {
	CreateCampaign(c fiber.Ctx) error
	UpdateCampaign(c fiber.Ctx) error
	CalculateCampaignCapacity(c fiber.Ctx) error
	CalculateCampaignCost(c fiber.Ctx) error
	GetWalletBalance(c fiber.Ctx) error
	ListCampaigns(c fiber.Ctx) error
}

// SMSCampaignHandler handles SMS campaign-related HTTP requests
type SMSCampaignHandler struct {
	campaignFlow businessflow.SMSCampaignFlow
	validator    *validator.Validate
}

func (h *SMSCampaignHandler) ErrorResponse(c fiber.Ctx, statusCode int, message, errorCode string, details any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: false,
		Message: message,
		Error: dto.ErrorDetail{
			Code:    errorCode,
			Details: details,
		},
	})
}

func (h *SMSCampaignHandler) SuccessResponse(c fiber.Ctx, statusCode int, message string, data any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// NewSMSCampaignHandler creates a new SMS campaign handler
func NewSMSCampaignHandler(campaignFlow businessflow.SMSCampaignFlow) *SMSCampaignHandler {
	handler := &SMSCampaignHandler{
		campaignFlow: campaignFlow,
		validator:    validator.New(),
	}

	// Setup custom validations
	handler.setupCustomValidations()

	return handler
}

// CreateCampaign handles the SMS campaign creation process
// @Summary Create SMS Campaign
// @Description Create a new SMS campaign with the specified parameters
// @Tags SMS Campaigns
// @Accept json
// @Produce json
// @Param request body dto.CreateSMSCampaignRequest true "SMS campaign creation data"
// @Success 201 {object} dto.APIResponse{data=dto.CreateSMSCampaignResponse} "Campaign created successfully"
// @Failure 400 {object} dto.APIResponse "Validation error or invalid request"
// @Failure 401 {object} dto.APIResponse "Unauthorized - customer not found or inactive"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/sms-campaigns [post]
func (h *SMSCampaignHandler) CreateCampaign(c fiber.Ctx) error {
	var req dto.CreateSMSCampaignRequest
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
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Get authenticated customer ID from context
	customerID, ok := c.Locals("customer_id").(uint)
	if !ok {
		return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer ID not found in context", "MISSING_CUSTOMER_ID", nil)
	}

	// Set the authenticated customer ID in the request
	req.CustomerID = customerID

	// Call business logic with proper context
	result, err := h.campaignFlow.CreateCampaign(h.createRequestContext(c, "/api/v1/sms-campaigns"), &req, metadata)
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
	return h.SuccessResponse(c, fiber.StatusCreated, "SMS campaign created successfully", fiber.Map{
		"message":    result.Message,
		"uuid":       result.UUID,
		"status":     result.Status,
		"created_at": result.CreatedAt,
	})
}

// UpdateCampaign handles the SMS campaign update process
// @Summary Update SMS Campaign
// @Description Update an existing SMS campaign with the specified parameters
// @Tags SMS Campaigns
// @Accept json
// @Produce json
// @Param uuid path string true "Campaign UUID"
// @Param request body dto.UpdateSMSCampaignRequest true "SMS campaign update data"
// @Success 200 {object} dto.APIResponse{data=dto.UpdateSMSCampaignResponse} "Campaign updated successfully"
// @Failure 400 {object} dto.APIResponse "Validation error or invalid request"
// @Failure 401 {object} dto.APIResponse "Unauthorized - customer not found or inactive"
// @Failure 403 {object} dto.APIResponse "Forbidden - campaign access denied or update not allowed"
// @Failure 404 {object} dto.APIResponse "Campaign not found"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/sms-campaigns/{uuid} [put]
func (h *SMSCampaignHandler) UpdateCampaign(c fiber.Ctx) error {
	// Get campaign UUID from path parameter
	campaignUUID := c.Params("uuid")
	if campaignUUID == "" {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Campaign UUID is required", "MISSING_CAMPAIGN_UUID", nil)
	}

	var req dto.UpdateSMSCampaignRequest
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
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business logic with proper context
	result, err := h.campaignFlow.UpdateCampaign(h.createRequestContext(c, "/api/v1/sms-campaigns/"+campaignUUID), &req, metadata)
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

		log.Println("Campaign update failed", err)
		// Handle generic business errors
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Campaign update failed", "CAMPAIGN_UPDATE_FAILED", nil)
	}

	// Successful campaign update
	return h.SuccessResponse(c, fiber.StatusOK, "SMS campaign updated successfully", fiber.Map{
		"message": result.Message,
	})
}

// CalculateCampaignCapacity handles the SMS campaign capacity calculation process
// @Summary Calculate SMS Campaign Capacity
// @Description Calculate the potential reach and capacity of an SMS campaign based on parameters
// @Tags SMS Campaigns
// @Accept json
// @Produce json
// @Param request body dto.CalculateCampaignCapacityRequest true "Campaign parameters for capacity calculation"
// @Success 200 {object} dto.APIResponse{data=dto.CalculateCampaignCapacityResponse} "Capacity calculated successfully"
// @Failure 400 {object} dto.APIResponse "Validation error or invalid request"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/sms-campaigns/calculate-capacity [post]
func (h *SMSCampaignHandler) CalculateCampaignCapacity(c fiber.Ctx) error {
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
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business logic with proper context
	result, err := h.campaignFlow.CalculateCampaignCapacity(h.createRequestContext(c, "/api/v1/sms-campaigns/calculate-capacity"), &req, metadata)
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

// CalculateCampaignCost handles the SMS campaign cost calculation process
// @Summary Calculate SMS Campaign Cost
// @Description Calculate the total cost of an SMS campaign based on content, target audience, and pricing factors
// @Tags SMS Campaigns
// @Accept json
// @Produce json
// @Param request body dto.CalculateCampaignCostRequest true "Campaign parameters for cost calculation"
// @Success 200 {object} dto.APIResponse{data=dto.CalculateCampaignCostResponse} "Cost calculated successfully"
// @Failure 400 {object} dto.APIResponse "Validation error or invalid request"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/sms-campaigns/calculate-cost [post]
func (h *SMSCampaignHandler) CalculateCampaignCost(c fiber.Ctx) error {
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
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business logic with proper context
	result, err := h.campaignFlow.CalculateCampaignCost(h.createRequestContext(c, "/api/v1/sms-campaigns/calculate-cost"), &req, metadata)
	if err != nil {
		log.Println("Campaign cost calculation failed", err)
		// Handle generic business errors
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Campaign cost calculation failed", "COST_CALCULATION_FAILED", nil)
	}

	// Successful cost calculation
	return h.SuccessResponse(c, fiber.StatusOK, "Campaign cost calculated successfully", fiber.Map{
		"message":        result.Message,
		"sub_total":      result.SubTotal,
		"tax":            result.Tax,
		"total":          result.Total,
		"msg_target":     result.MsgTarget,
		"max_msg_target": result.MaxMsgTarget,
	})
}

// GetWalletBalance handles the user wallet balance retrieval process
// @Summary Get User Wallet Balance
// @Description Retrieve the current wallet balance and financial information for the authenticated user
// @Tags Wallet
// @Accept json
// @Produce json
// @Success 200 {object} dto.APIResponse{data=dto.GetWalletBalanceResponse} "Wallet balance retrieved successfully"
// @Failure 401 {object} dto.APIResponse "Unauthorized - customer not found or inactive"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/wallet/balance [get]
func (h *SMSCampaignHandler) GetWalletBalance(c fiber.Ctx) error {
	// Get authenticated customer ID from context
	customerID, ok := c.Locals("customer_id").(uint)
	if !ok {
		return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer ID not found in context", "MISSING_CUSTOMER_ID", nil)
	}

	// Create request
	req := &dto.GetWalletBalanceRequest{
		CustomerID: customerID,
	}

	// Get client information
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business logic with proper context
	result, err := h.campaignFlow.GetWalletBalance(h.createRequestContext(c, "/api/v1/wallet/balance"), req, metadata)
	if err != nil {
		// Handle specific business errors
		if businessflow.IsCustomerNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer not found", "CUSTOMER_NOT_FOUND", nil)
		}
		if businessflow.IsAccountInactive(err) {
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer account is inactive", "ACCOUNT_INACTIVE", nil)
		}

		log.Println("Wallet balance retrieval failed", err)
		// Handle generic business errors
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Wallet balance retrieval failed", "WALLET_BALANCE_RETRIEVAL_FAILED", nil)
	}

	// Successful wallet balance retrieval
	return h.SuccessResponse(c, fiber.StatusOK, "Wallet balance retrieved successfully", fiber.Map{
		"message":              result.Message,
		"free":                 result.Free,
		"locked":               result.Locked,
		"frozen":               result.Frozen,
		"total":                result.Total,
		"currency":             result.Currency,
		"last_updated":         result.LastUpdated,
		"pending_transactions": result.PendingTransactions,
		"minimum_balance":      result.MinimumBalance,
		"credit_limit":         result.CreditLimit,
		"balance_status":       result.BalanceStatus,
	})
}

// ListCampaigns returns user's campaigns with filters and pagination
// @Summary List SMS Campaigns
// @Description Retrieve the authenticated user's campaigns with pagination, ordering, and filters
// @Tags SMS Campaigns
// @Accept json
// @Produce json
// @Param page query int true "Page number"
// @Param limit query int true "Items per page (max 100)"
// @Param orderby query string false "Order by (newest|oldest)" default(newest)
// @Param title query string false "Filter by title (contains)"
// @Param status query string false "Filter by status (initiated|in-progress|waiting-for-approval|approved|rejected)"
// @Success 200 {object} dto.APIResponse{data=dto.ListSMSCampaignsResponse}
// @Failure 400 {object} dto.APIResponse "Validation error"
// @Failure 401 {object} dto.APIResponse "Unauthorized"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/sms-campaigns [get]
func (h *SMSCampaignHandler) ListCampaigns(c fiber.Ctx) error {
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
	var filter *dto.ListSMSCampaignsFilter
	if title != "" || status != "" {
		filter = &dto.ListSMSCampaignsFilter{}
		if title != "" {
			filter.Title = &title
		}
		if status != "" {
			filter.Status = &status
		}
	}
	req := &dto.ListSMSCampaignsRequest{
		CustomerID: customerID,
		Page:       page,
		Limit:      limit,
		OrderBy:    orderby,
		Filter:     filter,
	}

	// Client metadata
	metadata := businessflow.NewClientMetadata(c.IP(), c.Get("User-Agent"))

	// Call business logic
	result, err := h.campaignFlow.ListCampaigns(h.createRequestContext(c, "/api/v1/sms-campaigns"), req, metadata)
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
func (h *SMSCampaignHandler) createRequestContext(c fiber.Ctx, endpoint string) context.Context {
	return h.createRequestContextWithTimeout(c, endpoint, 30*time.Second)
}

// createRequestContextWithTimeout creates a context with custom timeout and request-scoped values
func (h *SMSCampaignHandler) createRequestContextWithTimeout(c fiber.Ctx, endpoint string, timeout time.Duration) context.Context {
	// Create context with custom timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Add request-scoped values for observability
	ctx = context.WithValue(ctx, "request_id", c.Get("X-Request-ID"))
	ctx = context.WithValue(ctx, "user_agent", c.Get("User-Agent"))
	ctx = context.WithValue(ctx, "ip_address", c.IP())
	ctx = context.WithValue(ctx, "endpoint", endpoint)
	ctx = context.WithValue(ctx, "timeout", timeout)
	ctx = context.WithValue(ctx, "cancel_func", cancel) // Store cancel function for cleanup

	return ctx
}

// setupCustomValidations sets up custom validation rules
func (h *SMSCampaignHandler) setupCustomValidations() {
	// Add custom validation rules if needed
	// Example: h.validator.RegisterValidation("custom_rule", customValidationFunc)
}
