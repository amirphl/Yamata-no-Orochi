// Package handlers contains HTTP request handlers and presentation layer logic for the API endpoints
package handlers

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/app/middleware"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
)

// CampaignAdminHandlerInterface defines the contract for campaign admin handlers
type CampaignAdminHandlerInterface interface {
	ListCampaigns(c fiber.Ctx) error
	GetCampaign(c fiber.Ctx) error
	ApproveCampaign(c fiber.Ctx) error
	RejectCampaign(c fiber.Ctx) error
	CancelCampaign(c fiber.Ctx) error
	RemoveAudienceSpec(c fiber.Ctx) error
	RescheduleCampaign(c fiber.Ctx) error
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
// @Param status query string false "Filter by status (initiated|in_progress|waiting_for_approval|approved|rejected|expired)"
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
		if businessflow.IsStartDateAfterEndDate(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "End date must be after start date", "INVALID_DATE_RANGE", nil)
		}
		log.Println("Admin list campaigns failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to list campaigns", "ADMIN_LIST_CAMPAIGNS_FAILED", nil)
	}

	return h.SuccessResponse(c, fiber.StatusOK, "Campaigns retrieved successfully", resp)
}

// GetCampaign returns a single campaign by ID
// @Summary Admin Get Campaign
// @Tags Admin Campaigns
// @Produce json
// @Param id path string true "Campaign ID"
// @Success 200 {object} dto.APIResponse{data=dto.AdminGetCampaignResponse}
// @Failure 404 {object} dto.APIResponse
// @Failure 500 {object} dto.APIResponse
// @Router /api/v1/admin/campaigns/{id} [get]
func (h *CampaignAdminHandler) GetCampaign(c fiber.Ctx) error {
	id := c.Params("id")
	idUint, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid campaign ID", "INVALID_CAMPAIGN_ID", nil)
	}
	resp, err := h.campaignFlow.GetCampaign(h.createRequestContext(c, "/api/v1/admin/campaigns/"+id), uint(idUint))
	if err != nil {
		if businessflow.IsCampaignNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Campaign not found", "CAMPAIGN_NOT_FOUND", nil)
		}
		log.Println("Admin get campaign failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to get campaign", "ADMIN_GET_CAMPAIGN_FAILED", nil)
	}
	return h.SuccessResponse(c, fiber.StatusOK, "Campaign retrieved successfully", resp)
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
		if businessflow.IsInsufficientFunds(err) {
			return h.ErrorResponse(c, fiber.StatusConflict, "Insufficient funds", "INSUFFICIENT_FUNDS", nil)
		}
		if businessflow.IsAccountInactive(err) {
			return h.ErrorResponse(c, fiber.StatusForbidden, "Account is inactive", "ACCOUNT_INACTIVE", nil)
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
		if businessflow.IsInsufficientFunds(err) {
			return h.ErrorResponse(c, fiber.StatusConflict, "Insufficient funds", "INSUFFICIENT_FUNDS", nil)
		}
		if businessflow.IsAccountInactive(err) {
			return h.ErrorResponse(c, fiber.StatusForbidden, "Account is inactive", "ACCOUNT_INACTIVE", nil)
		}
		log.Println("Admin reject campaign failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to reject campaign", "ADMIN_REJECT_CAMPAIGN_FAILED", nil)
	}
	return h.SuccessResponse(c, fiber.StatusOK, "Campaign rejected successfully", res)
}

// RescheduleCampaign updates the scheduled time for a campaign (admin-only).
// @Summary Reschedule Campaign
// @Description Admin reschedules an eligible campaign; schedule_at should be provided in Tehran time.
// @Tags Admin Campaigns
// @Accept json
// @Produce json
// @Param request body dto.AdminRescheduleCampaignRequest true "Reschedule payload"
// @Success 200 {object} dto.APIResponse{data=dto.AdminRescheduleCampaignResponse}
// @Failure 400 {object} dto.APIResponse "Validation error"
// @Failure 404 {object} dto.APIResponse "Campaign not found"
// @Failure 409 {object} dto.APIResponse "Invalid state"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/admin/campaigns/reschedule [post]
func (h *CampaignAdminHandler) RescheduleCampaign(c fiber.Ctx) error {
	var req dto.AdminRescheduleCampaignRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}
	if err := h.validator.Struct(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", nil)
	}

	res, err := h.campaignFlow.RescheduleCampaign(h.createRequestContext(c, "/api/v1/admin/campaigns/reschedule"), &req)
	if err != nil {
		if businessflow.IsCampaignNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Campaign not found", "CAMPAIGN_NOT_FOUND", nil)
		}
		if businessflow.IsCampaignRescheduleNotAllowed(err) {
			return h.ErrorResponse(c, fiber.StatusConflict, "Campaign cannot be rescheduled in its current status", "INVALID_STATE", nil)
		}
		if businessflow.IsScheduleTimeTooSoon(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Schedule time must be at least 15 minutes in the future", "SCHEDULE_TIME_TOO_SOON", nil)
		}
		log.Println("Admin reschedule campaign failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to reschedule campaign", "ADMIN_RESCHEDULE_CAMPAIGN_FAILED", nil)
	}

	return h.SuccessResponse(c, fiber.StatusOK, "Campaign rescheduled successfully", res)
}

