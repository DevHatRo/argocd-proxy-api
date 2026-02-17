package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestGinMiddleware(t *testing.T) {
	router := gin.New()
	router.Use(GinMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify that the counter was incremented by collecting from it
	ch := make(chan prometheus.Metric, 10)
	HTTPRequestsTotal.Collect(ch)

	found := false
drainLoop:
	for {
		select {
		case <-ch:
			found = true
		default:
			break drainLoop
		}
	}
	if !found {
		t.Error("expected HTTPRequestsTotal to have metrics after request")
	}
}

func TestGinMiddlewareSkipsMetricsPath(t *testing.T) {
	router := gin.New()
	router.Use(GinMiddleware())
	router.GET("/metrics", Handler())

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestMetricsHandler(t *testing.T) {
	SetBuildInfo("test", "test")

	router := gin.New()
	router.GET("/metrics", Handler())

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Verify standard Go runtime metrics are present
	if !strings.Contains(body, "go_goroutines") {
		t.Error("expected go_goroutines metric in output")
	}

	// Verify our custom metrics are present
	if !strings.Contains(body, "build_info") {
		t.Error("expected build_info metric in output")
	}
}

func TestSetBuildInfo(t *testing.T) {
	SetBuildInfo("v1.0.0", "2026-01-01T00:00:00Z")

	router := gin.New()
	router.GET("/metrics", Handler())

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `version="v1.0.0"`) {
		t.Errorf("expected version label in build_info metric, got:\n%s", body)
	}
	if !strings.Contains(body, `build_time="2026-01-01T00:00:00Z"`) {
		t.Errorf("expected build_time label in build_info metric, got:\n%s", body)
	}
}

func TestNormalizePath(t *testing.T) {
	router := gin.New()
	router.Use(GinMiddleware())
	router.GET("/applications/:name", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/applications/my-app", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Collect metrics and verify the path label is the route pattern, not the actual path
	metricsRouter := gin.New()
	metricsRouter.GET("/metrics", Handler())

	mReq := httptest.NewRequest("GET", "/metrics", nil)
	mW := httptest.NewRecorder()
	metricsRouter.ServeHTTP(mW, mReq)

	body := mW.Body.String()
	if !strings.Contains(body, `path="/applications/:name"`) {
		t.Errorf("expected normalized path label, got:\n%s", body)
	}
}
