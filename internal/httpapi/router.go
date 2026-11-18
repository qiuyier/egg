// Package httpapi owns the Gin router, HTTP middleware, and API response format.
package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/qiuyier/egg/internal/config"
	"github.com/qiuyier/egg/internal/telemetry"
	"github.com/gin-gonic/gin"
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

func New(cfg config.Config, log *zap.Logger, provider trace.TracerProvider, checks []Check) (*gin.Engine, error) {
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
	if metricsHandler != nil {
		router.GET("/metrics", gin.WrapH(metricsHandler))
	}

	router.GET("/livez", func(c *gin.Context) {
		write(c, http.StatusOK, "ok", "alive", nil)
	})
	router.GET("/readyz", readiness(cfg.HTTP.HealthTimeout, checks, log))
	router.Group(V1Prefix).GET("/ping", func(c *gin.Context) {
		write(c, http.StatusOK, "ok", "pong", gin.H{"service": cfg.App.Name})
	})
	router.NoRoute(func(c *gin.Context) {
		write(c, http.StatusNotFound, "not_found", "route not found", nil)
	})
	router.NoMethod(func(c *gin.Context) {
		write(c, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	})
	return router, nil
}

func write(c *gin.Context, status int, code, message string, data any) {
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
			write(c, http.StatusServiceUnavailable, "not_ready", "dependency unavailable", data)
			return
		}
		write(c, http.StatusOK, "ok", "ready", data)
	}
}

func isOperationalRoute(route string) bool {
	return route == "/livez" || route == "/readyz" || route == "/metrics"
}
