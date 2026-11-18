package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiuyier/egg/internal/config"
	"go.uber.org/zap"
)

func TestJSONFileRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.log")
	log, closeLog, err := New(config.Log{
		Level: "info", File: path, MaxSizeMB: 1, MaxBackups: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeLog() })
	payload := strings.Repeat("a", 600_000)
	log.Info("first", zap.String("payload", payload))
	log.Info("second", zap.String("payload", payload))

	archives, err := filepath.Glob(filepath.Join(filepath.Dir(path), "service-*.log"))
	if err != nil || len(archives) != 1 {
		t.Fatalf("expected one backup, got %v: %v", archives, err)
	}
	for file, want := range map[string]string{archives[0]: "first", path: "second"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var entry struct {
			Message string `json:"msg"`
		}
		if err := json.Unmarshal(data, &entry); err != nil || entry.Message != want {
			t.Fatalf("expected JSON message %q, got %q: %v", want, entry.Message, err)
		}
	}
}

func TestRejectsLogDirectory(t *testing.T) {
	dir := t.TempDir()
	_, closeLog, err := New(config.Log{Level: "info", File: dir, MaxSizeMB: 1})
	if err == nil {
		_ = closeLog()
		t.Fatal("a directory must be rejected as a log destination")
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("the directory must remain in place: %v", err)
	}
}
