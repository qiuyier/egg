package database

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/qiuyier/egg/internal/config"
	"github.com/qiuyier/egg/internal/database/gormadapter"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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
				MaxOpenConns: 3, MaxIdleConns: 2,
			}, provider)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })

			ctx, parent := provider.Tracer("test").Start(t.Context(), "database test")
			defer parent.End()
			tx, err := db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()
			query, args, err := tx.BindNamed("SELECT 1 + :base AS result", map[string]any{"base": 41})
			if err != nil {
				t.Fatal(err)
			}
			var row struct {
				Result int `db:"result"`
			}
			if err := tx.GetContext(ctx, &row, query, args...); err != nil || row.Result != 42 {
				t.Fatalf("expected SQLx result 42, got %d: %v", row.Result, err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if !slices.ContainsFunc(spans.Ended(), func(span trace.ReadOnlySpan) bool {
				method := otelsql.Method(span.Name())
				return span.Parent().SpanID() == parent.SpanContext().SpanID() &&
					(method == otelsql.MethodConnQuery || method == otelsql.MethodStmtQuery)
			}) {
				t.Fatal("expected a SQL query span under the calling span")
			}

			t.Run("gorm shares pool", func(t *testing.T) {
				orm, err := gormadapter.Open(db, time.Second, nil)
				if err != nil {
					t.Fatal(err)
				}
				pool, err := orm.DB()
				if err != nil || pool != db.DB {
					t.Fatalf("GORM did not reuse the sqlx pool: %v", err)
				}
				var value int
				if err := orm.WithContext(ctx).Raw("SELECT 1 + ?", 41).Scan(&value).Error; err != nil || value != 42 {
					t.Fatalf("expected GORM result 42, got %d: %v", value, err)
				}
			})
		})
	}
}
