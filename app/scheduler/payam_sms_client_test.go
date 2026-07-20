package scheduler

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

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
