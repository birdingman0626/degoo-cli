package logger_test

import (
	"os"
	"strings"
	"testing"

	"degoo-cli/internal/logger"
)

func TestLoggerWritesToFile(t *testing.T) {
	f, _ := os.CreateTemp("", "degoo-log-*.log")
	f.Close()
	defer os.Remove(f.Name())

	log, err := logger.New(f.Name())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = log.Close() }()

	log.Info("hello %s", "world")

	data, _ := os.ReadFile(f.Name())
	if !strings.Contains(string(data), "[INFO] hello world") {
		t.Errorf("expected log file to contain '[INFO] hello world', got: %s", data)
	}
}

func TestLoggerLevels(t *testing.T) {
	f, _ := os.CreateTemp("", "degoo-log-*.log")
	f.Close()
	defer os.Remove(f.Name())

	log, _ := logger.New(f.Name())
	defer func() { _ = log.Close() }()

	log.Info("info msg")
	log.Warn("warn msg")
	log.Error("error msg")

	data, _ := os.ReadFile(f.Name())
	content := string(data)
	for _, want := range []string{"[INFO] info msg", "[WARN] warn msg", "[ERROR] error msg"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in log output:\n%s", want, content)
		}
	}
}
