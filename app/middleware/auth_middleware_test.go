package middleware_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amirphl/Yamata-no-Orochi/app/middleware"
	"github.com/amirphl/Yamata-no-Orochi/app/services"
	"github.com/gofiber/fiber/v3"
)

// stubTokenService is a minimal in-memory stub implementing services.TokenService.
// Only the methods exercised by the middleware are given meaningful behaviour.
type stubTokenService struct {
	validateFn      func(token string) (*services.TokenClaims, error)
	validateAdminFn func(token string) (*services.AdminTokenClaims, error)
	validateBotFn   func(token string) (*services.BotTokenClaims, error)
}

func (s *stubTokenService) GenerateTokens(customerID uint) (string, string, error) {
	return "", "", nil
}
func (s *stubTokenService) ValidateToken(token string) (*services.TokenClaims, error) {
	if s.validateFn != nil {
		return s.validateFn(token)
	}
	return nil, services.ErrTokenInvalid
}
func (s *stubTokenService) RefreshToken(refreshToken string) (string, string, error) {
	return "", "", nil
}
func (s *stubTokenService) RevokeToken(token string) error        { return nil }
func (s *stubTokenService) GetTokenClaims(token string) (*services.TokenClaims, error) {
	return nil, nil
}
func (s *stubTokenService) IsTokenRevoked(token string) bool { return false }
func (s *stubTokenService) GenerateAdminTokens(adminID uint) (string, string, error) {
	return "", "", nil
}
func (s *stubTokenService) ValidateAdminToken(token string) (*services.AdminTokenClaims, error) {
	if s.validateAdminFn != nil {
		return s.validateAdminFn(token)
	}
	return nil, services.ErrTokenInvalid
}
func (s *stubTokenService) GenerateBotTokens(botID uint) (string, string, error) {
	return "", "", nil
}
func (s *stubTokenService) ValidateBotToken(token string) (*services.BotTokenClaims, error) {
	if s.validateBotFn != nil {
		return s.validateBotFn(token)
	}
	return nil, services.ErrTokenInvalid
}

// newTestApp builds a Fiber app with a single GET /test route behind the provided middleware.
func newTestApp(mw fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Get("/test", mw, func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	return app
}

func doRequest(app *fiber.App, authHeader string) (*http.Response, error) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	return app.Test(req)
}

func decodeBody(t *testing.T, r io.Reader) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return m
}

// ---------------------------------------------------------------------------
// Authenticate middleware tests
// ---------------------------------------------------------------------------

func TestAuthenticateMissingHeader(t *testing.T) {
	t.Parallel()
	mw := middleware.NewAuthMiddleware(&stubTokenService{})
	app := newTestApp(mw.Authenticate())

	resp, err := doRequest(app, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	body := decodeBody(t, resp.Body)
	errField, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error field in response")
	}
	if errField["code"] != "MISSING_AUTHORIZATION_HEADER" {
		t.Fatalf("unexpected error code: %v", errField["code"])
	}
}

