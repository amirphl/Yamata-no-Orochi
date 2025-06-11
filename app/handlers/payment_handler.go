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

// PaymentHandlerInterface defines the contract for payment handlers
type PaymentHandlerInterface interface {
	ChargeWallet(c fiber.Ctx) error
	PaymentCallback(c fiber.Ctx) error
	GetPaymentHistory(c fiber.Ctx) error
}

// PaymentHandler handles payment-related HTTP requests
type PaymentHandler struct {
	paymentFlow businessflow.PaymentFlow
	validator   *validator.Validate
}

func (h *PaymentHandler) ErrorResponse(c fiber.Ctx, statusCode int, message, errorCode string, details any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: false,
		Message: message,
		Error: dto.ErrorDetail{
			Code:    errorCode,
			Details: details,
		},
	})
}

func (h *PaymentHandler) SuccessResponse(c fiber.Ctx, statusCode int, message string, data any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// NewPaymentHandler creates a new payment handler
func NewPaymentHandler(paymentFlow businessflow.PaymentFlow) *PaymentHandler {
	handler := &PaymentHandler{
		paymentFlow: paymentFlow,
		validator:   validator.New(),
	}

	// Setup custom validations
	handler.setupCustomValidations()

	return handler
}

// ChargeWallet handles the wallet charging process
// @Summary Charge Wallet
// @Description Charge a customer's wallet with the specified amount using Atipay payment gateway
// @Tags Payments
// @Accept json
// @Produce json
// @Param request body dto.ChargeWalletRequest true "Wallet charging data"
// @Success 200 {object} dto.APIResponse{data=dto.ChargeWalletResponse} "Wallet charged successfully"
// @Failure 400 {object} dto.APIResponse "Validation error or invalid request"
// @Failure 401 {object} dto.APIResponse "Unauthorized - customer not found or inactive"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/payments/charge-wallet [post]
func (h *PaymentHandler) ChargeWallet(c fiber.Ctx) error {
	var req dto.ChargeWalletRequest
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
	result, err := h.paymentFlow.ChargeWallet(h.createRequestContext(c, "/api/v1/payments/charge-wallet"), &req, metadata)
	if err != nil {
		// Handle specific business errors
		if businessflow.IsCustomerNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer not found", "CUSTOMER_NOT_FOUND", nil)
		}
		if businessflow.IsAccountInactive(err) {
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer account is inactive", "ACCOUNT_INACTIVE", nil)
		}
		if businessflow.IsWalletNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Wallet not found", "WALLET_NOT_FOUND", nil)
		}
		if businessflow.IsAmountTooLow(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Amount is too low", "AMOUNT_TOO_LOW", nil)
		}
		if businessflow.IsAmountNotMultiple(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Amount must be a multiple of the required increment", "AMOUNT_NOT_MULTIPLE", nil)
		}
		if businessflow.IsAtipayTokenEmpty(err) {
			return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to get payment token", "ATIPAY_TOKEN_ERROR", nil)
		}

		log.Println("Wallet charging failed", err)
		// Handle generic business errors
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Wallet charging failed", "WALLET_CHARGING_FAILED", nil)
	}

	// Successful wallet charging
	return h.SuccessResponse(c, fiber.StatusOK, "Wallet charged successfully", fiber.Map{
		"message": result.Message,
		"success": result.Success,
		"token":   result.Token,
	})
}

// PaymentCallback handles the callback from the payment gateway
// @Summary Payment Callback
// @Description Handles the callback from the payment gateway (Atipay)
// @Tags Payments
// @Accept json
// @Produce html
// @Param request body dto.PaymentCallbackRequest true "Callback data from Atipay"
// @Success 200 {string} string "HTML payment result page"
// @Failure 400 {object} dto.APIResponse "Invalid request"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/payments/callback [post]
func (h *PaymentHandler) PaymentCallback(c fiber.Ctx) error {
	var callbackReq dto.PaymentCallbackRequest
	if err := c.Bind().JSON(&callbackReq); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Failed to parse callback data", "CALLBACK_DATA_PARSE_ERROR", err.Error())
	}

	// Get client information
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business logic to process the callback and get HTML response
	htmlResponse, err := h.paymentFlow.PaymentCallback(h.createRequestContext(c, "/api/v1/payments/callback"), &callbackReq, metadata)
	if err != nil {
		// Handle specific business errors
		if businessflow.IsPaymentRequestNotFound(err) {
			log.Println("Payment request not found", err)
			return h.ErrorResponse(c, fiber.StatusNotFound, "Payment request not found", "PAYMENT_REQUEST_NOT_FOUND", nil)
		}
		if businessflow.IsPaymentRequestAlreadyProcessed(err) {
			log.Println("Payment already processed", err)
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Payment already processed", "PAYMENT_ALREADY_PROCESSED", nil)
		}

		log.Println("Payment callback processing failed", err)
		// Handle generic business errors
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Payment callback processing failed", "PAYMENT_CALLBACK_FAILED", nil)
	}

	// Set content type to HTML and return the rendered HTML page
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(htmlResponse)
}

