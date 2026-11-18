package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log" // zerolog's global logger
)

// InitLogger initializes zerolog's global logger instance.
// It allows for console or JSON output based on the LOG_FORMAT environment variable.
// Log level is also configured via LOG_LEVEL environment variable.
func InitLogger() {
	logFormat := os.Getenv("LOG_FORMAT")
	logLevelStr := os.Getenv("LOG_LEVEL")

	var level zerolog.Level
	switch logLevelStr {
	case "debug":
		level = zerolog.DebugLevel
	case "info":
		level = zerolog.InfoLevel
	case "warn":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	case "fatal":
		level = zerolog.FatalLevel
	case "panic":
		level = zerolog.PanicLevel
	default:
		level = zerolog.InfoLevel // Default to info level
	}

	zerolog.SetGlobalLevel(level)

	if logFormat != "json" { // Default to console if not "json"
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	}
	// If logFormat is "json", zerolog's default is JSON output to os.Stderr, so no change needed for log.Logger itself.
	// We just need to ensure TimeFormat and other global settings are applied if necessary.
	// zerolog.TimeFieldFormat = zerolog.TimeFormatUnix // Example: if you want Unix timestamps for JSON

	// Log the initialization event using the configured global logger
	log.Info().Str("logFormat", logFormat).Str("logLevel", level.String()).Msg("Logger initialized")
}

// GetLogger returns the configured global zerolog logger.
// This function is provided for convenience if direct access to log.Logger is not preferred elsewhere.
func GetLogger() zerolog.Logger {
	return log.Logger
}
