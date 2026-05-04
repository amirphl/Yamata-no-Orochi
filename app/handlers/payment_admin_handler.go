// Package handlers contains HTTP request handlers and presentation layer logic for the API endpoints
package handlers

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
)

// PaymentAdminHandlerInterface defines the contract for admin payment handlers.
type PaymentAdminHandlerInterface interface {
	ChargeWalletByAdmin(c fiber.Ctx) error
}

// PaymentAdminHandler handles admin payment HTTP requests.
type PaymentAdminHandler struct {
	paymentAdminFlow businessflow.PaymentAdminFlow
	validator        *validator.Validate
}

// NewPaymentAdminHandler creates a new admin payment handler.
func NewPaymentAdminHandler(paymentAdminFlow businessflow.PaymentAdminFlow) PaymentAdminHandlerInterface {
	return &PaymentAdminHandler{
		paymentAdminFlow: paymentAdminFlow,
		validator:        validator.New(),
	}
}

func (h *PaymentAdminHandler) ErrorResponse(c fiber.Ctx, statusCode int, message, errorCode string, details any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: false,
		Message: message,
		Error: dto.ErrorDetail{
			Code:    errorCode,
			Details: details,
		},
	})
}

func (h *PaymentAdminHandler) SuccessResponse(c fiber.Ctx, statusCode int, message string, data any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func (h *PaymentAdminHandler) createRequestContext(c fiber.Ctx, endpoint string) context.Context {
	return h.createRequestContextWithTimeout(c, endpoint, 30*time.Second)
}

func (h *PaymentAdminHandler) createRequestContextWithTimeout(c fiber.Ctx, endpoint string, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	ctx = context.WithValue(ctx, utils.RequestIDKey, c.Get("X-Request-ID"))
	ctx = context.WithValue(ctx, utils.UserAgentKey, c.Get("User-Agent"))
	ctx = context.WithValue(ctx, utils.IPAddressKey, c.IP())
	ctx = context.WithValue(ctx, utils.EndpointKey, endpoint)
	ctx = context.WithValue(ctx, utils.TimeoutKey, timeout)
	ctx = context.WithValue(ctx, utils.CancelFuncKey, cancel)
	return ctx
}

// ChargeWalletByAdmin directly charges a customer's wallet without payment gateway redirect.
// @Summary Charge Wallet By Admin
// @Description Admin endpoint to directly charge a customer wallet (manual card-to-card/offline payment settlement)
// @Tags Payments Admin
// @Accept json
// @Produce json
// @Param request body dto.ChargeWalletByAdminRequest true "Admin wallet charge payload"
// @Success 200 {object} dto.APIResponse{data=dto.ChargeWalletByAdminResponse} "Wallet charged successfully by admin"
// @Failure 400 {object} dto.APIResponse "Validation error or invalid request"
// @Failure 401 {object} dto.APIResponse "Unauthorized admin"
// @Failure 404 {object} dto.APIResponse "Customer or wallet not found"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/admin/payments/charge-wallet [post]
func (h *PaymentAdminHandler) ChargeWalletByAdmin(c fiber.Ctx) error {
	var req dto.ChargeWalletByAdminRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		req.IdempotencyKey = strings.TrimSpace(c.Get("Idempotency-Key"))
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		req.IdempotencyKey = strings.TrimSpace(c.Get("X-Idempotency-Key"))
	}
	if err := h.validator.Struct(&req); err != nil {
		var validationErrors []string
		for _, err := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, getValidationErrorMessage(err))
		}
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", validationErrors)
	}

	adminID, ok := c.Locals("admin_id").(uint)
	if !ok || adminID == 0 {
		return h.ErrorResponse(c, fiber.StatusUnauthorized, "Admin ID not found in context", "MISSING_ADMIN_ID", nil)
	}

	metadata := businessflow.NewClientMetadata(c.IP(), c.Get("User-Agent"))
	metadata.SetRequestID(strings.TrimSpace(c.Get("X-Request-ID")))
	result, err := h.paymentAdminFlow.ChargeWalletByAdmin(
		h.createRequestContext(c, "/api/v1/admin/payments/charge-wallet"),
		&req,
		metadata,
		adminID,
	)
	if err != nil {
		if businessflow.IsCustomerNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Customer not found", "CUSTOMER_NOT_FOUND", nil)
		}
		if businessflow.IsAccountInactive(err) {
			return h.ErrorResponse(c, fiber.StatusForbidden, "Customer account is inactive", "ACCOUNT_INACTIVE", nil)
		}
		if businessflow.IsWalletNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Wallet not found", "WALLET_NOT_FOUND", nil)
		}
		if businessflow.IsReferrerAgencyIDRequired(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Referrer agency ID is required", "REFERRER_AGENCY_ID_REQUIRED", nil)
		}
		if businessflow.IsAgencyDiscountNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Agency discount not found", "AGENCY_DISCOUNT_NOT_FOUND", nil)
		}
		if businessflow.IsAmountTooLow(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Amount is too low", "AMOUNT_TOO_LOW", nil)
		}
		if businessflow.IsAmountNotMultiple(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Amount must be a multiple of the required increment", "AMOUNT_NOT_MULTIPLE", nil)
		}
		if businessflow.IsTaxWalletNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Tax wallet not found", "TAX_WALLET_NOT_FOUND", nil)
		}
		if businessflow.IsSystemWalletNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "System wallet not found", "SYSTEM_WALLET_NOT_FOUND", nil)
		}
		if businessflow.IsBalanceSnapshotNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Balance snapshot not found", "BALANCE_SNAPSHOT_NOT_FOUND", nil)
		}
		if businessflow.IsTaxWalletBalanceSnapshotNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Tax wallet balance snapshot not found", "TAX_WALLET_BALANCE_SNAPSHOT_NOT_FOUND", nil)
		}
		if businessflow.IsSystemWalletBalanceSnapshotNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "System wallet balance snapshot not found", "SYSTEM_WALLET_BALANCE_SNAPSHOT_NOT_FOUND", nil)
		}

		log.Println("Admin wallet charging failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Wallet charging by admin failed", "WALLET_CHARGING_BY_ADMIN_FAILED", nil)
	}

	return h.SuccessResponse(c, fiber.StatusOK, "Wallet charged successfully by admin", result)
}
