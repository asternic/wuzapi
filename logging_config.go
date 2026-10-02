package main

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

// Load preserves existing process variables. Return its error so main can log
// it after applying the severity filter, including when .env is missing.
func loadEnvAndConfigureLogging(filenames ...string) error {
	err := godotenv.Load(filenames...)
	configureLogLevel(os.Getenv("LOG_LEVEL"))
	return err
}

func configureLogLevel(value string) {
	value = strings.ToLower(strings.TrimSpace(value))
	// ParseLevel also accepts arbitrary int8 values and "disabled". Only allow
	// documented names so invalid configuration cannot silence the service.
	switch value {
	case "trace", "debug", "info", "warn", "error", "fatal", "panic":
		level, _ := zerolog.ParseLevel(value)
		zerolog.SetGlobalLevel(level)
	}
}
