package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qiuyier/egg/internal/config"
	"go.uber.org/zap"
)

func TestRequestBodyLimits(t *testing.T) {
	limits := map[string]int64{"/upload/:id": 16, "/unlimited": 0}
	router, err := New(config.Config{HTTP: config.HTTP{MaxBodyBytes: 8}}, zap.NewNop(), nil, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	limits["/upload/:id"] = 1
	calls := 0
	handler := func(c *gin.Context) {
		calls++
		if _, err := io.ReadAll(c.Request.Body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"fallback": "handler error"})
			return
		}
		c.Status(http.StatusNoContent)
	}
	for _, path := range []string{"/default", "/upload/:id", "/unlimited"} {
		router.POST(path, handler)
	}

	for _, tc := range []struct {
		name, path string
		size       int
		unknown    bool
		status     int
		reads      bool
	}{
		{"at limit", "/default", 8, false, http.StatusNoContent, true},
		{"known oversize", "/default", 9, false, http.StatusRequestEntityTooLarge, false},
		{"unknown at limit", "/default", 8, true, http.StatusNoContent, true},
		{"unknown oversize", "/default", 9, true, http.StatusRequestEntityTooLarge, true},
		{"larger upload", "/upload/123", 16, false, http.StatusNoContent, true},
		{"upload oversize", "/upload/123", 17, true, http.StatusRequestEntityTooLarge, true},
		{"disabled for route", "/unlimited", 64, true, http.StatusNoContent, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls = 0
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(strings.Repeat("x", tc.size)))
			if tc.unknown {
				req.ContentLength = -1
			}
			req.Header.Set("X-Request-ID", "body-limits-test")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status || (calls > 0) != tc.reads {
				t.Fatalf("unexpected response or handler calls: %d %s, calls %d", rec.Code, rec.Body, calls)
			}
			if tc.status == http.StatusRequestEntityTooLarge {
				var body response
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != "request_body_too_large" || body.RequestID != "body-limits-test" {
					t.Fatalf("expected one structured 413 response: %s (%v)", rec.Body, err)
				}
			}
		})
	}

	accepted := false
	router.POST("/json", func(c *gin.Context) {
		var data map[string]int
		if err := c.ShouldBindJSON(&data); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		accepted = true
	})
	req := httptest.NewRequest(http.MethodPost, "/json", strings.NewReader(`{"a": 1}x`))
	req.ContentLength = -1
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if accepted || rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("accepted a JSON value truncated to fit the limit: %d %s", rec.Code, rec.Body)
	}
}

func TestRouteRequestTimeout(t *testing.T) {
	router, err := New(config.Config{}, zap.NewNop(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	group := router.Group("/timed", RequestTimeout(time.Second))
	group.GET("/wait", func(c *gin.Context) {
		ctx := c.Request.Context()
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 100*time.Millisecond {
			t.Error("expected the earlier parent deadline to be preserved")
			return
		}
		<-ctx.Done()
	})
	group.GET("/written", func(c *gin.Context) {
		c.String(http.StatusOK, "sent")
		<-c.Request.Context().Done()
	})
	router.GET("/route-deadline", RequestTimeout(10*time.Millisecond), func(c *gin.Context) {
		ctx := c.Request.Context()
		if _, ok := ctx.Deadline(); !ok {
			t.Error("route deadline was not applied")
			return
		}
		<-ctx.Done()
	})
	router.GET("/plain", func(c *gin.Context) {
		if _, ok := c.Request.Context().Deadline(); ok {
			t.Error("an unrelated route has a deadline")
		}
		c.Status(http.StatusNoContent)
	})

	parent, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	for _, tc := range []struct {
		path   string
		ctx    context.Context
		status int
	}{
		{"/timed/wait", parent, http.StatusGatewayTimeout},
		{"/timed/written", parent, http.StatusOK},
		{"/route-deadline", t.Context(), http.StatusGatewayTimeout},
		{"/plain", t.Context(), http.StatusNoContent},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequestWithContext(tc.ctx, http.MethodGet, tc.path, nil))
			if rec.Code != tc.status {
				t.Fatalf("unexpected status: %d %s", rec.Code, rec.Body)
			}
			if tc.status == http.StatusGatewayTimeout {
				var body response
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != "request_timeout" {
					t.Fatalf("unexpected timeout response: %s (%v)", rec.Body, err)
				}
			}
			if tc.path == "/timed/written" && rec.Body.String() != "sent" {
				t.Fatalf("the committed response was changed: %s", rec.Body)
			}
		})
	}
}
