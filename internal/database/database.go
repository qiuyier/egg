package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/XSAM/otelsql"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/qiuyier/egg/internal/config"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Open starts and verifies the shared SQL connection pool.
func Open(ctx context.Context, cfg config.Database, provider trace.TracerProvider) (*sqlx.DB, error) {
	if cfg.Driver == "disabled" {
		return nil, nil
	}

	connector, driverName, err := newConnector(cfg)
	if err != nil {
		return nil, err
	}

	var pool *sql.DB
	if provider == nil {
		pool = sql.OpenDB(connector)
	} else {
		system := cfg.Driver
		if system == "postgres" {
			system = "postgresql"
		}
		pool = otelsql.OpenDB(connector,
			otelsql.WithTracerProvider(provider),
			otelsql.WithAttributes(attribute.String("db.system.name", system)),
			otelsql.WithSpanOptions(otelsql.SpanOptions{
				DisableQuery: true, DisableErrSkip: true, OmitRows: true,
				OmitConnResetSession: true, OmitConnectorConnect: true,
			}),
		)
	}

	pool.SetMaxOpenConns(cfg.MaxOpenConns)
	pool.SetMaxIdleConns(cfg.MaxIdleConns)
	pool.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	pool.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	startup, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if err := pool.PingContext(startup); err != nil {
		return nil, errors.Join(fmt.Errorf("ping %s: %w", cfg.Driver, err), pool.Close())
	}
	return sqlx.NewDb(pool, driverName), nil
}

func newConnector(cfg config.Database) (driver.Connector, string, error) {
	switch cfg.Driver {
	case "mysql":
		params, err := mysqldriver.ParseDSN(cfg.DSN)
		if err != nil {
			return nil, "", errors.New("invalid MySQL DSN")
		}
		connector, err := mysqldriver.NewConnector(params)
		if err != nil {
			return nil, "", errors.New("invalid MySQL connection options")
		}
		return connector, "mysql", nil
	case "postgres":
		params, err := pgx.ParseConfig(cfg.DSN)
		if err != nil {
			return nil, "", errors.New("invalid PostgreSQL DSN")
		}
		return stdlib.GetConnector(*params), "pgx", nil
	default:
		return nil, "", fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
}
