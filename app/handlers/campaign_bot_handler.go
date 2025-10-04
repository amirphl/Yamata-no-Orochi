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

// CampaignBotHandlerInterface defines the contract for bot campaign handlers
type CampaignBotHandlerInterface interface {
	UpdateAudienceSpec(c fiber.Ctx) error
	ListReadyCampaigns(c fiber.Ctx) error
	MoveCampaignToExecuted(c fiber.Ctx) error
}

// CampaignBotHandler handles bot campaign-related HTTP requests
type CampaignBotHandler struct {
	campaignFlow businessflow.BotCampaignFlow
	validator    *validator.Validate
}

func NewCampaignBotHandler(flow businessflow.BotCampaignFlow) CampaignBotHandlerInterface {
	h := &CampaignBotHandler{
		campaignFlow: flow,
		validator:    validator.New(),
	}
	return h
}

func (h *CampaignBotHandler) ErrorResponse(c fiber.Ctx, statusCode int, message, errorCode string, details any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: false,
		Message: message,
		Error: dto.ErrorDetail{
			Code:    errorCode,
			Details: details,
		},
	})
}

func (h *CampaignBotHandler) SuccessResponse(c fiber.Ctx, statusCode int, message string, data any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// UpdateAudienceSpec updates audience spec (bot)
// @Summary Bot Update Audience Spec
// @Tags Bot Campaigns
// @Accept json
// @Produce json
// @Param request body dto.BotUpdateAudienceSpecRequest true "Audience spec update"
// @Success 201 {object} dto.APIResponse{data=dto.BotUpdateAudienceSpecResponse}
// @Failure 400 {object} dto.APIResponse
// @Router /api/v1/bot/campaigns/audience-spec [post]
func (h *CampaignBotHandler) UpdateAudienceSpec(c fiber.Ctx) error {
	var req dto.BotUpdateAudienceSpecRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}
	if err := h.validator.Struct(&req); err != nil {
		var validationErrors []string
		for _, err := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, getValidationErrorMessage(err))
		}
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", validationErrors)
	}
	metadata := businessflow.NewClientMetadata(c.IP(), c.Get("User-Agent"))
	_ = metadata
	res, err := h.campaignFlow.UpdateAudienceSpec(h.createRequestContext(c, "/api/v1/bot/campaigns/audience-spec"), &req)
	if err != nil {
		log.Println("Update audience spec failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to update audience spec", "UPDATE_AUDIENCE_SPEC_FAILED", nil)
	}
	return h.SuccessResponse(c, fiber.StatusCreated, "Audience spec updated", res)
}

// ListReadyCampaigns lists ready campaigns and marks them running
// @Summary Bot List Ready Campaigns
// @Tags Bot Campaigns
// @Produce json
// @Success 200 {object} dto.APIResponse{data=dto.BotListCampaignsResponse}
// @Router /api/v1/bot/campaigns/ready [get]
func (h *CampaignBotHandler) ListReadyCampaigns(c fiber.Ctx) error {
	res, err := h.campaignFlow.ListReadyCampaigns(h.createRequestContext(c, "/api/v1/bot/campaigns/ready"))
	if err != nil {
		log.Println("List ready campaigns failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to list ready campaigns", "LIST_READY_CAMPAIGNS_FAILED", nil)
	}
	return h.SuccessResponse(c, fiber.StatusOK, "Ready campaigns retrieved", res)
}

// MoveCampaignToExecuted updates status to executed
// @Summary Bot Move Campaign to Executed
// @Tags Bot Campaigns
// @Produce json
// @Param id path int true "Campaign ID"
// @Success 200 {object} dto.APIResponse
// @Router /api/v1/bot/campaigns/{id}/executed [post]
func (h *CampaignBotHandler) MoveCampaignToExecuted(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid campaign id", "INVALID_CAMPAIGN_ID", nil)
	}
	metadata := businessflow.NewClientMetadata(c.IP(), c.Get("User-Agent"))
	_ = metadata
	if err := h.campaignFlow.MoveCampaignToExecuted(h.createRequestContext(c, "/api/v1/bot/campaigns/"+idStr+"/executed"), uint(id)); err != nil {
		log.Println("Move to executed failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to move campaign to executed", "MOVE_TO_EXECUTED_FAILED", nil)
	}
	return h.SuccessResponse(c, fiber.StatusOK, "Campaign moved to executed", fiber.Map{"ok": true})
}

func (h *CampaignBotHandler) createRequestContext(c fiber.Ctx, endpoint string) context.Context {
	return h.createRequestContextWithTimeout(c, endpoint, 30*time.Second)
}

func (h *CampaignBotHandler) createRequestContextWithTimeout(c fiber.Ctx, endpoint string, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	ctx = context.WithValue(ctx, utils.RequestIDKey, c.Get("X-Request-ID"))
	ctx = context.WithValue(ctx, utils.UserAgentKey, c.Get("User-Agent"))
	ctx = context.WithValue(ctx, utils.IPAddressKey, c.IP())
	ctx = context.WithValue(ctx, utils.EndpointKey, endpoint)
	ctx = context.WithValue(ctx, utils.TimeoutKey, timeout)
	ctx = context.WithValue(ctx, utils.CancelFuncKey, cancel)
	return ctx
}
