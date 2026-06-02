package middleware

import (
	"log"
	"strings"

	"github.com/amirphl/Yamata-no-Orochi/app/authorization"
	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/gofiber/fiber/v3"
)

// AuthorizationMiddleware enforces fine-grained admin permissions.
type AuthorizationMiddleware struct {
	adminRepo repository.AdminRepository
}

// NewAuthorizationMiddleware constructs a new authorization middleware.
func NewAuthorizationMiddleware(adminRepo repository.AdminRepository) *AuthorizationMiddleware {
	return &AuthorizationMiddleware{adminRepo: adminRepo}
}

// AdminAuthorize checks permissions for admin routes using the central registry.
// It assumes AdminAuthenticate has already populated admin_id in context.
func (m *AuthorizationMiddleware) AdminAuthorize() fiber.Handler {
	return func(c fiber.Ctx) error {
		perm, ok := authorization.PermissionForRoute(string(c.Method()), c.Path())
		if !ok {
			log.Printf("permission_unmapped method=%s path=%s", c.Method(), c.Path())
			return c.Status(fiber.StatusForbidden).JSON(dto.APIResponse{
				Success: false,
				Message: "Permission mapping missing",
				Error:   dto.ErrorDetail{Code: "PERMISSION_NOT_MAPPED"},
			})
		}

		adminID, exists := GetAdminIDFromContext(c)
		if !exists || adminID == 0 {
			return c.Status(fiber.StatusUnauthorized).JSON(dto.APIResponse{
				Success: false,
				Message: "Admin authentication required",
				Error:   dto.ErrorDetail{Code: "ADMIN_AUTHENTICATION_REQUIRED"},
			})
		}

		admin, err := m.adminRepo.ByID(c.Context(), adminID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(dto.APIResponse{
				Success: false,
				Message: "Permission check failed",
				Error:   dto.ErrorDetail{Code: "PERMISSION_CHECK_FAILED"},
			})
		}
		if admin == nil || (admin.IsActive != nil && !*admin.IsActive) {
			return c.Status(fiber.StatusForbidden).JSON(dto.APIResponse{
				Success: false,
				Message: "Admin inactive",
				Error:   dto.ErrorDetail{Code: "ADMIN_INACTIVE"},
			})
		}

		if !authorization.HasPermission(admin, perm) {
			log.Printf("permission_denied admin_id=%d method=%s path=%s permission=%s", adminID, c.Method(), c.Path(), perm)
			return c.Status(fiber.StatusForbidden).JSON(dto.APIResponse{
				Success: false,
				Message: "Permission denied",
				Error:   dto.ErrorDetail{Code: "PERMISSION_DENIED", Details: map[string]any{"permission": perm}},
			})
		}

		return c.Next()
	}
}

// ServiceAccountAuthorize allows cron/service accounts with pre-assigned permissions.
// Service accounts should hit internal tools with an injected permission key header.
func (m *AuthorizationMiddleware) ServiceAccountAuthorize(required authorization.PermissionKey) fiber.Handler {
	return func(c fiber.Ctx) error {
		header := strings.TrimSpace(c.Get("X-Service-Permissions"))
		if header == "" {
			return c.Status(fiber.StatusForbidden).JSON(dto.APIResponse{
				Success: false,
				Message: "Permission denied",
				Error:   dto.ErrorDetail{Code: "PERMISSION_DENIED", Details: map[string]any{"permission": required}},
			})
		}
		has := false
		for _, part := range strings.Split(header, ",") {
			if strings.EqualFold(strings.TrimSpace(part), string(required)) {
				has = true
				break
			}
		}
		if !has {
			return c.Status(fiber.StatusForbidden).JSON(dto.APIResponse{
				Success: false,
				Message: "Permission denied",
				Error:   dto.ErrorDetail{Code: "PERMISSION_DENIED", Details: map[string]any{"permission": required}},
			})
		}
		return c.Next()
	}
}