func TestAuthenticateInvalidBearerFormat(t *testing.T) {
	t.Parallel()
	mw := middleware.NewAuthMiddleware(&stubTokenService{})
	app := newTestApp(mw.Authenticate())

	resp, err := doRequest(app, "Token abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	body := decodeBody(t, resp.Body)
	errField := body["error"].(map[string]any)
	if errField["code"] != "INVALID_AUTHORIZATION_FORMAT" {
		t.Fatalf("unexpected error code: %v", errField["code"])
	}
}

func TestAuthenticateInvalidToken(t *testing.T) {
	t.Parallel()
	stub := &stubTokenService{
		validateFn: func(_ string) (*services.TokenClaims, error) {
			return nil, services.ErrTokenInvalid
		},
	}
	mw := middleware.NewAuthMiddleware(stub)
	app := newTestApp(mw.Authenticate())

	resp, err := doRequest(app, "Bearer invalid.token.value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	body := decodeBody(t, resp.Body)
	errField := body["error"].(map[string]any)
	if errField["code"] != "TOKEN_INVALID" {
		t.Fatalf("unexpected error code: %v", errField["code"])
	}
}

func TestAuthenticateExpiredToken(t *testing.T) {
	t.Parallel()
	stub := &stubTokenService{
		validateFn: func(_ string) (*services.TokenClaims, error) {
			return nil, services.ErrTokenExpired
		},
	}
	mw := middleware.NewAuthMiddleware(stub)
	app := newTestApp(mw.Authenticate())

	resp, err := doRequest(app, "Bearer expired.token.here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	body := decodeBody(t, resp.Body)
	errField := body["error"].(map[string]any)
	if errField["code"] != "TOKEN_EXPIRED" {
		t.Fatalf("unexpected error code: %v", errField["code"])
	}
}

func TestAuthenticateRevokedToken(t *testing.T) {
	t.Parallel()
	stub := &stubTokenService{
		validateFn: func(_ string) (*services.TokenClaims, error) {
			return nil, services.ErrTokenRevoked
		},
	}
	mw := middleware.NewAuthMiddleware(stub)
	app := newTestApp(mw.Authenticate())

	resp, err := doRequest(app, "Bearer revoked.token.here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	body := decodeBody(t, resp.Body)
	errField := body["error"].(map[string]any)
	if errField["code"] != "TOKEN_REVOKED" {
		t.Fatalf("unexpected error code: %v", errField["code"])
	}
}

func TestAuthenticateValidToken(t *testing.T) {
	t.Parallel()
	const wantCustomerID uint = 42
	stub := &stubTokenService{
		validateFn: func(_ string) (*services.TokenClaims, error) {
			return &services.TokenClaims{CustomerID: wantCustomerID, TokenType: "access"}, nil
		},
	}
	mw := middleware.NewAuthMiddleware(stub)

	var capturedID any
	app := fiber.New(fiber.Config{})
	app.Get("/test", mw.Authenticate(), func(c fiber.Ctx) error {
		capturedID = c.Locals("customer_id")
		return c.SendString("ok")
	})

	resp, err := doRequest(app, "Bearer valid.token.here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if capturedID != wantCustomerID {
		t.Fatalf("expected customer_id=%d in context, got %v", wantCustomerID, capturedID)
	}
}

func TestAuthenticateGenericTokenError(t *testing.T) {
	t.Parallel()
	stub := &stubTokenService{
		validateFn: func(_ string) (*services.TokenClaims, error) {
			return nil, errors.New("unexpected validation error")
		},
	}
	mw := middleware.NewAuthMiddleware(stub)
	app := newTestApp(mw.Authenticate())

	resp, err := doRequest(app, "Bearer bad.token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	body := decodeBody(t, resp.Body)
	errField := body["error"].(map[string]any)
	if errField["code"] != "TOKEN_VALIDATION_FAILED" {
		t.Fatalf("unexpected error code: %v", errField["code"])
	}
}

// ---------------------------------------------------------------------------
// AdminAuthenticate middleware tests
// ---------------------------------------------------------------------------

func TestAdminAuthenticateMissingHeader(t *testing.T) {
	t.Parallel()
	mw := middleware.NewAuthMiddleware(&stubTokenService{})
	app := newTestApp(mw.AdminAuthenticate())

	resp, err := doRequest(app, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAdminAuthenticateValidToken(t *testing.T) {
	t.Parallel()
	const wantAdminID uint = 7
	stub := &stubTokenService{
		validateAdminFn: func(_ string) (*services.AdminTokenClaims, error) {
			return &services.AdminTokenClaims{AdminID: wantAdminID, TokenType: "access"}, nil
		},
	}
	mw := middleware.NewAuthMiddleware(stub)

	var capturedID any
	app := fiber.New(fiber.Config{})
	app.Get("/test", mw.AdminAuthenticate(), func(c fiber.Ctx) error {
		capturedID = c.Locals("admin_id")
		return c.SendString("ok")
	})

	resp, err := doRequest(app, "Bearer admin.valid.token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if capturedID != wantAdminID {
		t.Fatalf("expected admin_id=%d in context, got %v", wantAdminID, capturedID)
	}
}

func TestAdminAuthenticateExpiredToken(t *testing.T) {
	t.Parallel()
	stub := &stubTokenService{
		validateAdminFn: func(_ string) (*services.AdminTokenClaims, error) {
			return nil, services.ErrTokenExpired
		},
	}
	mw := middleware.NewAuthMiddleware(stub)
	app := newTestApp(mw.AdminAuthenticate())

	resp, err := doRequest(app, "Bearer expired")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	body := decodeBody(t, resp.Body)
	errField := body["error"].(map[string]any)
	if errField["code"] != "TOKEN_EXPIRED" {
		t.Fatalf("unexpected code: %v", errField["code"])
	}
}

// ---------------------------------------------------------------------------
// BotAuthenticate middleware tests
// ---------------------------------------------------------------------------

func TestBotAuthenticateMissingHeader(t *testing.T) {
	t.Parallel()
	mw := middleware.NewAuthMiddleware(&stubTokenService{})
	app := newTestApp(mw.BotAuthenticate())

	resp, err := doRequest(app, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestBotAuthenticateValidToken(t *testing.T) {
	t.Parallel()
	const wantBotID uint = 3
	stub := &stubTokenService{
		validateBotFn: func(_ string) (*services.BotTokenClaims, error) {
			return &services.BotTokenClaims{BotID: wantBotID, TokenType: "access"}, nil
		},
	}
	mw := middleware.NewAuthMiddleware(stub)

	var capturedID any
	app := fiber.New(fiber.Config{})
	app.Get("/test", mw.BotAuthenticate(), func(c fiber.Ctx) error {
		capturedID = c.Locals("bot_id")
		return c.SendString("ok")
	})

	resp, err := doRequest(app, "Bearer bot.valid.token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if capturedID != wantBotID {
		t.Fatalf("expected bot_id=%d in context, got %v", wantBotID, capturedID)
	}
}

// ---------------------------------------------------------------------------
// OptionalAuth middleware tests
// ---------------------------------------------------------------------------

func TestOptionalAuthNoHeader(t *testing.T) {
	t.Parallel()
	mw := middleware.NewAuthMiddleware(&stubTokenService{})

	var capturedID any
	app := fiber.New(fiber.Config{})
	app.Get("/test", mw.OptionalAuth(), func(c fiber.Ctx) error {
		capturedID = c.Locals("customer_id")
		return c.SendString("ok")
	})

	resp, err := doRequest(app, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 when no auth header provided, got %d", resp.StatusCode)
	}
	if capturedID != nil {
		t.Fatalf("expected no customer_id in context without auth header, got %v", capturedID)
	}
}

func TestOptionalAuthInvalidTokenContinues(t *testing.T) {
	t.Parallel()
	stub := &stubTokenService{
		validateFn: func(_ string) (*services.TokenClaims, error) {
			return nil, services.ErrTokenInvalid
		},
	}
	mw := middleware.NewAuthMiddleware(stub)
	app := newTestApp(mw.OptionalAuth())

	resp, err := doRequest(app, "Bearer bad.token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("optional auth should pass through on invalid token, got %d", resp.StatusCode)
	}
}

func TestOptionalAuthValidTokenSetsContext(t *testing.T) {
	t.Parallel()
	const wantCustomerID uint = 99
	stub := &stubTokenService{
		validateFn: func(_ string) (*services.TokenClaims, error) {
			return &services.TokenClaims{CustomerID: wantCustomerID, TokenType: "access"}, nil
		},
	}
	mw := middleware.NewAuthMiddleware(stub)

	var capturedID any
	app := fiber.New(fiber.Config{})
	app.Get("/test", mw.OptionalAuth(), func(c fiber.Ctx) error {
		capturedID = c.Locals("customer_id")
		return c.SendString("ok")
	})

	resp, err := doRequest(app, "Bearer valid.token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if capturedID != wantCustomerID {
		t.Fatalf("expected customer_id=%d in context, got %v", wantCustomerID, capturedID)
	}
}

func TestOptionalAuthInvalidBearerFormatContinues(t *testing.T) {
	t.Parallel()
	mw := middleware.NewAuthMiddleware(&stubTokenService{})
	app := newTestApp(mw.OptionalAuth())

	resp, err := doRequest(app, "Token not-bearer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("optional auth should continue on non-Bearer scheme, got %d", resp.StatusCode)
	}
}
