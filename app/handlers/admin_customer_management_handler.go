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

type AdminCustomerManagementHandlerInterface interface {
	GetCustomersShares(c fiber.Ctx) error
	GetCustomerWithCampaigns(c fiber.Ctx) error
}

type AdminCustomerManagementHandler struct {
	flow      businessflow.AdminCustomerManagementFlow
	validator *validator.Validate
}

func NewAdminCustomerManagementHandler(flow businessflow.AdminCustomerManagementFlow) AdminCustomerManagementHandlerInterface {
	return &AdminCustomerManagementHandler{flow: flow, validator: validator.New()}
}

func (h *AdminCustomerManagementHandler) ErrorResponse(c fiber.Ctx, statusCode int, message, errorCode string, details any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{Success: false, Message: message, Error: dto.ErrorDetail{Code: errorCode, Details: details}})
}

func (h *AdminCustomerManagementHandler) SuccessResponse(c fiber.Ctx, statusCode int, message string, data any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{Success: true, Message: message, Data: data})
}

// GetCustomersShares returns aggregated shares per customer
// @Summary Admin Customers Shares Report
// @Tags Admin Customer Management
// @Produce json
// @Param start_date query string false "Filter created_at >= start_date (RFC3339)"
// @Param end_date query string false "Filter created_at <= end_date (RFC3339)"
// @Success 200 {object} dto.APIResponse{data=dto.AdminCustomersSharesResponse}
// @Failure 400 {object} dto.APIResponse
// @Failure 401 {object} dto.APIResponse
// @Failure 500 {object} dto.APIResponse
// @Router /api/v1/admin/customer-management/shares [get]
func (h *AdminCustomerManagementHandler) GetCustomersShares(c fiber.Ctx) error {
	var req dto.AdminCustomersSharesRequest
	if v := c.Query("start_date"); v != "" {
		req.StartDate = &v
		if _, err := time.Parse(time.RFC3339, v); err != nil {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid start_date format", "VALIDATION_ERROR", nil)
		}
	}
	if v := c.Query("end_date"); v != "" {
		req.EndDate = &v
		if _, err := time.Parse(time.RFC3339, v); err != nil {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid end_date format", "VALIDATION_ERROR", nil)
		}
	}

	ctx := h.createRequestContext(c, "/api/v1/admin/customer-management/shares")
	res, err := h.flow.GetCustomersShares(ctx, &req)
	if err != nil {
		log.Println("Admin get customers shares failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to retrieve customers shares", "GET_ADMIN_CUSTOMERS_SHARES_FAILED", nil)
	}
	return h.SuccessResponse(c, fiber.StatusOK, "Customers shares retrieved successfully", res)
}

// GetCustomerWithCampaigns returns full customer info with campaigns
// @Summary Admin Get Customer With Campaigns
// @Tags Admin Customer Management
// @Produce json
// @Param customer_id path int true "Customer ID"
// @Success 200 {object} dto.APIResponse{data=dto.AdminCustomerWithCampaignsResponse}
// @Failure 400 {object} dto.APIResponse
// @Failure 404 {object} dto.APIResponse
// @Failure 500 {object} dto.APIResponse
// @Router /api/v1/admin/customer-management/{customer_id} [get]
func (h *AdminCustomerManagementHandler) GetCustomerWithCampaigns(c fiber.Ctx) error {
	cidStr := c.Params("customer_id")
	cid, err := strconv.ParseUint(cidStr, 10, 64)
	if err != nil || cid == 0 {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid customer_id", "VALIDATION_ERROR", nil)
	}
	ctx := h.createRequestContext(c, "/api/v1/admin/customer-management/"+cidStr)
	res, err := h.flow.GetCustomerWithCampaigns(ctx, uint(cid))
	if err != nil {
		if businessflow.IsCustomerNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Customer not found", "CUSTOMER_NOT_FOUND", nil)
		}
		log.Println("Admin get customer with campaigns failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to retrieve customer details", "GET_ADMIN_CUSTOMER_WITH_CAMPAIGNS_FAILED", nil)
	}
	return h.SuccessResponse(c, fiber.StatusOK, "Customer details retrieved successfully", res)
}

func (h *AdminCustomerManagementHandler) createRequestContext(c fiber.Ctx, endpoint string) context.Context {
	return h.createRequestContextWithTimeout(c, endpoint, 30*time.Second)
}

func (h *AdminCustomerManagementHandler) createRequestContextWithTimeout(c fiber.Ctx, endpoint string, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	ctx = context.WithValue(ctx, utils.RequestIDKey, c.Get("X-Request-ID"))
	ctx = context.WithValue(ctx, utils.UserAgentKey, c.Get("User-Agent"))
	ctx = context.WithValue(ctx, utils.IPAddressKey, c.IP())
	ctx = context.WithValue(ctx, utils.EndpointKey, endpoint)
	ctx = context.WithValue(ctx, utils.TimeoutKey, timeout)
	ctx = context.WithValue(ctx, utils.CancelFuncKey, cancel)
	return ctx
}
