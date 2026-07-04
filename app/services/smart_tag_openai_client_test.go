package services

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNewSmartTagOpenAIClientAllowsMissingProxy(t *testing.T) {
	t.Setenv("TEST_OPENAI_API_KEY", "test-key")

	client, err := NewSmartTagOpenAIClient(config.SmartTagOpenAIConfig{
		APIKeyEnv: "TEST_OPENAI_API_KEY",
		Model:     "test-model",
		Timeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("NewSmartTagOpenAIClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewSmartTagOpenAIClient() returned a nil client")
	}
}

func TestNewSmartTagOpenAIClientRejectsInvalidConfiguredProxy(t *testing.T) {
	invalidProxy := "://invalid"
	_, err := NewSmartTagOpenAIClient(config.SmartTagOpenAIConfig{HTTPProxy: &invalidProxy})
	if err == nil || !strings.Contains(err.Error(), "OPENAI_PROXY_INVALID") {
		t.Fatalf("NewSmartTagOpenAIClient() error = %v, want OPENAI_PROXY_INVALID", err)
	}
}

func TestSmartTagOpenAIClientReturnsResultForRequestConstructionError(t *testing.T) {
	client := &smartTagOpenAIClient{
		baseURL:    "://invalid",
		model:      "test-model",
		httpClient: http.DefaultClient,
	}

	result, err := client.CallResponsesAPI(context.Background(), map[string]any{"input": "test"})
	if err == nil {
		t.Fatal("expected request construction error")
	}
	if result == nil {
		t.Fatal("expected a non-nil audit result")
	}
	if len(result.RequestPayload) == 0 || result.RequestedAt.IsZero() || result.RespondedAt.IsZero() {
		t.Fatalf("expected populated audit result, got %+v", result)
	}
}

func TestSmartTagOpenAIClientRejectsHTTPErrorStatus(t *testing.T) {
	client := &smartTagOpenAIClient{
		baseURL: "https://api.example.test",
		model:   "test-model",
		httpClient: &http.Client{
			Timeout: time.Second,
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`)),
				}, nil
			}),
		},
	}
	result, err := client.CallResponsesAPI(context.Background(), map[string]any{"input": "test"})
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("expected provider HTTP error, got %v", err)
	}
	if result == nil || result.HTTPStatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected status in audit result, got %+v", result)
	}
	httpErr, ok := err.(*SmartTagOpenAIHTTPError)
	if !ok || !httpErr.Retryable() {
		t.Fatalf("expected retryable typed HTTP error, got %T: %v", err, err)
	}
}
