// Package config loads the application's startup configuration.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
)

type Config struct {
	App      App      `mapstructure:"app"`
	HTTP     HTTP     `mapstructure:"http"`
	CORS     CORS     `mapstructure:"cors"`
	Log      Log      `mapstructure:"log"`
	Cache    Cache    `mapstructure:"cache"`
	Database Database `mapstructure:"database"`
	Tracing  Tracing  `mapstructure:"tracing"`
}

type App struct {
	Name        string `mapstructure:"name"`
	Environment string `mapstructure:"environment"`
}

type HTTP struct {
	Address           string        `mapstructure:"address"`
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout"`
	ReadTimeout       time.Duration `mapstructure:"read_timeout"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout   time.Duration `mapstructure:"shutdown_timeout"`
	HealthTimeout     time.Duration `mapstructure:"health_timeout"`
	MaxBodyBytes      int64         `mapstructure:"max_body_bytes"`
	TrustedProxies    []string      `mapstructure:"trusted_proxies"`
	Metrics           bool          `mapstructure:"metrics"`
}

type CORS struct {
	Enabled          bool     `mapstructure:"enabled"`
	AllowOrigins     []string `mapstructure:"allow_origins"`
	AllowMethods     []string `mapstructure:"allow_methods"`
	AllowHeaders     []string `mapstructure:"allow_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
	ExposeHeaders    []string `mapstructure:"expose_headers"`
}

// Validate checks that enabled CORS uses explicit HTTP(S) origins.
func (c CORS) Validate() error {
	if !c.Enabled {
		return nil
	}
	if len(c.AllowOrigins) == 0 {
		return errors.New("cors.allow_origins is required when CORS is enabled")
	}
	for _, origin := range c.AllowOrigins {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
			!strings.HasPrefix(origin, u.Scheme+"://") || u.User != nil || u.Path != "" ||
			u.RawQuery != "" || u.ForceQuery || strings.ContainsAny(origin, "*#") {
			return fmt.Errorf("cors.allow_origins entry %q must be an exact HTTP(S) origin without a path, query or fragment", origin)
		}
	}
	return nil
}

