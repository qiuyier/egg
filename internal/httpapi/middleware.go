package httpapi

import (
	"errors"
	"net/http"
	"runtime/debug"
	"time"
	"uuid"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const requestIDKey = "request_id"

func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if !validRequestID(id) {
			id = uuid.NewV4().String()
		}
		c.Set(requestIDKey, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func validRequestID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for i := range len(id) {
		b := id[i]
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' ||
			b == '-' || b == '_' || b == '.') {
			return false
		}
	}
	return true
}

func requestFields(c *gin.Context) []zap.Field {
	fields := []zap.Field{zap.String(requestIDKey, c.GetString(requestIDKey))}
	if span := trace.SpanContextFromContext(c.Request.Context()); span.IsValid() {
		fields = append(fields, zap.String("trace_id", span.TraceID().String()), zap.String("span_id", span.SpanID().String()))
	}
	return fields
}

func accessLog(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		fields := append(requestFields(c),
			zap.String("method", c.Request.Method),
			zap.String("route", routeLabel(c)),
			zap.Int("status", status),
			zap.Duration("duration", time.Since(start)),
			zap.Int("bytes", max(c.Writer.Size(), 0)),
		)
		switch {
		case status >= 500:
			log.Error("http request", fields...)
		case status >= 400:
			log.Warn("http request", fields...)
		default:
			log.Info("http request", fields...)
		}
	}
}

func recovery(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				fields := append(requestFields(c), zap.Any("panic", recovered), zap.ByteString("stack", debug.Stack()))
				log.Error("http handler panic", fields...)
				trace.SpanFromContext(c.Request.Context()).RecordError(errors.New("http handler panic"))
				c.Abort()
				if !c.Writer.Written() {
					write(c, http.StatusInternalServerError, "internal_error", "internal server error", nil)
				}
			}
		}()
		c.Next()
	}
}

func routeLabel(c *gin.Context) string {
	if route := c.FullPath(); route != "" {
		return route
	}
	return "unmatched"
}
