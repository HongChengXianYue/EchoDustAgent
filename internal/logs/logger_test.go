package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerWritesFile(t *testing.T) {
	logger, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer logger.Close()

	logger.Errorf("hello %s", "world")

	data, err := os.ReadFile(filepath.Join(filepath.Dir(logger.Path()), "agent.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "hello world") {
		t.Fatalf("log content = %q, want hello world", string(data))
	}
}
