package scheduler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/config"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPayamFetchStatusRetainsRawEmptyResponse(t *testing.T) {
	t.Parallel()

	const rawResponse = "[\n]"
	client := newHTTPPayamSMSClientWithClient(config.PayamSMSConfig{}, &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(rawResponse)),
				Request:    req,
			}, nil
		}),
	})

	result, err := client.FetchStatus(context.Background(), "token", []string{"tracking-1"})
	if err != nil {
		t.Fatalf("FetchStatus returned an error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected no parsed status items, got=%d", len(result.Items))
	}
	if result.RawResponse == nil || *result.RawResponse != rawResponse {
		t.Fatalf("raw response mismatch: got=%v want=%q", result.RawResponse, rawResponse)
	}
}

func TestPayamFetchStatusRefreshesUnauthorizedTokenAndRetainsIt(t *testing.T) {
	t.Parallel()

	var statusAuthorization []string
	tokenCalls := 0
	client := newHTTPPayamSMSClientWithClient(config.PayamSMSConfig{
		TokenURL:        "https://www.payamsms.com/auth/oauth/token",
		RootAccessToken: "root-token",
	}, &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/auth/oauth/token") {
				tokenCalls++
				if got := req.Header.Get("Authorization"); got != "Basic root-token" {
					t.Fatalf("token authorization mismatch: got=%q", got)
				}
				return payamTestResponse(req, http.StatusOK, `{"access_token":"refreshed-token","expires_in":3600}`), nil
			}

			statusAuthorization = append(statusAuthorization, req.Header.Get("Authorization"))
			if len(statusAuthorization) == 1 {
				return payamTestResponse(req, http.StatusUnauthorized, "expired"), nil
			}
			return payamTestResponse(req, http.StatusOK, `[{"customerId":"tracking-1","status":"Delivered"}]`), nil
		}),
	})
	client.statusUnauthorizedRetryDelay = func(int) time.Duration { return 0 }

	result, err := client.FetchStatus(context.Background(), "expired-token", []string{"tracking-1"})
	if err != nil {
		t.Fatalf("FetchStatus returned an error after token refresh: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].TrackingID != "tracking-1" {
		t.Fatalf("unexpected status result: %+v", result.Items)
	}

	// Even though the caller still has the rejected token, the refreshed token
	// is retained by the client and used by the next status job.
	if _, err := client.FetchStatus(context.Background(), "expired-token", []string{"tracking-2"}); err != nil {
		t.Fatalf("second FetchStatus returned an error: %v", err)
	}

	if tokenCalls != 1 {
		t.Fatalf("token endpoint calls mismatch: got=%d want=1", tokenCalls)
	}
	wantAuthorization := []string{
		"Bearer expired-token",
		"Bearer refreshed-token",
		"Bearer refreshed-token",
	}
	if !reflect.DeepEqual(statusAuthorization, wantAuthorization) {
		t.Fatalf("status authorization mismatch: got=%v want=%v", statusAuthorization, wantAuthorization)
	}
}

func TestPayamFetchStatusStopsAfterThreeUnauthorizedAttempts(t *testing.T) {
	t.Parallel()

	statusCalls := 0
	tokenCalls := 0
	var retryAttempts []int
	client := newHTTPPayamSMSClientWithClient(config.PayamSMSConfig{
		TokenURL:        "https://www.payamsms.com/auth/oauth/token",
		RootAccessToken: "root-token",
	}, &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/auth/oauth/token") {
				tokenCalls++
				return payamTestResponse(req, http.StatusOK, fmt.Sprintf(`{"access_token":"refreshed-token-%d"}`, tokenCalls)), nil
			}

			statusCalls++
			return payamTestResponse(req, http.StatusUnauthorized, fmt.Sprintf("unauthorized-%d", statusCalls)), nil
		}),
	})
	client.statusUnauthorizedRetryDelay = func(attempt int) time.Duration {
		retryAttempts = append(retryAttempts, attempt)
		return 0
	}

	result, err := client.FetchStatus(context.Background(), "expired-token", []string{"tracking-1"})
	if err == nil {
		t.Fatal("FetchStatus unexpectedly succeeded")
	}
	if !isPayamUnauthorizedError(err) {
		t.Fatalf("expected an unauthorized error, got=%v", err)
	}
	if statusCalls != payamStatusUnauthorizedMaxAttempts {
		t.Fatalf("status calls mismatch: got=%d want=%d", statusCalls, payamStatusUnauthorizedMaxAttempts)
	}
	if tokenCalls != payamStatusUnauthorizedMaxAttempts-1 {
		t.Fatalf("token calls mismatch: got=%d want=%d", tokenCalls, payamStatusUnauthorizedMaxAttempts-1)
	}
	if !reflect.DeepEqual(retryAttempts, []int{0, 1}) {
		t.Fatalf("retry attempts mismatch: got=%v want=[0 1]", retryAttempts)
	}
	if result.RawResponse == nil || *result.RawResponse != "unauthorized-3" {
		t.Fatalf("final raw response mismatch: got=%v", result.RawResponse)
	}
}

func payamTestResponse(req *http.Request, statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}
