package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFormatsAndOverrides(t *testing.T) {
	t.Setenv("EGG_APP_NAME", "from-env")
	t.Setenv("EGG_HTTP_HEALTH_TIMEOUT", "125ms")
	t.Setenv("EGG_DATABASE_DSN", "from-env-dsn")

	for _, fixture := range []struct {
		ext  string
		data string
	}{
		{"toml", "[app]\nname = 'from-file'\n[http]\naddress = ':8081'\nread_timeout = '7s'\nmetrics = true"},
		{"yaml", "app:\n  name: from-file\nhttp:\n  address: ':8081'\n  read_timeout: 7s\n  metrics: true"},
		{"yml", "app:\n  name: from-file\nhttp:\n  address: ':8081'\n  read_timeout: 7s\n  metrics: true"},
		{"json", `{"app":{"name":"from-file"},"http":{"address":":8081","read_timeout":"7s","metrics":true}}`},
		{"ini", "[app]\nname = from-file\n[http]\naddress = :8081\nread_timeout = 7s\nmetrics = true"},
		{"xml", "<config><app><name>from-file</name></app><http><address>:8081</address><read_timeout>7s</read_timeout><metrics>true</metrics></http></config>"},
	} {
		t.Run(fixture.ext, func(t *testing.T) {
			cfg, err := Load(writeFixture(t, fixture.ext, fixture.data))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.App.Name != "from-env" || cfg.HTTP.HealthTimeout != 125*time.Millisecond ||
				cfg.Database.DSN != "from-env-dsn" {
				t.Fatal("environment overrides were not applied")
			}
			if cfg.HTTP.Address != ":8081" || cfg.HTTP.ReadTimeout != 7*time.Second || !cfg.HTTP.Metrics {
				t.Fatalf("file settings were not applied: %+v", cfg.HTTP)
			}
			if cfg.App.Environment != "development" || cfg.Cache.Driver != "otter" {
				t.Fatal("defaults were not preserved")
			}
		})
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	for _, fixture := range []struct {
		name string
		ext  string
		data string
	}{
		{"unknown field", "yaml", "http:\n  read_timout: 3s"},
		{"bare timeout", "json", `{"http":{"read_timeout":5}}`},
		{"zero timeout", "json", `{"http":{"read_timeout":"0s"}}`},
		{"missing dsn", "toml", "[database]\ndriver = 'postgres'"},
		{"bad sample rate", "ini", "[tracing]\nsample_rate = 1.5"},
		{"extra yaml document", "yml", "http: {}\n---\nlog: {}"},
		{"duplicate xml field", "xml", "<config><http><metrics>true</metrics><metrics>false</metrics></http></config>"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if _, err := Load(writeFixture(t, fixture.ext, fixture.data)); err == nil {
				t.Fatal("expected invalid config to be rejected")
			}
		})
	}
}

func writeFixture(t *testing.T, ext, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config."+ext)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
