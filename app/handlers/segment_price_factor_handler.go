package handlers

import (
	"context"
	"log"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/gofiber/fiber/v3"
)

type SegmentPriceFactorHandlerInterface interface {
	ListLatest(c fiber.Ctx) error
}

type SegmentPriceFactorHandler struct {
	flow businessflow.SegmentPriceFactorFlow
}

func NewSegmentPriceFactorHandler(flow businessflow.SegmentPriceFactorFlow) SegmentPriceFactorHandlerInterface {
	return &SegmentPriceFactorHandler{flow: flow}
}

func (h *SegmentPriceFactorHandler) ErrorResponse(c fiber.Ctx, status int, message, code string, details any) error {
	return c.Status(status).JSON(dto.APIResponse{Success: false, Message: message, Error: dto.ErrorDetail{Code: code, Details: details}})
}

func (h *SegmentPriceFactorHandler) SuccessResponse(c fiber.Ctx, status int, message string, data any) error {
	return c.Status(status).JSON(dto.APIResponse{Success: true, Message: message, Data: data})
}

// ListLatest returns the latest price factor per level3 for authenticated users.
// @Summary List Segment Price Factors
// @Description List the latest price factor per level3
// @Tags Segment Price Factors
// @Produce json
// @Success 200 {object} dto.APIResponse{data=dto.ListLatestSegmentPriceFactorsResponse}
// @Failure 500 {object} dto.APIResponse "List failed"
// @Router /api/v1/segment-price-factors [get]
func (h *SegmentPriceFactorHandler) ListLatest(c fiber.Ctx) error {
	res, err := h.flow.ListLatestSegmentPriceFactors(h.createRequestContext(c, "/api/v1/segment-price-factors"))
	if err != nil {
		log.Println("List segment price factors failed:", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "List segment price factors failed", "SEGMENT_PRICE_FACTOR_LIST_FAILED", nil)
	}
	return h.SuccessResponse(c, fiber.StatusOK, "Segment price factors retrieved", res)
}

func (h *SegmentPriceFactorHandler) createRequestContext(c fiber.Ctx, endpoint string) context.Context {
	return h.createRequestContextWithTimeout(c, endpoint, 30*time.Second)
}

func (h *SegmentPriceFactorHandler) createRequestContextWithTimeout(c fiber.Ctx, endpoint string, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	ctx = context.WithValue(ctx, utils.RequestIDKey, c.Get("X-Request-ID"))
	ctx = context.WithValue(ctx, utils.UserAgentKey, c.Get("User-Agent"))
	ctx = context.WithValue(ctx, utils.IPAddressKey, c.IP())
	ctx = context.WithValue(ctx, utils.EndpointKey, endpoint)
	ctx = context.WithValue(ctx, utils.TimeoutKey, timeout)
	ctx = context.WithValue(ctx, utils.CancelFuncKey, cancel)
	return ctx
}
