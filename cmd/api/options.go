package main

import (
	"github.com/qiuyier/egg/internal/app"
	"github.com/qiuyier/egg/internal/config"
)

func apiOptions(_ config.Config) (app.Options, error) {
	return app.Options{}, nil
}
