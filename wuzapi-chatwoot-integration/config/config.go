package config

import (
	// "fmt" // No longer needed
	"os"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log" // Use global logger
)

// Config holds all configuration fields for the application.
type Config struct {
	WuzapiBaseURL             string
	WuzapiAPIKey              string
	WuzapiInstanceID          string
	WuzapiWebhookURLChatwoot  string
	ChatwootBaseURL           string
	ChatwootAccessToken       string
	ChatwootAccountID         string
	ChatwootInboxID           string
	WebhookSecret             string
	RedisURL                  string
	DatabaseURL               string
	Port                      string
	LogLevel                  string
	LogFormat                 string // Added to control log format (e.g., "console" or "json")
	WuzapiWebhookPath         string // Path for incoming Wuzapi webhooks
}

// LoadConfig loads configuration from environment variables.
// It attempts to load a .env file if present.
func LoadConfig() (*Config, error) {
	// Attempt to load .env file, but don't fail if it's not present.
	// Environment variables will take precedence.
	err := godotenv.Load()
	if err != nil {
		log.Info().Err(err).Msg("No .env file found or error loading it, relying on environment variables")
	} else {
		log.Info().Msg("Loaded configuration from .env file (if present)")
	}

	log.Info().Msg("Loading configuration from environment variables...")

	cfg := &Config{
		WuzapiBaseURL:            os.Getenv("WUZAPI_BASE_URL"),
		WuzapiAPIKey:             os.Getenv("WUZAPI_API_KEY"),
		WuzapiInstanceID:         os.Getenv("WUZAPI_INSTANCE_ID"),
		WuzapiWebhookURLChatwoot: os.Getenv("WUZAPI_WEBHOOK_URL_CHATWOOT"),
		ChatwootBaseURL:          os.Getenv("CHATWOOT_BASE_URL"),
		ChatwootAccessToken:      os.Getenv("CHATWOOT_ACCESS_TOKEN"),
		ChatwootAccountID:        os.Getenv("CHATWOOT_ACCOUNT_ID"),
		ChatwootInboxID:          os.Getenv("CHATWOOT_INBOX_ID"),
		WebhookSecret:            os.Getenv("WEBHOOK_SECRET"),
		RedisURL:                 os.Getenv("REDIS_URL"),
		DatabaseURL:              os.Getenv("DATABASE_URL"),
		Port:                     os.Getenv("PORT"),
		LogLevel:                 os.Getenv("LOG_LEVEL"),
		LogFormat:                os.Getenv("LOG_FORMAT"),
		WuzapiWebhookPath:        os.Getenv("WUZAPI_WEBHOOK_PATH"),
	}

	if cfg.WuzapiWebhookPath == "" {
		cfg.WuzapiWebhookPath = "/webhooks/wuzapi" // Default path
		log.Info().Str("path", cfg.WuzapiWebhookPath).Msg("WUZAPI_WEBHOOK_PATH not set, using default")
	}

	// In a real application, you would validate these values.
	// For debugging, you might log these, but be careful with sensitive data.
	// Example: log.Debug().Str("wuzapi_base_url", cfg.WuzapiBaseURL).Msg("Config value")
	// Omitting individual value logging here for brevity and security.

	log.Info().Msg("Configuration loading attempt complete.")
	return cfg, nil
}
