package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qiuyier/egg/internal/config"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestRouterResponsesAndMetrics(t *testing.T) {
	cfg := config.Config{
		App:  config.App{Name: "test-service"},
		HTTP: config.HTTP{Metrics: true, HealthTimeout: 20 * time.Millisecond},
	}
	router, err := New(cfg, zap.NewNop(), nil, []Check{{
		Name: "database",
		Ping: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	router.GET("/panic", func(_ *gin.Context) { panic("internal details") })

	for _, tc := range []struct {
		method string
		path   string
		status int
		code   string
	}{
		{http.MethodGet, "/api/v1/ping", http.StatusOK, "ok"},
		{http.MethodGet, "/livez", http.StatusOK, "ok"},
		{http.MethodGet, "/readyz", http.StatusServiceUnavailable, "not_ready"},
		{http.MethodGet, "/api/v1/ping/", http.StatusNotFound, "not_found"},
		{http.MethodPost, "/api/v1/ping", http.StatusMethodNotAllowed, "method_not_allowed"},
		{http.MethodGet, "/panic", http.StatusInternalServerError, "internal_error"},
	} {
		t.Run(tc.path+"_"+tc.method, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("X-Request-ID", "request-123")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			var body response
			err := json.Unmarshal(rec.Body.Bytes(), &body)
			if rec.Code != tc.status || err != nil || body.Code != tc.code {
				t.Fatalf("unexpected response: %d %s (%v)", rec.Code, rec.Body, err)
			}
			if rec.Header().Get("X-Request-ID") != "request-123" || body.RequestID != "request-123" {
				t.Fatal("request ID was not propagated")
			}
			if strings.Contains(rec.Body.String(), "internal details") {
				t.Fatal("response exposed panic details")
			}
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("X-Request-ID", "invalid id")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `egg_http_requests_total{method="GET",route="/api/v1/ping",status="200"} 1`) {
		t.Fatal("API metrics are missing")
	}
	if id := rec.Header().Get("X-Request-ID"); !validRequestID(id) || id == "invalid id" {
		t.Fatal("invalid request ID was not replaced")
	}
}
