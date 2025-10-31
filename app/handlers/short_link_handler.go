package handlers

import (
	"context"
	"log"
	"time"

	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/gofiber/fiber/v3"
)

// ShortLinkHandlerInterface defines contract for public short link visit
type ShortLinkHandlerInterface interface {
	Visit(c fiber.Ctx) error
}

type ShortLinkHandler struct {
	flow businessflow.ShortLinkVisitFlow
}

func NewShortLinkHandler(flow businessflow.ShortLinkVisitFlow) ShortLinkHandlerInterface {
	return &ShortLinkHandler{flow: flow}
}

// Visit resolves short link and redirects
// @Summary Visit Short Link
// @Tags ShortLinks
// @Produce json
// @Param uid path string true "Short link UID"
// @Success 302 {string} string "Redirect"
// @Failure 404 {object} any
// @Failure 500 {object} any
// @Router /s/{uid} [get]
func (h *ShortLinkHandler) Visit(c fiber.Ctx) error {
	uid := c.Params("uid")
	if uid == "" {
		return c.Status(fiber.StatusBadRequest).SendString("invalid short link")
	}
	ua := c.Get("User-Agent")
	ip := c.IP()

	link, err := h.flow.Visit(h.createRequestContext(c, "/s/"+uid), uid, &ua, &ip)
	if err != nil {
		if businessflow.IsShortLinkNotFound(err) {
			return c.Status(fiber.StatusNotFound).SendString("not found")
		}
		log.Println("Visit short link failed", err)
		return c.Status(fiber.StatusInternalServerError).SendString("internal error")
	}
	c.Redirect().Status(fiber.StatusFound).To(link)
	return nil
}

func (h *ShortLinkHandler) createRequestContext(c fiber.Ctx, endpoint string) context.Context {
	return h.createRequestContextWithTimeout(c, endpoint, 10*time.Second)
}

func (h *ShortLinkHandler) createRequestContextWithTimeout(c fiber.Ctx, endpoint string, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	ctx = context.WithValue(ctx, utils.RequestIDKey, c.Get("X-Request-ID"))
	ctx = context.WithValue(ctx, utils.UserAgentKey, c.Get("User-Agent"))
	ctx = context.WithValue(ctx, utils.IPAddressKey, c.IP())
	ctx = context.WithValue(ctx, utils.EndpointKey, endpoint)
	ctx = context.WithValue(ctx, utils.TimeoutKey, timeout)
	ctx = context.WithValue(ctx, utils.CancelFuncKey, cancel)
	return ctx
}
