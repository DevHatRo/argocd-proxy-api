package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HTTP metrics
var (
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Number of HTTP requests currently being served.",
		},
	)
)

// ArgoCD upstream API metrics
var (
	ArgocdAPIRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "argocd_api_requests_total",
			Help: "Total number of requests made to the ArgoCD API.",
		},
		[]string{"endpoint", "status"},
	)

	ArgocdAPIRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "argocd_api_request_duration_seconds",
			Help:    "Duration of requests to the ArgoCD API in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint"},
	)
)

// Token metrics
var (
	TokenRefreshTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "token_refresh_total",
			Help: "Total number of token refresh attempts.",
		},
		[]string{"result"},
	)

	TokenRefreshDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "token_refresh_duration_seconds",
			Help:    "Duration of token refresh operations in seconds.",
			Buckets: prometheus.DefBuckets,
		},
	)
)

// Cache metrics
var (
	CacheHitsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total number of cache hits.",
		},
		[]string{"cache"},
	)

	CacheMissesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total number of cache misses.",
		},
		[]string{"cache"},
	)
)

// Build info metric
var BuildInfo = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "build_info",
		Help: "Build information for the argocd-proxy.",
	},
	[]string{"version", "build_time"},
)

// SetBuildInfo sets the build info gauge to 1 with the given labels.
func SetBuildInfo(version, buildTime string) {
	BuildInfo.WithLabelValues(version, buildTime).Set(1)
}

// normalizePath collapses path parameters to reduce cardinality.
func normalizePath(c *gin.Context) string {
	route := c.FullPath()
	if route != "" {
		return route
	}
	return "unknown"
}

// GinMiddleware returns a Gin middleware that records Prometheus metrics.
func GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}

		HTTPRequestsInFlight.Inc()
		start := time.Now()

		c.Next()

		HTTPRequestsInFlight.Dec()

		status := strconv.Itoa(c.Writer.Status())
		path := normalizePath(c)
		method := c.Request.Method
		duration := time.Since(start).Seconds()

		HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
		HTTPRequestDuration.WithLabelValues(method, path, status).Observe(duration)
	}
}

// Handler returns the Prometheus metrics HTTP handler for use with Gin.
func Handler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
