package observability

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

type SentryConfig struct {
	DSN         string
	Environment string
	Release     string
	ServerName  string
	Timeout     time.Duration
	Capture4xx  bool
	Capture5xx  bool
}

type sentryClient struct {
	storeURL    string
	authHeader  string
	environment string
	release     string
	serverName  string
	timeout     time.Duration
	capture4xx  bool
	capture5xx  bool
	httpClient  *http.Client
	events      chan []byte
	wg          sync.WaitGroup
}

var (
	clientMu     sync.RWMutex
	activeClient *sentryClient
)

func InitSentry(cfg SentryConfig) error {
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		return nil
	}

	storeURL, authHeader, err := parseDSN(dsn)
	if err != nil {
		return err
	}

	serverName := strings.TrimSpace(cfg.ServerName)
	if serverName == "" {
		serverName, _ = os.Hostname()
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	sc := &sentryClient{
		storeURL:    storeURL,
		authHeader:  authHeader,
		environment: cfg.Environment,
		release:     cfg.Release,
		serverName:  serverName,
		timeout:     timeout,
		capture4xx:  cfg.Capture4xx,
		capture5xx:  cfg.Capture5xx,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		events: make(chan []byte, 512),
	}

	sc.wg.Add(1)
	go sc.loop()

	clientMu.Lock()
	prev := activeClient
	activeClient = sc
	clientMu.Unlock()

	if prev != nil {
		prev.shutdown(context.Background())
	}

	log.Printf("sentry transport enabled for %s", storeURL)
	return nil
}

func ShutdownSentry(ctx context.Context) {
	clientMu.Lock()
	sc := activeClient
	activeClient = nil
	clientMu.Unlock()

	if sc != nil {
		sc.shutdown(ctx)
	}
}

func HTTPStatusCaptureMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		err := c.Next()
		if err == nil {
			status := c.Response().StatusCode()
			if status >= fiber.StatusBadRequest {
				captureHTTPStatus(c, status, "fiber_response", "")
			}
		}
		return err
	}
}

func CaptureError(c fiber.Ctx, status int, err error, source string) {
	if err == nil {
		return
	}
	captureHTTPStatus(c, status, source, err.Error())
}

func CapturePanic(c fiber.Ctx, recovered any) {
	if recovered == nil {
		return
	}

	message := fmt.Sprintf("panic recovered: %v", recovered)
	extra := map[string]any{
		"panic": string(debug.Stack()),
	}

	enqueueEvent(buildEvent(c, fiber.StatusInternalServerError, "fatal", "fiber_panic", message, extra))
}

func captureHTTPStatus(c fiber.Ctx, status int, source string, message string) {
	clientMu.RLock()
	sc := activeClient
	clientMu.RUnlock()
	if sc == nil {
		return
	}

	if status >= 500 && !sc.capture5xx {
		return
	}
	if status >= 400 && status < 500 && !sc.capture4xx {
		return
	}

	if message == "" {
		message = extractResponseMessage(c, status)
	}

	extra := map[string]any{
		"request_id":    fmt.Sprintf("%v", c.Locals("requestid")),
		"response_body": truncate(string(c.Response().Body()), 4096),
		"source":        source,
	}

	enqueueEvent(buildEvent(c, status, statusLevel(status), source, message, extra))
}

func enqueueEvent(payload []byte) {
	clientMu.RLock()
	sc := activeClient
	clientMu.RUnlock()
	if sc == nil || len(payload) == 0 {
		return
	}

	select {
	case sc.events <- payload:
	default:
		log.Printf("sentry event dropped because the buffer is full")
	}
}

func buildEvent(c fiber.Ctx, status int, level, source, message string, extra map[string]any) []byte {
	clientMu.RLock()
	sc := activeClient
	clientMu.RUnlock()
	if sc == nil {
		return nil
	}

	event := map[string]any{
		"event_id":    randomEventID(),
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"platform":    "go",
		"logger":      "yamata-no-orochi",
		"level":       level,
		"server_name": sc.serverName,
		"environment": sc.environment,
		"release":     sc.release,
		"message":     truncate(message, 2048),
		"tags": map[string]string{
			"http.status_code": fmt.Sprintf("%d", status),
			"http.method":      c.Method(),
			"error.source":     source,
		},
		"request": map[string]any{
			"url":          requestURL(c),
			"method":       c.Method(),
			"headers":      filteredHeaders(c),
			"query_string": string(c.Request().URI().QueryString()),
		},
		"extra": extra,
	}

	if route := c.Route(); route != nil && route.Path != "" {
		event["transaction"] = route.Path
	}

	body, err := json.Marshal(event)
	if err != nil {
		log.Printf("failed to marshal sentry event: %v", err)
		return nil
	}
	return body
}

func (sc *sentryClient) loop() {
	defer sc.wg.Done()
	for payload := range sc.events {
		if err := sc.send(payload); err != nil {
			log.Printf("failed to send sentry event: %v", err)
		}
	}
}

func (sc *sentryClient) send(payload []byte) error {
	req, err := http.NewRequest(http.MethodPost, sc.storeURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Sentry-Auth", sc.authHeader)

	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}
	return nil
}

func (sc *sentryClient) shutdown(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		close(sc.events)
		sc.wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}

func parseDSN(dsn string) (string, string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", "", fmt.Errorf("invalid sentry DSN: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", "", fmt.Errorf("invalid sentry DSN: missing scheme or host")
	}

	projectID := strings.Trim(parsed.Path, "/")
	if projectID == "" {
		return "", "", fmt.Errorf("invalid sentry DSN: missing project ID")
	}

	publicKey := parsed.User.Username()
	if publicKey == "" {
		return "", "", fmt.Errorf("invalid sentry DSN: missing public key")
	}

	secret, _ := parsed.User.Password()
	storeURL := fmt.Sprintf("%s://%s/api/%s/store/", parsed.Scheme, parsed.Host, projectID)
	authHeader := fmt.Sprintf(
		"Sentry sentry_version=7, sentry_client=yamata-no-orochi/1.0, sentry_key=%s, sentry_secret=%s",
		publicKey,
		secret,
	)

	return storeURL, authHeader, nil
}

func requestURL(c fiber.Ctx) string {
	proto := c.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
	}
	return fmt.Sprintf("%s://%s%s", proto, c.Host(), c.OriginalURL())
}

func filteredHeaders(c fiber.Ctx) map[string]string {
	headers := map[string]string{}
	for key, values := range c.GetReqHeaders() {
		value := strings.Join(values, ", ")
		switch strings.ToLower(key) {
		case "authorization", "cookie", "x-api-key":
			headers[key] = "[redacted]"
		default:
			headers[key] = truncate(value, 512)
		}
	}
	return headers
}

func extractResponseMessage(c fiber.Ctx, status int) string {
	if body := strings.TrimSpace(string(c.Response().Body())); body != "" {
		return truncate(body, 2048)
	}
	return fmt.Sprintf("HTTP %d %s", status, http.StatusText(status))
}

func statusLevel(status int) string {
	if status >= 500 {
		return "error"
	}
	return "warning"
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func randomEventID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return strings.ReplaceAll(fmt.Sprintf("%d", time.Now().UnixNano()), "-", "")
	}
	return hex.EncodeToString(buf)
}
