package cache

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
	"uuid"

	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestOtterAdapter(t *testing.T) {
	adapter, err := NewOtter(100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	var registry Manager
	if err := registry.Register("custom", adapter); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("custom", adapter); err == nil {
		t.Fatal("duplicate adapter registration succeeded")
	}
	selected, ok := registry.Lookup("custom")
	if !ok || selected != adapter {
		t.Fatal("registered adapter was not found")
	}
	checkAdapter(t, selected, t.Context())
}

func TestRedisAdapter(t *testing.T) {
	url := os.Getenv("EGG_TEST_REDIS_URL")
	if url == "" {
		t.Skip("EGG_TEST_REDIS_URL is unset")
	}

	spans := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(spans))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	adapter, err := NewRedis(url, "egg:test:"+uuid.NewV4().String()+":", time.Second, provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	ctx, parent := provider.Tracer("test").Start(t.Context(), "cache test")
	defer parent.End()
	if err := adapter.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	checkAdapter(t, adapter, ctx)
	for _, span := range spans.Ended() {
		if span.Parent().SpanID() == parent.SpanContext().SpanID() {
			return
		}
	}
	t.Fatal("expected a Redis span under the calling span")
}

func checkAdapter(t *testing.T, adapter Adapter, ctx context.Context) {
	t.Helper()
	const key = "entry"
	t.Cleanup(func() { _ = adapter.Delete(context.Background(), key) })

	if _, err := adapter.Get(ctx, key); !errors.Is(err, ErrMiss) {
		t.Fatalf("expected miss, got %v", err)
	}
	if err := adapter.Set(ctx, key, []byte("ignored"), -time.Second); !errors.Is(err, ErrInvalidTTL) {
		t.Fatalf("expected invalid TTL, got %v", err)
	}
	value := []byte("value")
	if err := adapter.Set(ctx, key, value, 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Set(ctx, key, value, 0); err != nil {
		t.Fatal(err)
	}
	value[0] = 'X'
	time.Sleep(60 * time.Millisecond)

	data, err := adapter.Get(ctx, key)
	if err != nil || string(data) != "value" {
		t.Fatalf("overwrite should replace the TTL and preserve bytes, got %q: %v", data, err)
	}
	data[0] = 'Y'
	data, err = adapter.Get(ctx, key)
	if err != nil || string(data) != "value" {
		t.Fatalf("Get leaked mutable storage, got %q: %v", data, err)
	}
	if err := adapter.Set(ctx, key, nil, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := adapter.Get(ctx, key)
		if errors.Is(err, ErrMiss) {
			break
		}
		if err != nil || time.Now().After(deadline) {
			t.Fatalf("expected expiry, last Get error was %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := adapter.Set(ctx, key, nil, 0); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Get(ctx, key); !errors.Is(err, ErrMiss) {
		t.Fatalf("expected miss after deletion, got %v", err)
	}
}
