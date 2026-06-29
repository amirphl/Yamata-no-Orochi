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

type BundleHandlerInterface interface {
	Create(c fiber.Ctx) error
	Update(c fiber.Ctx) error
	Get(c fiber.Ctx) error
	List(c fiber.Ctx) error
}

type BundleHandler struct {
	flow      businessflow.BundleFlow
	validator *validator.Validate
}

func NewBundleHandler(flow businessflow.BundleFlow) *BundleHandler {
	return &BundleHandler{
		flow:      flow,
		validator: validator.New(),
	}
}

func (h *BundleHandler) ErrorResponse(c fiber.Ctx, statusCode int, message, errorCode string, details any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: false,
		Message: message,
		Error: dto.ErrorDetail{
			Code:    errorCode,
			Details: details,
		},
	})
}

func (h *BundleHandler) SuccessResponse(c fiber.Ctx, statusCode int, message string, data any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// Create creates a bundle for the authenticated customer.
// @Summary Create bundle
// @Description Create a new bundle for the authenticated customer
// @Tags Bundles
// @Accept json
// @Produce json
// @Param request body dto.CreateBundleRequest true "Bundle payload"
// @Success 201 {object} dto.APIResponse{data=dto.CreateBundleResponse} "Created"
// @Failure 400 {object} dto.APIResponse "Validation error"
// @Failure 401 {object} dto.APIResponse "Unauthorized"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/bundles [post]
func (h *BundleHandler) Create(c fiber.Ctx) error {
	var req dto.CreateBundleRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	customerID, ok := c.Locals("customer_id").(uint)
	if !ok {
		return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer ID not found in context", "MISSING_CUSTOMER_ID", nil)
	}
	req.CustomerID = customerID

	if err := h.validator.Struct(&req); err != nil {
		var validationErrors []string
		for _, e := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, getValidationErrorMessage(e))
		}
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", validationErrors)
	}

	metadata := businessflow.NewClientMetadata(c.IP(), c.Get("User-Agent"))
	ctx, cancel := h.createRequestContextWithTimeout(c, "/api/v1/bundles", 30*time.Second)
	defer cancel()

	res, err := h.flow.CreateBundle(ctx, &req, metadata)
	if err != nil {
		log.Println("Create bundle failed", err)
		return h.handleBundleFlowError(c, err, fiber.StatusInternalServerError, "Failed to create bundle", "CREATE_BUNDLE_FAILED")
	}

	return h.SuccessResponse(c, fiber.StatusCreated, "Bundle created successfully", res)
}

// Update updates a bundle for the authenticated customer.
// @Summary Update bundle
// @Description Update an existing bundle for the authenticated customer
// @Tags Bundles
// @Accept json
// @Produce json
// @Param id path int true "Bundle ID"
// @Param request body dto.UpdateBundleRequest true "Bundle payload"
// @Success 200 {object} dto.APIResponse{data=dto.UpdateBundleResponse} "Updated"
// @Failure 400 {object} dto.APIResponse "Validation error"
// @Failure 401 {object} dto.APIResponse "Unauthorized"
// @Failure 403 {object} dto.APIResponse "Forbidden"
// @Failure 404 {object} dto.APIResponse "Not found"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/bundles/{id} [put]
func (h *BundleHandler) Update(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid bundle ID", "INVALID_BUNDLE_ID", nil)
	}

	var req dto.UpdateBundleRequest
	if err := c.Bind().JSON(&req); err != nil {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	customerID, ok := c.Locals("customer_id").(uint)
	if !ok {
		return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer ID not found in context", "MISSING_CUSTOMER_ID", nil)
	}
	req.CustomerID = customerID
	req.ID = uint(id)

	if err := h.validator.Struct(&req); err != nil {
		var validationErrors []string
		for _, e := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, getValidationErrorMessage(e))
		}
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Validation failed", "VALIDATION_ERROR", validationErrors)
	}

	metadata := businessflow.NewClientMetadata(c.IP(), c.Get("User-Agent"))
	ctx, cancel := h.createRequestContextWithTimeout(c, "/api/v1/bundles/:id", 30*time.Second)
	defer cancel()

	res, err := h.flow.UpdateBundle(ctx, &req, metadata)
	if err != nil {
		log.Println("Update bundle failed", err)
		return h.handleBundleFlowError(c, err, fiber.StatusInternalServerError, "Failed to update bundle", "UPDATE_BUNDLE_FAILED")
	}

	return h.SuccessResponse(c, fiber.StatusOK, "Bundle updated successfully", res)
}

