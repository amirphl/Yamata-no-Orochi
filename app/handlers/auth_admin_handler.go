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

// AdminHandlerInterface defines the contract for admin auth handlers
type AdminHandlerInterface interface {
	InitCaptcha(cCtx fiber.Ctx) error
	VerifyLogin(cCtx fiber.Ctx) error
}

// AdminHandler implements AdminHandlerInterface
type AdminHandler struct {
	flow      businessflow.AdminFlow
	validator *validator.Validate
}

// ErrorResponse standard JSON error
func (h *AdminHandler) ErrorResponse(c fiber.Ctx, statusCode int, message, errorCode string, details any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: false,
		Message: message,
		Error: dto.ErrorDetail{
			Code:    errorCode,
			Details: details,
		},
	})
}

// SuccessResponse standard JSON success
func (h *AdminHandler) SuccessResponse(c fiber.Ctx, statusCode int, message string, data any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func NewAdminHandler(flow businessflow.AdminFlow) AdminHandlerInterface {
	return &AdminHandler{
		flow:      flow,
		validator: validator.New(),
	}
}

// InitCaptcha starts the admin login by returning a rotate captcha challenge
// @Summary Admin captcha init
// @Description Initialize rotate captcha for admin login (returns base64 images and challenge ID)
// @Tags Admin Authentication
// @Produce json
// @Success 200 {object} dto.APIResponse{data=dto.AdminCaptchaInitResponse} "Captcha initialized"
// @Failure 500 {object} dto.APIResponse "Failed to initialize captcha"
// @Router /api/v1/admin/auth/captcha/init [get]
func (h *AdminHandler) InitCaptcha(c fiber.Ctx) error {
	resp, err := h.flow.InitCaptcha(h.createRequestContext(c, "/api/v1/admin/auth/captcha/init"))
	if err != nil {
		log.Println("Admin captcha init failed", err)
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Admin captcha init failed", "ADMIN_CAPTCHA_INIT_FAILED", nil)
	}

	return h.SuccessResponse(c, fiber.StatusOK, "Captcha initialized", resp)
}

// VerifyLogin completes admin login by verifying captcha and credentials
// @Summary Admin login
// @Description Verify captcha and authenticate admin with username/password
// @Tags Admin Authentication
// @Accept json
// @Produce json
// @Param request body dto.AdminCaptchaVerifyRequest true "Admin login data"
// @Success 200 {object} dto.APIResponse{data=object{access_token=string,refresh_token=string,token_type=string,expires_in=int,admin=dto.AdminDTO}} "Login successful"
// @Failure 400 {object} dto.APIResponse "Invalid request or captcha"
// @Failure 401 {object} dto.APIResponse "Incorrect credentials or admin not found"
// @Failure 403 {object} dto.APIResponse "Admin inactive"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/admin/auth/login [post]
func (h *AdminHandler) VerifyLogin(c fiber.Ctx) error {
	var req dto.AdminCaptchaVerifyRequest
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
	result, err := h.flow.Verify(h.createRequestContext(c, "/api/v1/admin/auth/login"), &req, metadata)
	if err != nil {
		if businessflow.IsInvalidCaptcha(err) {
			return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid captcha", "INVALID_CAPTCHA", nil)
		}
		if businessflow.IsAdminNotFound(err) {
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Admin not found", "ADMIN_NOT_FOUND", nil)
		}
		if businessflow.IsAdminInactive(err) {
			return h.ErrorResponse(c, fiber.StatusForbidden, "Admin inactive", "ADMIN_INACTIVE", nil)
		}
		if businessflow.IsIncorrectPassword(err) {
			return h.ErrorResponse(c, fiber.StatusUnauthorized, "Incorrect password", "INCORRECT_PASSWORD", nil)
		}
		log.Println("Admin login failed", err)
		return h.ErrorResponse(c, fiber.StatusUnauthorized, "Login failed", "LOGIN_FAILED", nil)
	}

	return h.SuccessResponse(c, fiber.StatusOK, "Login successful", fiber.Map{
		"access_token":  result.Session.AccessToken,
		"refresh_token": result.Session.RefreshToken,
		"token_type":    result.Session.TokenType,
		"expires_in":    result.Session.ExpiresIn,
		"admin":         result.Admin,
	})
}

// createRequestContext mirrors other handlers for request-scoped values
func (h *AdminHandler) createRequestContext(c fiber.Ctx, endpoint string) context.Context {
	return h.createRequestContextWithTimeout(c, endpoint, 30*time.Second)
}

func (h *AdminHandler) createRequestContextWithTimeout(c fiber.Ctx, endpoint string, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	ctx = context.WithValue(ctx, utils.RequestIDKey, c.Get("X-Request-ID"))
	ctx = context.WithValue(ctx, utils.UserAgentKey, c.Get("User-Agent"))
	ctx = context.WithValue(ctx, utils.IPAddressKey, c.IP())
	ctx = context.WithValue(ctx, utils.EndpointKey, endpoint)
	ctx = context.WithValue(ctx, utils.TimeoutKey, timeout)
	ctx = context.WithValue(ctx, utils.CancelFuncKey, cancel)
	return ctx
}
