// Package handlers contains HTTP request handlers and presentation layer logic for the API endpoints
package handlers

import (
	"context"
	"log"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
)

// CampaignAdminHandlerInterface defines the contract for campaign admin handlers
type CampaignAdminHandlerInterface interface {
	ListCampaigns(c fiber.Ctx) error
	ApproveCampaign(c fiber.Ctx) error
	RejectCampaign(c fiber.Ctx) error
}

// CampaignAdminHandler handles campaign-related HTTP requests
type CampaignAdminHandler struct {
	campaignFlow businessflow.AdminCampaignFlow
	validator    *validator.Validate
}

func NewCampaignAdminHandler(flow businessflow.AdminCampaignFlow) CampaignAdminHandlerInterface {
	h := &CampaignAdminHandler{
		campaignFlow: flow,
		validator:    validator.New(),
	}
	h.setupCustomValidations()
	return h
}

func (h *CampaignAdminHandler) ErrorResponse(c fiber.Ctx, statusCode int, message, errorCode string, details any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: false,
		Message: message,
		Error: dto.ErrorDetail{
			Code:    errorCode,
			Details: details,
		},
	})
}

func (h *CampaignAdminHandler) SuccessResponse(c fiber.Ctx, statusCode int, message string, data any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// ListCampaigns returns campaigns filtered for admin
// @Summary Admin List Campaigns
// @Description Retrieve campaigns by title, status, start date, and end date
// @Tags Admin Campaigns
// @Produce json
// @Param title query string false "Filter by title (contains)"
// @Param status query string false "Filter by status (initiated|in_progress|waiting_for_approval|approved|rejected)"
// @Param start_date query string false "Filter created_at >= start_date (RFC3339)"
// @Param end_date query string false "Filter created_at <= end_date (RFC3339)"
// @Success 200 {object} dto.APIResponse{data=[]dto.GetCampaignResponse}
// @Failure 400 {object} dto.APIResponse "Validation error"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/admin/campaigns [get]
func (h *CampaignAdminHandler) ListCampaigns(c fiber.Ctx) error {
	title := c.Query("title")
	status := c.Query("status")
	startStr := c.Query("start_date")
	endStr := c.Query("end_date")

	var filter dto.AdminListCampaignsFilter
	if title != "" {
		filter.Title = &title
	}
	if status != "" {
		filter.Status = &status
	}
	if startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			filter.StartDate = &t
		} else {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid start_date format", "INVALID_DATE", nil)
		}
	}
	if endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			filter.EndDate = &t
		} else {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid end_date format", "INVALID_DATE", nil)
		}
	}

	resp, err := h.campaignFlow.ListCampaigns(h.createRequestContext(c, "/api/v1/admin/campaigns"), filter)
	if err != nil {
		log.Println("Admin list campaigns failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to list campaigns", "ADMIN_LIST_CAMPAIGNS_FAILED", nil)
	}

	return h.SuccessResponse(c, fiber.StatusOK, "Campaigns retrieved successfully", resp)
}

// ApproveCampaign approves a campaign
// @Summary Approve Campaign
// @Description Approve a campaign waiting for approval; moves reserved funds from frozen to locked
// @Tags Admin Campaigns
// @Accept json
// @Produce json
// @Param request body dto.AdminApproveCampaignRequest true "Approval payload"
// @Success 200 {object} dto.APIResponse{data=dto.AdminApproveCampaignResponse}
// @Failure 400 {object} dto.APIResponse "Validation error"
// @Failure 404 {object} dto.APIResponse "Campaign not found"
// @Failure 409 {object} dto.APIResponse "Invalid state"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/admin/campaigns/approve [post]
func (h *CampaignAdminHandler) ApproveCampaign(c fiber.Ctx) error {
	var req dto.AdminApproveCampaignRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}
	if err := h.validator.Struct(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", nil)
	}

	res, err := h.campaignFlow.ApproveCampaign(h.createRequestContext(c, "/api/v1/admin/campaigns/approve"), &req)
	if err != nil {
		if businessflow.IsCampaignNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Campaign not found", "CAMPAIGN_NOT_FOUND", nil)
		}
		if businessflow.IsCampaignNotWaitingForApproval(err) {
			return h.ErrorResponse(c, fiber.StatusConflict, "Invalid campaign state for approval", "INVALID_STATE", nil)
		}
		if businessflow.IsScheduleTimeTooSoon(err) {
			return h.ErrorResponse(c, fiber.StatusConflict, "Schedule time is too soon", "SCHEDULE_TIME_TOO_SOON", nil)
		}
		if businessflow.IsCustomerNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Customer not found", "CUSTOMER_NOT_FOUND", nil)
		}
		if businessflow.IsAccountTypeNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Account type not found", "ACCOUNT_TYPE_NOT_FOUND", nil)
		}
		if businessflow.IsWalletNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Wallet not found", "WALLET_NOT_FOUND", nil)
		}
		if businessflow.IsBalanceSnapshotNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Balance snapshot not found", "BALANCE_SNAPSHOT_NOT_FOUND", nil)
		}
		if businessflow.IsFreezeTransactionNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusConflict, "Freeze transaction not found", "FREEZE_TRANSACTION_NOT_FOUND", nil)
		}
		if businessflow.IsMultipleFreezeTransactionsFound(err) {
			return h.ErrorResponse(c, fiber.StatusConflict, "Multiple freeze transactions found", "MULTIPLE_FREEZE_TRANSACTIONS_FOUND", nil)
		}
		log.Println("Admin approve campaign failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to approve campaign", "ADMIN_APPROVE_CAMPAIGN_FAILED", nil)
	}
	return h.SuccessResponse(c, fiber.StatusOK, "Campaign approved successfully", res)
}