// CancelCampaign cancels an approved campaign and refunds consumed budget.
// @Summary Cancel Campaign
// @Description Cancel an approved campaign by admin and refund consumed budget to customer
// @Tags Admin Campaigns
// @Accept json
// @Produce json
// @Param request body dto.AdminCancelCampaignRequest true "Cancellation payload"
// @Success 200 {object} dto.APIResponse{data=dto.AdminCancelCampaignResponse}
// @Failure 400 {object} dto.APIResponse "Validation error"
// @Failure 404 {object} dto.APIResponse "Campaign not found"
// @Failure 409 {object} dto.APIResponse "Invalid state"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/admin/campaigns/cancel [post]
func (h *CampaignAdminHandler) CancelCampaign(c fiber.Ctx) error {
	var req dto.AdminCancelCampaignRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}
	if err := h.validator.Struct(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", nil)
	}

	res, err := h.campaignFlow.CancelCampaign(h.createRequestContext(c, "/api/v1/admin/campaigns/cancel"), &req)
	if err != nil {
		if businessflow.IsCampaignNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Campaign not found", "CAMPAIGN_NOT_FOUND", nil)
		}
		if businessflow.IsCampaignNotApproved(err) {
			return h.ErrorResponse(c, fiber.StatusConflict, "Invalid campaign state for cancel", "INVALID_STATE", nil)
		}
		if businessflow.IsCustomerNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Customer not found", "CUSTOMER_NOT_FOUND", nil)
		}
		if businessflow.IsWalletNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Wallet not found", "WALLET_NOT_FOUND", nil)
		}
		if businessflow.IsBalanceSnapshotNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Balance snapshot not found", "BALANCE_SNAPSHOT_NOT_FOUND", nil)
		}
		if businessflow.IsCampaignDebitTransactionNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusConflict, "Campaign debit transaction not found", "CAMPAIGN_DEBIT_TRANSACTION_NOT_FOUND", nil)
		}
		if businessflow.IsMultipleCampaignDebitTransactionsFound(err) {
			return h.ErrorResponse(c, fiber.StatusConflict, "Multiple campaign debit transactions found", "MULTIPLE_CAMPAIGN_DEBIT_TRANSACTIONS_FOUND", nil)
		}
		if businessflow.IsInsufficientFunds(err) {
			return h.ErrorResponse(c, fiber.StatusConflict, "Insufficient funds", "INSUFFICIENT_FUNDS", nil)
		}
		if businessflow.IsAccountInactive(err) {
			return h.ErrorResponse(c, fiber.StatusForbidden, "Account is inactive", "ACCOUNT_INACTIVE", nil)
		}
		log.Println("Admin cancel campaign failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to cancel campaign", "ADMIN_CANCEL_CAMPAIGN_FAILED", nil)
	}
	return h.SuccessResponse(c, fiber.StatusOK, "Campaign cancelled successfully", res)
}

// RemoveAudienceSpec removes audience spec for a platform from both file and cache.
// @Summary Remove Audience Spec
// @Description Remove audience spec for a platform from storage and cache (default platform is sms)
// @Tags Admin Campaigns
// @Produce json
// @Param platform query string false "Platform (default: sms)"
// @Success 200 {object} dto.APIResponse{data=dto.AdminRemoveAudienceSpecResponse}
// @Failure 400 {object} dto.APIResponse "Validation error"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/admin/campaigns/audience-spec [delete]
func (h *CampaignAdminHandler) RemoveAudienceSpec(c fiber.Ctx) error {
	var platform *string
	platformRaw := c.Query("platform")
	if platformRaw != "" {
		platform = &platformRaw
	}

	res, err := h.campaignFlow.RemoveAudienceSpec(h.createRequestContext(c, "/api/v1/admin/campaigns/audience-spec"), platform)
	if err != nil {
		if businessflow.IsAudienceSpecPlatformInvalid(err) || businessflow.IsAudienceSpecPlatformRequired(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid platform", "INVALID_PLATFORM", nil)
		}
		if businessflow.IsCacheNotAvailable(err) {
			return h.ErrorResponse(c, fiber.StatusServiceUnavailable, "Cache is not available", "CACHE_NOT_AVAILABLE", nil)
		}
		var be *businessflow.BusinessError
		if errors.As(err, &be) {
			switch be.Code {
			case "ADMIN_REMOVE_AUDIENCE_SPEC_LOCK_BUSY":
				return h.ErrorResponse(c, fiber.StatusConflict, "Another worker is updating audience spec", "AUDIENCE_SPEC_LOCK_BUSY", nil)
			}
		}
		log.Println("Admin remove audience spec failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to remove audience spec", "ADMIN_REMOVE_AUDIENCE_SPEC_FAILED", nil)
	}
	return h.SuccessResponse(c, fiber.StatusOK, "Audience spec removed successfully", res)
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
	if adminID, ok := middleware.GetAdminIDFromContext(c); ok {
		ctx = context.WithValue(ctx, utils.AdminIDKey, adminID)
	}

	return ctx
}

// setupCustomValidations sets up custom validation rules
func (h *CampaignAdminHandler) setupCustomValidations() {
	// Add custom validation rules if needed
	// Example: h.validator.RegisterValidation("custom_rule", customValidationFunc)
}
