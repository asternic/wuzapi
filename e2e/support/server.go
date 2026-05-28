package support

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	DefaultAdminToken = "wuzapi-e2e-admin-token"
)

type Server struct {
	BaseURL    string
	AdminToken string

	cancel  context.CancelFunc
	command *exec.Cmd
	logFile *os.File
	wait    chan error
}

func StartServer(ctx context.Context) (*Server, error) {
	repoRoot, err := repoRoot()
	if err != nil {
		return nil, err
	}

	port, err := freePort()
	if err != nil {
		return nil, err
	}

	if configuredPort := os.Getenv("E2E_WUZAPI_PORT"); configuredPort != "" {
		port = configuredPort
	}

	adminToken := os.Getenv("E2E_WUZAPI_ADMIN_TOKEN")
	if adminToken == "" {
		adminToken = DefaultAdminToken
	}

	runtimeDir := filepath.Join(repoRoot, "e2e", ".tmp", fmt.Sprintf("server-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(runtimeDir, 0751); err != nil {
		return nil, fmt.Errorf("failed to create the e2e temporary directory: %w", err)
	}

	logFile, err := os.Create(filepath.Join(runtimeDir, "http.log"))
	if err != nil {
		return nil, fmt.Errorf("failed to create the e2e HTTP log: %w", err)
	}

	commandCtx, cancel := context.WithCancel(ctx)
	command := exec.CommandContext(commandCtx, "go", "run", ".", "-address", "127.0.0.1", "-port", port, "-admintoken", adminToken, "-datadir", runtimeDir, "-logtype", "json", "-color=false")
	command.Dir = repoRoot
	command.Env = isolatedEnvironment(adminToken)
	command.Stdout = logFile
	command.Stderr = logFile

	if err := command.Start(); err != nil {
		cancel()
		_ = logFile.Close()
		return nil, fmt.Errorf("failed to start the project HTTP server for e2e: %w", err)
	}

	server := &Server{
		BaseURL:    "http://127.0.0.1:" + port,
		AdminToken: adminToken,
		cancel:     cancel,
		command:    command,
		logFile:    logFile,
		wait:       make(chan error, 1),
	}

	go func() {
		server.wait <- command.Wait()
	}()

	if err := server.waitUntilHealthy(30 * time.Second); err != nil {
		server.Stop()
		return nil, err
	}

	os.Setenv("E2E_WUZAPI_BASE_URL", server.BaseURL)
	os.Setenv("E2E_WUZAPI_ADMIN_TOKEN", server.AdminToken)

	return server, nil
}

func (server *Server) Stop() {
	if server == nil {
		return
	}

	if server.cancel != nil {
		server.cancel()
	}

	if server.command != nil && server.command.Process != nil {
		select {
		case <-server.wait:
		case <-time.After(5 * time.Second):
			_ = server.command.Process.Kill()
			<-server.wait
		}
	}

	if server.logFile != nil {
		_ = server.logFile.Close()
	}
}

func (server *Server) waitUntilHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := http.Client{Timeout: 500 * time.Millisecond}
	var lastErr error

	for time.Now().Before(deadline) {
		select {
		case err := <-server.wait:
			return fmt.Errorf("the project HTTP server exited before becoming ready: %w", err)
		default:
		}

		response, err := client.Get(server.BaseURL + "/health")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode <= 299 {
				return nil
			}
			lastErr = fmt.Errorf("health returned HTTP %d", response.StatusCode)
		} else {
			lastErr = err
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("the project HTTP server was not ready within %s: %w", timeout, lastErr)
}