// RejectCampaign rejects a campaign and refunds frozen funds
// @Summary Reject Campaign
// @Description Reject a campaign waiting for approval; refunds reserved funds to free balance
// @Tags Admin Campaigns
// @Accept json
// @Produce json
// @Param request body dto.AdminRejectCampaignRequest true "Rejection payload"
// @Success 200 {object} dto.APIResponse{data=dto.AdminRejectCampaignResponse}
// @Failure 400 {object} dto.APIResponse "Validation error"
// @Failure 404 {object} dto.APIResponse "Campaign not found"
// @Failure 409 {object} dto.APIResponse "Invalid state"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/admin/campaigns/reject [post]
func (h *CampaignAdminHandler) RejectCampaign(c fiber.Ctx) error {
	var req dto.AdminRejectCampaignRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}
	if err := h.validator.Struct(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", nil)
	}

	res, err := h.campaignFlow.RejectCampaign(h.createRequestContext(c, "/api/v1/admin/campaigns/reject"), &req)
	if err != nil {
		if businessflow.IsCampaignNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Campaign not found", "CAMPAIGN_NOT_FOUND", nil)
		}
		if businessflow.IsCampaignNotWaitingForApproval(err) {
			return h.ErrorResponse(c, fiber.StatusConflict, "Invalid campaign state for approval", "INVALID_STATE", nil)
		}
		if businessflow.IsCustomerNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Customer not found", "CUSTOMER_NOT_FOUND", nil)
		}
		if businessflow.IsAccountTypeNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Account type not found", "ACCOUNT_TYPE_NOT_FOUND", nil)
		}
		if businessflow.IsWalletNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Wallet not found", "WALLET_NOT_FOUND", nil)
		}
		if businessflow.IsBalanceSnapshotNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Balance snapshot not found", "BALANCE_SNAPSHOT_NOT_FOUND", nil)
		}
		if businessflow.IsFreezeTransactionNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusConflict, "Freeze transaction not found", "FREEZE_TRANSACTION_NOT_FOUND", nil)
		}
		if businessflow.IsMultipleFreezeTransactionsFound(err) {
			return h.ErrorResponse(c, fiber.StatusConflict, "Multiple freeze transactions found", "MULTIPLE_FREEZE_TRANSACTIONS_FOUND", nil)
		}
		log.Println("Admin reject campaign failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to reject campaign", "ADMIN_REJECT_CAMPAIGN_FAILED", nil)
	}
	return h.SuccessResponse(c, fiber.StatusOK, "Campaign rejected successfully", res)
}

// createRequestContext creates a context with request-scoped values for observability and timeout
func (h *CampaignAdminHandler) createRequestContext(c fiber.Ctx, endpoint string) context.Context {
	return h.createRequestContextWithTimeout(c, endpoint, 30*time.Second)
}

// createRequestContextWithTimeout creates a context with custom timeout and request-scoped values
func (h *CampaignAdminHandler) createRequestContextWithTimeout(c fiber.Ctx, endpoint string, timeout time.Duration) context.Context {
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
func (h *CampaignAdminHandler) setupCustomValidations() {
	// Add custom validation rules if needed
	// Example: h.validator.RegisterValidation("custom_rule", customValidationFunc)
}
