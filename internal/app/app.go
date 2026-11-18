// Package app wires application components and owns their lifecycle.
package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/qiuyier/egg/internal/config"
	"github.com/qiuyier/egg/internal/database"
	"github.com/qiuyier/egg/internal/httpapi"
	"github.com/qiuyier/egg/internal/logging"
	"github.com/qiuyier/egg/internal/telemetry"
	"github.com/qiuyier/egg/pkg/cache"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type App struct {
	server          *http.Server
	log             *zap.Logger
	shutdownTimeout time.Duration
	cleanup         []func(context.Context) error
}

type Dependencies struct {
	Cache cache.Adapter
	DB    *sqlx.DB
}

type Options struct {
	// CustomCaches contains caller-owned adapters selected by cache.driver.
	CustomCaches *cache.Manager
	RegisterV1   func(*gin.RouterGroup, Dependencies)
	// MaxBodyBytesByRoute overrides the limit for a full Gin route pattern. Zero disables it on that route.
	MaxBodyBytesByRoute map[string]int64
}

func New(ctx context.Context, cfg config.Config, options Options) (_ *App, err error) {
	a := &App{shutdownTimeout: cfg.HTTP.ShutdownTimeout}
	defer func() {
		if err != nil {
			err = errors.Join(err, a.Close())
		}
	}()

	log, closeLog, err := logging.New(cfg.Log)
	if err != nil {
		return nil, fmt.Errorf("initialize logger: %w", err)
	}
	a.log = log.With(zap.String("service", cfg.App.Name))
	a.cleanup = append(a.cleanup, func(context.Context) error { return closeLog() })

	var provider trace.TracerProvider
	tracer, err := telemetry.New(ctx, cfg.App, cfg.Tracing)
	if err != nil {
		return nil, err
	}
	if tracer != nil {
		provider = tracer
		a.cleanup = append(a.cleanup, tracer.Shutdown)
	}

	activeCache, err := a.openCache(cfg.Cache, options.CustomCaches, provider)
	if err != nil {
		return nil, fmt.Errorf("initialize %s cache: %w", cfg.Cache.Driver, err)
	}
	deps := Dependencies{Cache: activeCache}

	var checks []httpapi.Check
	if remote, ok := activeCache.(interface{ Ping(context.Context) error }); ok {
		startup, cancel := context.WithTimeout(ctx, cfg.Cache.DialTimeout)
		err := remote.Ping(startup)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("ping %s cache: %w", cfg.Cache.Driver, err)
		}
		checks = append(checks, httpapi.Check{Name: "cache", Ping: remote.Ping})
	}

	db, err := database.Open(ctx, cfg.Database, provider)
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	if db != nil {
		deps.DB = db
		a.cleanup = append(a.cleanup, func(context.Context) error { return db.Close() })
		checks = append(checks, httpapi.Check{Name: "database", Ping: db.PingContext})
	}

	router, err := httpapi.New(cfg, a.log, provider, checks, options.MaxBodyBytesByRoute)
	if err != nil {
		return nil, fmt.Errorf("initialize router: %w", err)
	}
	if options.RegisterV1 != nil {
		options.RegisterV1(router.Group(httpapi.V1Prefix), deps)
	}
	a.server = &http.Server{
		Addr:              cfg.HTTP.Address,
		Handler:           router,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		ErrorLog:          zap.NewStdLog(a.log.Named("http")),
	}
	a.log.Info("dependencies initialized",
		zap.String("environment", cfg.App.Environment),
		zap.String("cache_driver", cfg.Cache.Driver),
		zap.String("database_driver", cfg.Database.Driver),
		zap.Bool("tracing", cfg.Tracing.Enabled),
	)
	return a, nil
}

func (a *App) openCache(cfg config.Cache, custom *cache.Manager, provider trace.TracerProvider) (cache.Adapter, error) {
	var adapter interface {
		cache.Adapter
		Close() error
	}
	var err error
	switch cfg.Driver {
	case "otter":
		adapter, err = cache.NewOtter(cfg.MaxEntries)
	case "redis":
		adapter, err = cache.NewRedis(cfg.RedisURL, cfg.KeyPrefix, cfg.DialTimeout, provider)
	default:
		if custom != nil {
			if adapter, found := custom.Lookup(cfg.Driver); found {
				return adapter, nil
			}
		}
		return nil, fmt.Errorf("cache adapter %q is not registered", cfg.Driver)
	}
	if err != nil {
		return nil, err
	}
	a.cleanup = append(a.cleanup, func(context.Context) error { return adapter.Close() })
	return adapter, nil
}

func (a *App) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", a.server.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	a.log.Info("http listening", zap.String("address", listener.Addr().String()))
	served := make(chan error, 1)
	go func() { served <- a.server.Serve(listener) }()

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}

	a.log.Info("shutting down http server")
	shutdown, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()
	if err := a.server.Shutdown(shutdown); err != nil {
		return errors.Join(fmt.Errorf("graceful HTTP shutdown: %w", err), a.server.Close())
	}
	if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Close releases initialized components after HTTP serving has stopped.
func (a *App) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()
	var err error
	for i := len(a.cleanup) - 1; i >= 0; i-- {
		err = errors.Join(err, a.cleanup[i](ctx))
	}
	a.cleanup = nil
	return err
}