// createRequestContext creates a context with request-scoped values for observability and timeout
func (h *PaymentHandler) createRequestContext(c fiber.Ctx, endpoint string) context.Context {
	return h.createRequestContextWithTimeout(c, endpoint, 30*time.Second)
}

// createRequestContextWithTimeout creates a context with custom timeout and request-scoped values
func (h *PaymentHandler) createRequestContextWithTimeout(c fiber.Ctx, endpoint string, timeout time.Duration) context.Context {
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
func (h *PaymentHandler) setupCustomValidations() {
	// Add custom validation rules if needed
	// Example: h.validator.RegisterValidation("custom_rule", customValidationFunc)
}

// GetPaymentHistory handles the retrieval of payment history for a customer
// @Summary Get Payment History
// @Description Retrieve paginated payment history for the authenticated customer
// @Tags Payments
// @Accept json
// @Produce json
// @Param page query int false "Page number (default: 1)" minimum(1)
// @Param page_size query int false "Number of items per page (default: 20, max: 100)" minimum(1) maximum(100)
// @Param start_date query string false "Start date filter (ISO 8601 format)"
// @Param end_date query string false "End date filter (ISO 8601 format)"
// @Param type query string false "Transaction type filter"
// @Param status query string false "Transaction status filter"
// @Success 200 {object} dto.APIResponse{data=dto.PaymentHistoryResponse} "Payment history retrieved successfully"
// @Failure 400 {object} dto.APIResponse "Validation error or invalid request"
// @Failure 401 {object} dto.APIResponse "Unauthorized - customer not found or inactive"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/payments/history [get]
func (h *PaymentHandler) GetPaymentHistory(c fiber.Ctx) error {
	// Get authenticated customer ID from context
	customerID, ok := c.Locals("customer_id").(uint)
	if !ok {
		return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer ID not found in context", "MISSING_CUSTOMER_ID", nil)
	}

	// Parse query parameters
	page := uint(1)
	if pageStr := c.Query("page"); pageStr != "" {
		if parsed, err := strconv.ParseUint(pageStr, 10, 32); err == nil {
			page = uint(parsed)
		}
	}

	pageSize := uint(20)
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if parsed, err := strconv.ParseUint(pageSizeStr, 10, 32); err == nil {
			pageSize = uint(parsed)
		}
	}

	// Parse date filters
	var startDate, endDate *time.Time
	if startDateStr := c.Query("start_date"); startDateStr != "" {
		if parsed, err := time.Parse(time.RFC3339, startDateStr); err == nil {
			startDate = &parsed
		}
	}
	if endDateStr := c.Query("end_date"); endDateStr != "" {
		if parsed, err := time.Parse(time.RFC3339, endDateStr); err == nil {
			endDate = &parsed
		}
	}

	// Parse type and status filters
	var transactionType, transactionStatus *string
	if typeStr := c.Query("type"); typeStr != "" {
		transactionType = &typeStr
	}
	if statusStr := c.Query("status"); statusStr != "" {
		transactionStatus = &statusStr
	}

	// Build request
	req := &dto.GetPaymentHistoryRequest{
		CustomerID: customerID,
		Page:       page,
		PageSize:   pageSize,
		StartDate:  startDate,
		EndDate:    endDate,
		Type:       transactionType,
		Status:     transactionStatus,
	}

	// Get client information
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business logic
	result, err := h.paymentFlow.GetPaymentHistory(h.createRequestContext(c, "/api/v1/payments/history"), req, metadata)
	if err != nil {
		// Handle specific business errors
		if businessflow.IsCustomerNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer not found", "CUSTOMER_NOT_FOUND", nil)
		}
		if businessflow.IsAccountInactive(err) {
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer account is inactive", "ACCOUNT_INACTIVE", nil)
		}
		if businessflow.IsWalletNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusNotFound, "Wallet not found", "WALLET_NOT_FOUND", nil)
		}

		log.Println("Payment history retrieval failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to retrieve payment history", "PAYMENT_HISTORY_RETRIEVAL_FAILED", nil)
	}

	// Return successful response
	return h.SuccessResponse(c, fiber.StatusOK, "Payment history retrieved successfully", result)
}
