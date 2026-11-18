// Package httpapi owns the Gin router, HTTP middleware, and API response format.
package httpapi

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/qiuyier/egg/internal/config"
	"github.com/qiuyier/egg/internal/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const V1Prefix = "/api/v1"

type Check struct {
	Name string
	Ping func(context.Context) error
}

type response struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	TraceID   string `json:"trace_id,omitempty"`
	Data      any    `json:"data,omitempty"`
}

func New(cfg config.Config, log *zap.Logger, provider trace.TracerProvider, checks []Check, maxBodyBytesByRoute map[string]int64) (*gin.Engine, error) {
	if err := cfg.CORS.Validate(); err != nil {
		return nil, err
	}
	if cfg.HTTP.MaxBodyBytes < 0 {
		return nil, fmt.Errorf("http.max_body_bytes must be non-negative")
	}
	for route, limit := range maxBodyBytesByRoute {
		if !strings.HasPrefix(route, "/") || limit < 0 {
			return nil, fmt.Errorf("body limit for route %q requires an absolute Gin route pattern and a non-negative byte limit", route)
		}
	}
	router := gin.New()
	router.RedirectTrailingSlash = false
	router.HandleMethodNotAllowed = true
	if err := router.SetTrustedProxies(cfg.HTTP.TrustedProxies); err != nil {
		return nil, err
	}
	router.Use(requestID())
	if provider != nil {
		router.Use(otelgin.Middleware(cfg.App.Name,
			otelgin.WithTracerProvider(provider),
			otelgin.WithPropagators(telemetry.Propagator()),
			otelgin.WithGinFilter(func(c *gin.Context) bool { return !isOperationalRoute(c.FullPath()) }),
		))
	}
	router.Use(accessLog(log))
	var metricsHandler http.Handler
	if cfg.HTTP.Metrics {
		var metrics gin.HandlerFunc
		metrics, metricsHandler = newMetrics()
		router.Use(metrics)
	}
	router.Use(recovery(log))
	if cfg.CORS.Enabled {
		corsConfig := cors.DefaultConfig()
		corsConfig.AllowOrigins = cfg.CORS.AllowOrigins
		corsConfig.AllowMethods = cfg.CORS.AllowMethods
		corsConfig.AllowHeaders = cfg.CORS.AllowHeaders
		corsConfig.AllowCredentials = cfg.CORS.AllowCredentials
		corsConfig.ExposeHeaders = cfg.CORS.ExposeHeaders
		router.Use(cors.New(corsConfig))
	}
	if cfg.HTTP.MaxBodyBytes > 0 || len(maxBodyBytesByRoute) > 0 {
		router.Use(limitRequestBody(cfg.HTTP.MaxBodyBytes, maps.Clone(maxBodyBytesByRoute)))
	}
	if metricsHandler != nil {
		router.GET("/metrics", gin.WrapH(metricsHandler))
	}

	router.GET("/livez", func(c *gin.Context) {
		Respond(c, http.StatusOK, "ok", "alive", nil)
	})
	router.GET("/readyz", readiness(cfg.HTTP.HealthTimeout, checks, log))
	router.Group(V1Prefix).GET("/ping", func(c *gin.Context) {
		Respond(c, http.StatusOK, "ok", "pong", gin.H{"service": cfg.App.Name})
	})
	router.NoRoute(func(c *gin.Context) {
		Respond(c, http.StatusNotFound, "not_found", "route not found", nil)
	})
	router.NoMethod(func(c *gin.Context) {
		Respond(c, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	})
	return router, nil
}

// Respond writes the common API payload with request and trace IDs.
func Respond(c *gin.Context, status int, code, message string, data any) {
	payload := response{Code: code, Message: message, RequestID: c.GetString(requestIDKey), Data: data}
	if span := trace.SpanContextFromContext(c.Request.Context()); span.IsValid() {
		payload.TraceID = span.TraceID().String()
	}
	c.JSON(status, payload)
}

func readiness(timeout time.Duration, checks []Check, log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		errs := make([]error, len(checks))
		var group sync.WaitGroup
		for i, check := range checks {
			group.Go(func() { errs[i] = check.Ping(ctx) })
		}
		group.Wait()

		status := make(map[string]string, len(checks))
		ready := true
		for i, check := range checks {
			status[check.Name] = "up"
			if errs[i] != nil {
				status[check.Name] = "down"
				ready = false
				log.Warn("readiness check failed", zap.String("dependency", check.Name), zap.Error(errs[i]))
			}
		}
		data := gin.H{"checks": status}
		if !ready {
			Respond(c, http.StatusServiceUnavailable, "not_ready", "dependency unavailable", data)
			return
		}
		Respond(c, http.StatusOK, "ok", "ready", data)
	}
}

func isOperationalRoute(route string) bool {
	return route == "/livez" || route == "/readyz" || route == "/metrics"
}
