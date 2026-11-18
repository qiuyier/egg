package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/qiuyier/egg/internal/app"
	"github.com/qiuyier/egg/internal/config"
	"github.com/qiuyier/egg/internal/logging"
	"github.com/gin-gonic/gin"
)

func main() {
	path := flag.String("config", os.Getenv("EGG_CONFIG"), "configuration file path")
	flag.Parse()
	if err := run(*path); err != nil {
		fmt.Fprintln(os.Stderr, "egg:", err)
		os.Exit(1)
	}
}

func run(path string) (err error) {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	gin.SetMode(gin.ReleaseMode)
	logging.PrintStartupBanner(cfg.Log)
	service, err := app.New(ctx, cfg, app.Options{})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, service.Close()) }()
	return service.Run(ctx)
}
