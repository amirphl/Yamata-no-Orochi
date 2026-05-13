package handlers

import (
	"context"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/app/middleware"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/gofiber/fiber/v3"
)

// MultimediaAdminHandlerInterface defines admin multimedia handlers.
type MultimediaAdminHandlerInterface interface {
	Download(c fiber.Ctx) error
	Preview(c fiber.Ctx) error
}

// MultimediaAdminHandler handles admin multimedia requests.
type MultimediaAdminHandler struct {
	flow businessflow.MultimediaAdminFlow
}

// NewMultimediaAdminHandler creates a new admin multimedia handler.
func NewMultimediaAdminHandler(flow businessflow.MultimediaAdminFlow) MultimediaAdminHandlerInterface {
	return &MultimediaAdminHandler{flow: flow}
}

func (h *MultimediaAdminHandler) ErrorResponse(c fiber.Ctx, statusCode int, message, errorCode string, details any) error {
	return c.Status(statusCode).JSON(dto.APIResponse{
		Success: false,
		Message: message,
		Error: dto.ErrorDetail{
			Code:    errorCode,
			Details: details,
		},
	})
}

// Download handles multimedia download for admins.
// @Summary Admin download multimedia
// @Description Download an image or video by uuid (admin access)
// @Tags Admin Multimedia
// @Security BearerAuth
// @Produce application/octet-stream
// @Param uuid path string true "Multimedia UUID"
// @Success 200 {string} string "Binary file"
// @Failure 400 {object} dto.APIResponse "Invalid request"
// @Failure 401 {object} dto.APIResponse "Unauthorized"
// @Failure 403 {object} dto.APIResponse "Forbidden"
// @Failure 404 {object} dto.APIResponse "Not found"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/admin/media/{uuid} [get]
func (h *MultimediaAdminHandler) Download(c fiber.Ctx) error {
	mediaUUID := c.Params("uuid")
	if mediaUUID == "" {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid uuid", "INVALID_UUID", nil)
	}

	filename, contentType, data, err := h.flow.DownloadMultimediaByAdmin(h.createRequestContext(c, "/api/v1/admin/media/{uuid}"), mediaUUID)
	if err != nil {
		if be, ok := err.(*businessflow.BusinessError); ok {
			switch be.Code {
			case "INVALID_UUID":
				return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid uuid", be.Code, be.Error())
			case "MULTIMEDIA_NOT_FOUND":
				return h.ErrorResponse(c, fiber.StatusNotFound, "Multimedia not found", be.Code, be.Error())
			case "INVALID_PATH":
				return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid file path", be.Code, be.Error())
			}
		}
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to download multimedia", "DOWNLOAD_FAILED", nil)
	}

	if contentType != "" {
		c.Set("Content-Type", contentType)
	}
	c.Set("Content-Disposition", "attachment; filename="+filename)
	return c.Send(data)
}

// Preview handles multimedia preview (thumbnail) for admins.
// @Summary Admin preview multimedia
// @Description Return a thumbnail image for multimedia (video -> frame image, image -> resized)
// @Tags Admin Multimedia
// @Security BearerAuth
// @Produce image/jpeg
// @Param uuid path string true "Multimedia UUID"
// @Success 200 {string} string "Thumbnail image"
// @Failure 400 {object} dto.APIResponse "Invalid request"
// @Failure 401 {object} dto.APIResponse "Unauthorized"
// @Failure 403 {object} dto.APIResponse "Forbidden"
// @Failure 404 {object} dto.APIResponse "Not found"
// @Failure 500 {object} dto.APIResponse "Internal server error"
// @Router /api/v1/admin/media/{uuid}/preview [get]
func (h *MultimediaAdminHandler) Preview(c fiber.Ctx) error {
	mediaUUID := c.Params("uuid")
	if mediaUUID == "" {
		return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid uuid", "INVALID_UUID", nil)
	}

	filename, contentType, data, err := h.flow.PreviewMultimediaByAdmin(h.createRequestContext(c, "/api/v1/admin/media/{uuid}/preview"), mediaUUID)
	if err != nil {
		if be, ok := err.(*businessflow.BusinessError); ok {
			switch be.Code {
			case "INVALID_UUID":
				return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid uuid", be.Code, be.Error())
			case "MULTIMEDIA_NOT_FOUND":
				return h.ErrorResponse(c, fiber.StatusNotFound, "Multimedia not found", be.Code, be.Error())
			case "INVALID_PATH":
				return h.ErrorResponse(c, fiber.StatusBadRequest, "Invalid file path", be.Code, be.Error())
			case "VIDEO_PREVIEW_UNAVAILABLE":
				return h.ErrorResponse(c, fiber.StatusNotImplemented, "Video preview unavailable", be.Code, be.Error())
			case "VIDEO_PREVIEW_FAILED":
				return h.ErrorResponse(c, fiber.StatusBadRequest, "Video preview failed", be.Code, be.Error())
			}
		}
		return h.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to generate preview", "PREVIEW_FAILED", nil)
	}

	if contentType != "" {
		c.Set("Content-Type", contentType)
	}
	c.Set("Content-Disposition", "inline; filename="+filename)
	return c.Send(data)
}

func (h *MultimediaAdminHandler) createRequestContext(c fiber.Ctx, endpoint string) context.Context {
	return h.createRequestContextWithTimeout(c, endpoint, 30*time.Second)
}

func (h *MultimediaAdminHandler) createRequestContextWithTimeout(c fiber.Ctx, endpoint string, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	ctx = context.WithValue(ctx, utils.RequestIDKey, c.Get("X-Request-ID"))
	ctx = context.WithValue(ctx, utils.UserAgentKey, c.Get("User-Agent"))
	ctx = context.WithValue(ctx, utils.IPAddressKey, c.IP())
	ctx = context.WithValue(ctx, utils.EndpointKey, endpoint)
	ctx = context.WithValue(ctx, utils.TimeoutKey, timeout)
	ctx = context.WithValue(ctx, utils.CancelFuncKey, cancel)
	if adminID, ok := middleware.GetAdminIDFromContext(c); ok {
		ctx = context.WithValue(ctx, utils.AdminIDKey, adminID)
	}
	return ctx
}
