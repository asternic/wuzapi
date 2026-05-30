package support

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServerStopRemovesRuntimeDir(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(runtimeDir, 0751); err != nil {
		t.Fatalf("failed to create runtime dir: %v", err)
	}

	logFile, err := os.Create(filepath.Join(runtimeDir, "http.log"))
	if err != nil {
		t.Fatalf("failed to create log file: %v", err)
	}

	server := &Server{
		RuntimeDir: runtimeDir,
		logFile:    logFile,
	}
	server.Stop()

	if _, err := os.Stat(runtimeDir); !os.IsNotExist(err) {
		t.Fatalf("expected runtime dir to be removed, got err=%v", err)
	}
}
