package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/qiuyier/egg/internal/config"
	"github.com/XSAM/otelsql"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Database struct {
	ORM  *gorm.DB
	pool *sql.DB
}

func Open(ctx context.Context, cfg config.Database, log *zap.Logger, provider trace.TracerProvider) (*Database, error) {
	if cfg.Driver == "disabled" {
		return nil, nil
	}

	connector, err := newConnector(cfg)
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

	opened := false
	defer func() {
		if !opened {
			_ = pool.Close()
		}
	}()
	pool.SetMaxOpenConns(cfg.MaxOpenConns)
	pool.SetMaxIdleConns(cfg.MaxIdleConns)
	pool.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	pool.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	startup, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if err := pool.PingContext(startup); err != nil {
		return nil, fmt.Errorf("ping %s: %w", cfg.Driver, err)
	}

	var dialect gorm.Dialector
	switch cfg.Driver {
	case "mysql":
		// The MySQL 8 dialect is selected explicitly to avoid an unbounded
		// version query during GORM initialization.
		dialect = mysql.New(mysql.Config{Conn: pool, SkipInitializeWithVersion: true})
	case "postgres":
		dialect = postgres.New(postgres.Config{Conn: pool})
	}
	writer, err := zap.NewStdLogAt(log.Named("gorm"), zap.WarnLevel)
	if err != nil {
		return nil, err
	}
	orm, err := gorm.Open(dialect, &gorm.Config{
		DisableAutomaticPing: true,
		Logger: logger.New(writer, logger.Config{
			LogLevel:                  logger.Warn,
			SlowThreshold:             cfg.SlowThreshold,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("initialize %s dialect: %w", cfg.Driver, err)
	}

	opened = true
	return &Database{ORM: orm, pool: pool}, nil
}

func newConnector(cfg config.Database) (driver.Connector, error) {
	switch cfg.Driver {
	case "mysql":
		params, err := mysqldriver.ParseDSN(cfg.DSN)
		if err != nil {
			return nil, errors.New("invalid MySQL DSN")
		}
		connector, err := mysqldriver.NewConnector(params)
		if err != nil {
			return nil, errors.New("invalid MySQL connection options")
		}
		return connector, nil
	case "postgres":
		params, err := pgx.ParseConfig(cfg.DSN)
		if err != nil {
			return nil, errors.New("invalid PostgreSQL DSN")
		}
		return stdlib.GetConnector(*params), nil
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
}

func (d *Database) Ping(ctx context.Context) error {
	return d.pool.PingContext(ctx)
}

func (d *Database) Close() error {
	return d.pool.Close()
}
