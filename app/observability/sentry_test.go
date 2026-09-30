package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestBuildHTTPEventCapturesRequestRoutingDetails(t *testing.T) {
	setActiveClientForTest(t)

	app := fiber.New()
	app.Use(requestid.New(requestid.Config{
		Generator: func() string { return "generated-request-id" },
	}))

	var captured *sentry.Event
	app.Get("/widgets/:widget_id", func(c fiber.Ctx) error {
		captured = buildHTTPEvent(c, fiber.StatusBadRequest, sentry.LevelWarning, "test", "bad request", nil, nil, nil)
		return c.SendStatus(fiber.StatusBadRequest)
	})

	req := httptest.NewRequest(http.MethodGet, "/widgets/widget-123?include=owner&access_token=must-not-leak", nil)
	req.Host = "api.example.test"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Request-ID", "edge-request-123")

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	require.NotNil(t, captured)

	require.Equal(t, "/widgets/:widget_id", captured.Transaction)
	require.Equal(t, "/widgets/:widget_id", captured.Tags["http.route"])
	require.Equal(t, "https://api.example.test/widgets/widget-123", captured.Request.URL)
	require.Equal(t, "access_token=%5Bredacted%5D&include=owner", captured.Request.QueryString)

	requestContext := captured.Contexts["request"]
	require.Equal(t, "edge-request-123", requestContext["id"])
	require.Equal(t, "/widgets/widget-123", requestContext["path"])
	require.Equal(t, "/widgets/:widget_id", requestContext["route"])
	require.Equal(t, true, requestContext["is_complete"])
	require.Equal(t, map[string]string{"widget_id": "widget-123"}, requestContext["path_params"])
	require.Equal(t, map[string][]string{
		"include":      {"owner"},
		"access_token": {"[redacted]"},
	}, requestContext["query_params"])

	responseContext := captured.Contexts["response"]
	require.Equal(t, "edge-request-123", responseContext["request_id"])
}

func TestRequestRouteMarksUnmatchedRequests(t *testing.T) {
	app := fiber.New()

	var route string
	app.Use(func(c fiber.Ctx) error {
		route, _ = requestRoute(c)
		return c.SendStatus(fiber.StatusNotFound)
	})

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Host = "api.example.test"
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	require.Equal(t, "<unmatched>", route)
}

func TestBuildHTTPEventMarksIncompleteRequestsUnavailable(t *testing.T) {
	setActiveClientForTest(t)

	app := fiber.New()
	ctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	t.Cleanup(func() { app.ReleaseCtx(ctx) })

	event := buildHTTPEvent(ctx, fiber.StatusRequestTimeout, sentry.LevelWarning, "test", "Request Timeout", nil, nil, nil)
	require.NotNil(t, event)
	require.Equal(t, "<unavailable>", event.Transaction)
	require.Equal(t, "<unavailable>", event.Tags["http.route"])
	require.Empty(t, event.Request.URL)
	require.Equal(t, false, event.Contexts["request"]["is_complete"])
}

func setActiveClientForTest(t *testing.T) {
	t.Helper()

	clientMu.Lock()
	previous := activeClient
	activeClient = &sentryClient{}
	clientMu.Unlock()

	t.Cleanup(func() {
		clientMu.Lock()
		activeClient = previous
		clientMu.Unlock()
	})
}
