package app

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qiuyier/egg/internal/config"
	"github.com/qiuyier/egg/pkg/cache"
	"github.com/gin-gonic/gin"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestOTLPHTTPExportOnShutdown(t *testing.T) {
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	const parentID = "00f067aa0ba902b7"

	requests := make(chan *tracepb.ExportTraceServiceRequest, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/traces" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		data, err := io.ReadAll(r.Body)
		payload := new(tracepb.ExportTraceServiceRequest)
		if err != nil || proto.Unmarshal(data, payload) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		select {
		case requests <- payload:
		default:
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(collector.Close)

	t.Setenv("EGG_TRACING_ENABLED", "true")
	t.Setenv("EGG_TRACING_ENDPOINT", collector.URL+"/v1/traces")
	t.Setenv("EGG_LOG_LEVEL", "error")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_COMPRESSION", "none")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(t.Context(), cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	req.Header.Set("traceparent", "00-"+traceID+"-"+parentID+"-01")
	rec := httptest.NewRecorder()
	service.server.Handler.ServeHTTP(rec, req)
	var body struct {
		TraceID string `json:"trace_id"`
	}
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || err != nil || body.TraceID != traceID {
		t.Fatalf("request trace was not propagated: %d %s (%v)", rec.Code, rec.Body, err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case payload := <-requests:
		for _, resource := range payload.ResourceSpans {
			for _, scope := range resource.ScopeSpans {
				for _, span := range scope.Spans {
					if hex.EncodeToString(span.TraceId) == traceID && hex.EncodeToString(span.ParentSpanId) == parentID {
						return
					}
				}
			}
		}
		t.Fatal("export did not contain a server span with the incoming trace and parent IDs")
	default:
		t.Fatal("no spans were exported during shutdown")
	}
}

func TestCustomCacheAndRoutes(t *testing.T) {
	t.Setenv("EGG_CACHE_DRIVER", "custom")
	t.Setenv("EGG_LOG_LEVEL", "error")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := cache.NewOtter(10)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	var registry cache.Manager
	if err := registry.Register("custom", adapter); err != nil {
		t.Fatal(err)
	}
	service, err := New(t.Context(), cfg, Options{
		CustomCaches: &registry,
		RegisterV1: func(routes *gin.RouterGroup, deps Dependencies) {
			if deps.Cache != adapter || deps.DB != nil {
				t.Fatal("unexpected dependencies")
			}
			routes.GET("/custom", func(c *gin.Context) {
				if err := deps.Cache.Set(c.Request.Context(), "example", []byte("ok"), 0); err != nil {
					c.Status(http.StatusInternalServerError)
					return
				}
				c.Status(http.StatusNoContent)
			})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })

	rec := httptest.NewRecorder()
	service.server.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/custom", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	got, err := adapter.Get(t.Context(), "example")
	if err != nil || string(got) != "ok" {
		t.Fatalf("route did not write to the configured cache: %q %v", got, err)
	}
}
