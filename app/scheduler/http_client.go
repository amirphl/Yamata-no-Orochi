package scheduler

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func newHTTPClientWithHTTPSProxy(timeout time.Duration, proxyURL string) (*http.Client, error) {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return newHTTPClient(timeout), nil
	}

	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(parsed)

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}, nil
}
