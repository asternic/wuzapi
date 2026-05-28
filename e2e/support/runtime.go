package support

import (
	"fmt"
	"net"
	"path/filepath"
	"runtime"
)

func repoRoot() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to resolve the repository path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..")), nil
}

func freePort() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("failed to reserve a local port for the e2e HTTP server: %w", err)
	}
	defer listener.Close()

	return fmt.Sprintf("%d", listener.Addr().(*net.TCPAddr).Port), nil
}