type Log struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	Color      bool   `mapstructure:"color"`
	Stdout     bool   `mapstructure:"stdout"`
	File       string `mapstructure:"file"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAgeDays int    `mapstructure:"max_age_days"`
	Compress   bool   `mapstructure:"compress"`
}

type Cache struct {
	Driver      string        `mapstructure:"driver"`
	MaxEntries  int           `mapstructure:"max_entries"`
	RedisURL    string        `mapstructure:"redis_url"`
	KeyPrefix   string        `mapstructure:"key_prefix"`
	DialTimeout time.Duration `mapstructure:"dial_timeout"`
}

type Database struct {
	Driver          string        `mapstructure:"driver"`
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
	ConnectTimeout  time.Duration `mapstructure:"connect_timeout"`
	SlowThreshold   time.Duration `mapstructure:"slow_threshold"`
}

type Tracing struct {
	Enabled    bool    `mapstructure:"enabled"`
	Endpoint   string  `mapstructure:"endpoint"`
	SampleRate float64 `mapstructure:"sample_rate"`
}

func defaults() Config {
	return Config{
		App: App{Name: "egg", Environment: "development"},
		HTTP: HTTP{
			Address: ":8080", ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second,
			IdleTimeout: 60 * time.Second, ShutdownTimeout: 10 * time.Second,
			HealthTimeout: 2 * time.Second, MaxBodyBytes: 1 << 20,
		},
		CORS: CORS{
			AllowMethods:  []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders:  []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID"},
			ExposeHeaders: []string{"X-Request-ID"},
		},
		Log: Log{
			Level: "info", Format: "console", Color: true, Stdout: true,
			MaxSizeMB: 100, MaxBackups: 7, MaxAgeDays: 14, Compress: true,
		},
		Cache: Cache{
			Driver: "otter", MaxEntries: 10_000, KeyPrefix: "egg:", DialTimeout: 3 * time.Second,
		},
		Database: Database{
			Driver: "disabled", MaxOpenConns: 25, MaxIdleConns: 10,
			ConnMaxLifetime: 30 * time.Minute, ConnMaxIdleTime: 5 * time.Minute,
			ConnectTimeout: 5 * time.Second, SlowThreshold: 200 * time.Millisecond,
		},
		Tracing: Tracing{SampleRate: 1},
	}
}

// Load applies defaults, the selected file, then EGG_<SECTION>_<FIELD> overrides.
func Load(path string) (Config, error) {
	cfg := defaults()
	if path != "" {
		fields, err := readFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("read config %q: %w", path, err)
		}
		if err := decode(fields, &cfg); err != nil {
			return Config{}, fmt.Errorf("decode config %q: %w", path, err)
		}
	}

	var schema map[string]any
	if err := mapstructure.Decode(defaults(), &schema); err != nil {
		return Config{}, fmt.Errorf("build config environment schema: %w", err)
	}
	env := environment(schema, "EGG")
	if err := decode(env, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode environment overrides: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func environment(schema map[string]any, prefix string) map[string]any {
	fields := make(map[string]any)
	for key, value := range schema {
		name := prefix + "_" + strings.ToUpper(key)
		if children, ok := value.(map[string]any); ok {
			if overrides := environment(children, name); len(overrides) > 0 {
				fields[key] = overrides
			}
		} else if override, found := os.LookupEnv(name); found {
			fields[key] = override
		}
	}
	return fields
}

func decode(fields map[string]any, cfg *Config) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           cfg,
		ErrorUnused:      true,
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			durationHook,
			mapstructure.StringToSliceHookFunc(","),
		),
	})
	if err != nil {
		return err
	}
	return decoder.Decode(fields)
}

func durationHook(from, to reflect.Type, value any) (any, error) {
	if to != reflect.TypeFor[time.Duration]() {
		return value, nil
	}
	if from.Kind() != reflect.String {
		return nil, errors.New("duration must be a string such as 250ms or 5s")
	}
	return time.ParseDuration(value.(string))
}

func (c Config) validate() error {
	if strings.TrimSpace(c.App.Name) == "" || strings.TrimSpace(c.App.Environment) == "" {
		return errors.New("app.name and app.environment must be non-empty")
	}
	if _, port, err := net.SplitHostPort(c.HTTP.Address); err != nil || port == "" {
		return errors.New("http.address must be in host:port form")
	}
	if c.HTTP.ReadHeaderTimeout <= 0 || c.HTTP.ReadTimeout <= 0 ||
		c.HTTP.WriteTimeout <= 0 || c.HTTP.IdleTimeout <= 0 ||
		c.HTTP.ShutdownTimeout <= 0 || c.HTTP.HealthTimeout <= 0 {
		return errors.New("http timeouts must be positive")
	}
	if c.HTTP.MaxBodyBytes < 0 {
		return errors.New("http.max_body_bytes must be non-negative")
	}
	if err := c.CORS.Validate(); err != nil {
		return err
	}
	for _, proxy := range c.HTTP.TrustedProxies {
		if _, err := netip.ParsePrefix(proxy); err != nil {
			if addr, err := netip.ParseAddr(proxy); err != nil || addr.Zone() != "" {
				return fmt.Errorf("invalid http.trusted_proxies entry %q", proxy)
			}
		}
	}
	switch c.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("log.level must be debug, info, warn or error")
	}
	if c.Log.Format != "console" && c.Log.Format != "json" {
		return errors.New("log.format must be console or json")
	}
	if !c.Log.Stdout && c.Log.File == "" {
		return errors.New("log requires stdout or a file")
	}
	if c.Log.File != "" && (c.Log.MaxSizeMB <= 0 || c.Log.MaxBackups < 0 || c.Log.MaxAgeDays < 0) {
		return errors.New("log rotation requires a positive size and non-negative retention limits")
	}
	if strings.TrimSpace(c.Cache.Driver) == "" || c.Cache.DialTimeout <= 0 {
		return errors.New("cache requires a driver and a positive cache.dial_timeout")
	}
	switch c.Cache.Driver {
	case "otter":
		if c.Cache.MaxEntries <= 0 {
			return errors.New("cache.max_entries must be positive")
		}
	case "redis":
		if c.Cache.RedisURL == "" {
			return errors.New("redis requires cache.redis_url")
		}
	}
	switch c.Database.Driver {
	case "disabled":
	case "mysql", "postgres":
		if c.Database.DSN == "" {
			return errors.New("database.dsn is required when the database is enabled")
		}
		if c.Database.MaxOpenConns <= 0 || c.Database.MaxIdleConns < 0 ||
			c.Database.MaxIdleConns > c.Database.MaxOpenConns {
			return errors.New("database pool requires max_open_conns > 0 and 0 <= max_idle_conns <= max_open_conns")
		}
		if c.Database.ConnectTimeout <= 0 || c.Database.SlowThreshold <= 0 ||
			c.Database.ConnMaxLifetime < 0 || c.Database.ConnMaxIdleTime < 0 {
			return errors.New("database timeouts are invalid")
		}
	default:
		return errors.New("database.driver must be disabled, mysql or postgres")
	}
	if !(c.Tracing.SampleRate >= 0 && c.Tracing.SampleRate <= 1) {
		return errors.New("tracing.sample_rate must be between 0 and 1")
	}
	if c.Tracing.Endpoint != "" {
		u, err := url.Parse(c.Tracing.Endpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") ||
			u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("tracing.endpoint must be an HTTP(S) URL without credentials, query or fragment")
		}
	}
	return nil
}
