package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/qiuyier/egg/internal/config"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestDatabaseDrivers(t *testing.T) {
	for driver, variable := range map[string]string{
		"mysql": "EGG_TEST_MYSQL_DSN", "postgres": "EGG_TEST_POSTGRES_DSN",
	} {
		t.Run(driver, func(t *testing.T) {
			dsn := os.Getenv(variable)
			if dsn == "" {
				t.Skip(variable + " is unset")
			}
			spans := tracetest.NewSpanRecorder()
			provider := trace.NewTracerProvider(trace.WithSpanProcessor(spans))
			t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
			db, err := Open(t.Context(), config.Database{
				Driver: driver, DSN: dsn, ConnectTimeout: 5 * time.Second,
				MaxOpenConns: 3, MaxIdleConns: 2, SlowThreshold: time.Second,
			}, zap.NewNop(), provider)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })

			ctx, parent := provider.Tracer("test").Start(t.Context(), "database test")
			defer parent.End()
			err = db.ORM.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var value int
				if err := tx.Raw("SELECT 1 + ?", 41).Scan(&value).Error; err != nil {
					return err
				}
				if value != 42 {
					t.Errorf("expected 42, got %d", value)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, span := range spans.Ended() {
				if span.Parent().SpanID() == parent.SpanContext().SpanID() {
					return
				}
			}
			t.Fatal("expected a SQL span under the calling span")
		})
	}
}
