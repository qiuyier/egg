package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/qiuyier/egg/internal/app"
	"github.com/qiuyier/egg/internal/config"
	"github.com/qiuyier/egg/internal/logging"
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
	options, err := apiOptions(cfg)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	gin.SetMode(gin.ReleaseMode)
	logging.PrintStartupBanner(cfg.Log)
	service, err := app.New(ctx, cfg, options)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, service.Close()) }()
	return service.Run(ctx)
}