// Get retrieves one bundle for the authenticated customer.
// @Summary Get bundle
// @Description Get a single bundle by ID for the authenticated customer
// @Tags Bundles
// @Produce json
// @Param id path int true "Bundle ID"
// @Success 200 {object} dto.APIResponse{data=dto.GetBundleResponse} "Retrieved"
// @Failure 401 {object} dto.APIResponse "Unauthorized"
// @Failure 403 {object} dto.APIResponse "Forbidden"
// @Failure 404 {object} dto.APIResponse "Not found"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/bundles/{id} [get]
func (h *BundleHandler) Get(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid bundle ID", "INVALID_BUNDLE_ID", nil)
	}

	customerID, ok := c.Locals("customer_id").(uint)
	if !ok {
		return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer ID not found in context", "MISSING_CUSTOMER_ID", nil)
	}

	req := dto.GetBundleRequest{
		CustomerID: customerID,
		ID:         uint(id),
	}

	metadata := businessflow.NewClientMetadata(c.IP(), c.Get("User-Agent"))
	ctx, cancel := h.createRequestContextWithTimeout(c, "/api/v1/bundles/:id", 30*time.Second)
	defer cancel()

	res, err := h.flow.GetBundle(ctx, &req, metadata)
	if err != nil {
		log.Println("Get bundle failed", err)
		return h.handleBundleFlowError(c, err, fiber.StatusInternalServerError, "Failed to get bundle", "GET_BUNDLE_FAILED")
	}

	return h.SuccessResponse(c, fiber.StatusOK, "Bundle retrieved successfully", res)
}

// List lists bundles for the authenticated customer.
// @Summary List bundles
// @Description List bundles for the authenticated customer with optional filters and pagination
// @Tags Bundles
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(10)
// @Param title query string false "Title filter"
// @Param target_customer_name query string false "Target customer name filter"
// @Success 200 {object} dto.APIResponse{data=dto.ListBundlesResponse} "Retrieved"
// @Failure 401 {object} dto.APIResponse "Unauthorized"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/bundles [get]
func (h *BundleHandler) List(c fiber.Ctx) error {
	customerID, ok := c.Locals("customer_id").(uint)
	if !ok {
		return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer ID not found in context", "MISSING_CUSTOMER_ID", nil)
	}

	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		parsed, err := strconv.Atoi(pageStr)
		if err != nil || parsed <= 0 {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid page", "INVALID_PAGE", nil)
		}
		page = parsed
	}

	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed <= 0 {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid limit", "INVALID_LIMIT", nil)
		}
		limit = parsed
	}

	var filter *dto.ListBundlesFilter
	title := c.Query("title")
	targetCustomerName := c.Query("target_customer_name")
	if title != "" || targetCustomerName != "" {
		filter = &dto.ListBundlesFilter{}
		if title != "" {
			filter.Title = &title
		}
		if targetCustomerName != "" {
			filter.TargetCustomerName = &targetCustomerName
		}
	}

	req := dto.ListBundlesRequest{
		CustomerID: customerID,
		Page:       page,
		Limit:      limit,
		Filter:     filter,
	}

	metadata := businessflow.NewClientMetadata(c.IP(), c.Get("User-Agent"))
	ctx, cancel := h.createRequestContextWithTimeout(c, "/api/v1/bundles", 30*time.Second)
	defer cancel()

	res, err := h.flow.ListBundles(ctx, &req, metadata)
	if err != nil {
		log.Println("List bundles failed", err)
		return h.handleBundleFlowError(c, err, fiber.StatusInternalServerError, "Failed to list bundles", "LIST_BUNDLES_FAILED")
	}

	return h.SuccessResponse(c, fiber.StatusOK, "Bundles retrieved successfully", fiber.Map{
		"message":    res.Message,
		"items":      res.Items,
		"pagination": res.Pagination,
	})
}

func (h *BundleHandler) handleBundleFlowError(c fiber.Ctx, err error, defaultStatus int, defaultMessage, defaultCode string) error {
	if be, ok := err.(*businessflow.BusinessError); ok {
		switch be.Code {
		case "CREATE_BUNDLE_VALIDATION_FAILED", "UPDATE_BUNDLE_VALIDATION_FAILED":
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Bundle validation failed", be.Code, be.Error())
		case "BUNDLE_NOT_FOUND":
			return h.ErrorResponse(c, fiber.StatusNotFound, "Bundle not found", be.Code, nil)
		case "BUNDLE_ACCESS_DENIED":
			return h.ErrorResponse(c, fiber.StatusForbidden, "Bundle access denied", be.Code, nil)
		case "GET_BUNDLE_FAILED", "LIST_BUNDLES_FAILED", "CREATE_BUNDLE_FAILED", "UPDATE_BUNDLE_FAILED":
			return h.ErrorResponse(c, defaultStatus, defaultMessage, be.Code, nil)
		case "MISSING_CUSTOMER_ID":
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer ID not found", be.Code, nil)
		case "CUSTOMER_NOT_FOUND":
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Customer not found", be.Code, nil)
		}
	}

	return h.ErrorResponse(c, defaultStatus, defaultMessage, defaultCode, nil)
}

func (h *BundleHandler) createRequestContextWithTimeout(c fiber.Ctx, endpoint string, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	ctx = context.WithValue(ctx, utils.RequestIDKey, c.Get("X-Request-ID"))
	ctx = context.WithValue(ctx, utils.UserAgentKey, c.Get("User-Agent"))
	ctx = context.WithValue(ctx, utils.IPAddressKey, c.IP())
	ctx = context.WithValue(ctx, utils.EndpointKey, endpoint)
	ctx = context.WithValue(ctx, utils.TimeoutKey, timeout)
	ctx = context.WithValue(ctx, utils.CancelFuncKey, cancel)
	return ctx, cancel
}
