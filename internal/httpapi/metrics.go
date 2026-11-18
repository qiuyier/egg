package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func newMetrics() (gin.HandlerFunc, http.Handler) {
	registry := prometheus.NewRegistry()
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "egg_http_requests_total",
		Help: "Completed API HTTP requests.",
	}, []string{"method", "route", "status"})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "egg_http_request_duration_seconds",
		Help: "API HTTP request duration in seconds.",
	}, []string{"method", "route"})

	registry.MustRegister(requests, duration,
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	middleware := func(c *gin.Context) {
		start := time.Now()
		c.Next()
		if isOperationalRoute(c.FullPath()) {
			return
		}
		method, route := methodLabel(c.Request.Method), routeLabel(c)
		requests.WithLabelValues(method, route, strconv.Itoa(c.Writer.Status())).Inc()
		duration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
	}
	return middleware, promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}

func methodLabel(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		return method
	default:
		return "OTHER"
	}
}
