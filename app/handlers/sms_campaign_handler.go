// Package handlers contains HTTP request handlers and presentation layer logic for the API endpoints
package handlers

import (
	"context"
	"log"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
)

// SMSCampaignHandlerInterface defines the contract for SMS campaign handlers
type SMSCampaignHandlerInterface interface {
	CreateCampaign(c fiber.Ctx) error
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
