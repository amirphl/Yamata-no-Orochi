package middleware

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Total HTTP requests partitioned by method, route, and status code
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed",
		},
		[]string{"method", "route", "status"},
	)

	// Request duration in seconds partitioned by method, route, and status code
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latencies in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route", "status"},
	)

	// In-flight HTTP requests
	httpInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_inflight_requests",
			Help: "Number of HTTP requests currently being served",
		},
	)
)

// Metrics returns a Fiber v3 middleware that records basic Prometheus metrics.
// Labels are kept low-cardinality by using the matched route path when available.
func Metrics() fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		httpInFlight.Inc()
		defer httpInFlight.Dec()

		// Call the next handler in the chain
		err := c.Next()

		status := c.Response().StatusCode()
		method := c.Method()
		route := c.Path()
		if r := c.Route(); r != nil && r.Path != "" {
			route = r.Path // Use route template to avoid high cardinality
		}

		labels := prometheus.Labels{
			"method": method,
			"route":  route,
			"status": intToString(status),
		}
		httpRequestsTotal.With(labels).Inc()
		httpRequestDuration.With(labels).Observe(time.Since(start).Seconds())

		return err
	}
}

func intToString(v int) string {
	// Avoid importing strconv to keep this middleware minimal
	if v == 0 {
		return "0"
	}
	// Fast path for common codes
	switch v {
	case 200:
		return "200"
	case 201:
		return "201"
	case 204:
		return "204"
	case 301:
		return "301"
	case 302:
		return "302"
	case 400:
		return "400"
	case 401:
		return "401"
	case 403:
		return "403"
	case 404:
		return "404"
	case 409:
		return "409"
	case 422:
		return "422"
	case 429:
		return "429"
	case 500:
		return "500"
	case 502:
		return "502"
	case 503:
		return "503"
	default:
		// Fallback
		return fmtInt(v)
	}
}

func fmtInt(v int) string {
	// Minimal int to string conversion without extra imports
	// This is fine for small counts like HTTP statuses
	var buf [4]byte
	i := len(buf)
	n := v
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 && i > 0 {
		i--
		buf[i] = byte('0' + (n % 10))
		n /= 10
	}
	if neg && i > 0 {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
