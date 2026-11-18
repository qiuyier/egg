package logging

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qiuyier/egg/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

func New(cfg config.Log) (*zap.Logger, func() error, error) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		return nil, nil, err
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	var cores []zapcore.Core
	if cfg.Stdout {
		var encoder zapcore.Encoder
		if cfg.Format == "json" {
			encoder = zapcore.NewJSONEncoder(encoderConfig)
		} else {
			consoleConfig := encoderConfig
			if cfg.Color {
				consoleConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
			}
			encoder = zapcore.NewConsoleEncoder(consoleConfig)
		}
		cores = append(cores, zapcore.NewCore(encoder, zapcore.Lock(os.Stdout), level))
	}

	closeFile := func() error { return nil }
	if cfg.File != "" {
		if err := prepareLogFile(cfg.File); err != nil {
			return nil, nil, fmt.Errorf("open log file: %w", err)
		}
		rolling := &lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    cfg.MaxSizeMB,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAgeDays,
			Compress:   cfg.Compress,
		}
		// Lumberjack opens lazily. Check the destination before serving requests.
		if _, err := rolling.Write(nil); err != nil {
			return nil, nil, fmt.Errorf("open log file: %w", err)
		}
		closeFile = rolling.Close
		cores = append(cores, zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig), zapcore.AddSync(rolling), level,
		))
	}

	log := zap.New(zapcore.NewTee(cores...), zap.AddCaller(), zap.ErrorOutput(zapcore.Lock(os.Stderr)))
	return log, closeFile, nil
}

func prepareLogFile(path string) error {
	info, err := os.Lstat(path)
	switch {
	case err == nil && !info.Mode().IsRegular():
		return fmt.Errorf("%q must be a regular file", path)
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	return file.Close()
}
