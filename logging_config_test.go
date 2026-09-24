package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func preserveLogLevel(t *testing.T) {
	t.Helper()
	old := zerolog.GlobalLevel()
	t.Cleanup(func() { zerolog.SetGlobalLevel(old) })
}
func TestConfigureLogLevel(t *testing.T) {
	preserveLogLevel(t)
	for _, tc := range []struct {
		value string
		want  zerolog.Level
	}{
		{"trace", zerolog.TraceLevel}, {"debug", zerolog.DebugLevel}, {"info", zerolog.InfoLevel},
		{" WARN ", zerolog.WarnLevel}, {"error", zerolog.ErrorLevel}, {"fatal", zerolog.FatalLevel}, {"panic", zerolog.PanicLevel},
	} {
		t.Run(tc.value, func(t *testing.T) {
			zerolog.SetGlobalLevel(zerolog.TraceLevel)
			configureLogLevel(tc.value)
			if got := zerolog.GlobalLevel(); got != tc.want {
				t.Fatalf("level=%v want %v", got, tc.want)
			}
		})
	}
	for _, invalid := range []string{"", " ", "warning", "disabled", "7", "99", "-1", "6", "128"} {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		configureLogLevel(invalid)
		if zerolog.GlobalLevel() != zerolog.DebugLevel {
			t.Errorf("invalid %q changed level", invalid)
		}
	}
}
func TestLoadEnvAndConfigureLogging(t *testing.T) {
	for _, tc := range []struct {
		name, env, file string
		exported        bool
		want            zerolog.Level
	}{
		{name: "dotenv", file: "LOG_LEVEL=warn\n", want: zerolog.WarnLevel},
		{name: "process overrides dotenv", env: "error", exported: true, file: "LOG_LEVEL=debug\n", want: zerolog.ErrorLevel},
		{name: "explicit empty environment", exported: true, file: "LOG_LEVEL=error\n", want: zerolog.TraceLevel},
		{name: "unset", file: "# empty\n", want: zerolog.TraceLevel},
		{name: "invalid dotenv", file: "LOG_LEVEL=99\n", want: zerolog.TraceLevel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preserveLogLevel(t)
			zerolog.SetGlobalLevel(zerolog.TraceLevel)
			t.Setenv("LOG_LEVEL", tc.env)
			if !tc.exported {
				if err := os.Unsetenv("LOG_LEVEL"); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, []byte(tc.file), 0600); err != nil {
				t.Fatal(err)
			}
			if err := loadEnvAndConfigureLogging(path); err != nil {
				t.Fatal(err)
			}
			if got := zerolog.GlobalLevel(); got != tc.want {
				t.Fatalf("level=%v want %v", got, tc.want)
			}
		})
	}
}
func TestMissingDotenvStillFiltersStartupLogs(t *testing.T) {
	preserveLogLevel(t)
	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	t.Setenv("LOG_LEVEL", "error")
	var out bytes.Buffer
	logger := zerolog.New(&out)
	if err := loadEnvAndConfigureLogging(filepath.Join(t.TempDir(), "missing")); !os.IsNotExist(err) {
		t.Fatalf("expected missing dotenv: %v", err)
	}
	logger.Debug().Msg("hidden debug")
	logger.Info().Msg("hidden info")
	logger.Warn().Msg("hidden dotenv warning")
	logger.Error().Msg("visible error")
	if strings.Contains(out.String(), "hidden") || !strings.Contains(out.String(), "visible error") {
		t.Fatalf("unexpected logs: %s", out.String())
	}
}
