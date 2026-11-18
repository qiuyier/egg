package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func limitRequestBody(defaultBytes int64, perRoute map[string]int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := defaultBytes
		if override, exists := perRoute[c.FullPath()]; exists {
			limit = override
		}
		if limit > 0 {
			if c.Request.ContentLength > limit {
				rejectLargeBody(c)
				return
			}
			if c.Request.Body != nil && c.Request.Body != http.NoBody {
				c.Request.Body = &limitedBody{
					ReadCloser: http.MaxBytesReader(c.Writer, c.Request.Body, limit),
					context:    c,
				}
			}
		}
		c.Next()
	}
}

func rejectLargeBody(c *gin.Context) {
	if c.Request.ProtoMajor == 1 {
		// Send the rejection without draining the rest of an oversized HTTP/1.x request.
		c.Header("Connection", "close")
	}
	c.Abort()
	Respond(c, http.StatusRequestEntityTooLarge, "request_body_too_large", "request body exceeds the configured limit", nil)
}

type limitedBody struct {
	io.ReadCloser
	context *gin.Context
}

func (b *limitedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		if !b.context.Writer.Written() {
			rejectLargeBody(b.context)
			// The handler is still unwinding and may try to render its own error response.
			b.context.Writer = discardResponseBody{b.context.Writer}
		}
		return 0, err
	}
	return n, err
}

type discardResponseBody struct{ gin.ResponseWriter }

func (discardResponseBody) WriteHeader(int)                   {}
func (discardResponseBody) Write(p []byte) (int, error)       { return len(p), nil }
func (discardResponseBody) WriteString(s string) (int, error) { return len(s), nil }

// RequestTimeout adds a processing deadline to the request's context. A zero or negative value leaves it unchanged.
func RequestTimeout(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if timeout <= 0 {
			c.Next()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		if ctx.Err() == context.DeadlineExceeded && !c.Writer.Written() {
			c.Abort()
			Respond(c, http.StatusGatewayTimeout, "request_timeout", "request processing deadline exceeded", nil)
		}
	}
}
