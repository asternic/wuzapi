package main

import (
	"fmt"
	"net/http"
	"wuzapi-chatwoot-integration/config"
	"wuzapi-chatwoot-integration/internal/adapters/chatwoot"
	"wuzapi-chatwoot-integration/internal/adapters/wuzapi"
	"wuzapi-chatwoot-integration/internal/db"
	"wuzapi-chatwoot-integration/internal/handlers" // Import handlers package
	"wuzapi-chatwoot-integration/internal/models"
	"wuzapi-chatwoot-integration/internal/services" // Import services package
	"wuzapi-chatwoot-integration/pkg/logger" // For InitLogger
	"github.com/rs/zerolog/log"             // Import zerolog's global logger
)

func main() {
	logger.InitLogger() // Configures the global log.Logger

	log.Info().Msg("Loading configuration...")
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}
	log.Info().Msg("Configuration loaded successfully.")
	// log.Debug().Interface("config", cfg).Msg("Loaded configuration values") // For debugging

	// Initialize Database
	log.Info().Str("database_url", cfg.DatabaseURL).Msg("Initializing database...") // Log DSN safely
	if err := db.InitDB(cfg.DatabaseURL); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	// db.InitDB now logs its own success message, so no need for: log.Info().Msg("Database initialized successfully.")

	// Run Migrations
	log.Info().Msg("Running database migrations...")
	if err := db.MigrateDB(&models.ConversationMap{}, &models.QueuedMessage{}); err != nil {
		log.Fatal().Err(err).Msg("Failed to run database migrations")
	}
	// db.MigrateDB now logs its own success message, so no need for: log.Info().Msg("Database migrations completed successfully.")

	// Initialize Wuzapi Client
	wClient, err := wuzapi.NewClient(cfg.WuzapiBaseURL, cfg.WuzapiAPIKey, cfg.WuzapiInstanceID)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize Wuzapi client")
	} else {
		// Wuzapi NewClient is expected to log its own successful initialization message
		// _ = wClient // No longer assign to blank if used by services
	}

	// Initialize Chatwoot Client
	cClient, err := chatwoot.NewClient(cfg.ChatwootBaseURL, cfg.ChatwootAccessToken, cfg.ChatwootAccountID, cfg.ChatwootInboxID)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize Chatwoot client")
	} else {
		// Chatwoot NewClient is expected to log its own successful initialization message
		// _ = cClient // No longer assigning to blank if used
	}

	// Initialize Services
	contactService, err := services.NewContactSyncService(cClient, cfg.ChatwootInboxID)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize ContactSyncService")
	}
	log.Info().Msg("ContactSyncService initialized successfully")

	conversationService, err := services.NewConversationSyncService(cClient, db.DB, cfg.ChatwootInboxID)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize ConversationSyncService")
	}
	log.Info().Msg("ConversationSyncService initialized successfully")

	messageService, err := services.NewMessageSyncService(wClient, cClient, db.DB) // Pass wClient now
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize MessageSyncService")
	}
	log.Info().Msg("MessageSyncService initialized successfully")

	// The initialized clients (wClient, cClient), services (contactService, conversationService, messageService), and db.DB
	// can now be passed to handlers, etc., as needed.


	// Initialize Handlers
	// The WuzapiHandler now takes dependencies.
	wuzapiHandler := handlers.NewWuzapiHandler(contactService, conversationService, messageService, cfg.WebhookSecret)


	// Setup HTTP routes
	// TODO: Consider using a router like gorilla/mux for more complex routing
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// This is a handler, logging here would be request-specific.
		// For now, simple response is fine.
		fmt.Fprintln(w, "Welcome to Wuzapi-Chatwoot Integration! API Server is running.")
	})
	http.HandleFunc(cfg.WuzapiWebhookPath, wuzapiHandler.Handle) // Use the Handle method of the struct instance
	log.Info().Str("path", cfg.WuzapiWebhookPath).Msg("Registered Wuzapi webhook handler")


	port := cfg.Port
	if port == "" {
		port = "8080" // Default port
		log.Info().Str("port", port).Msg("Defaulting to port")
	}

	log.Info().Str("port", port).Msgf("Server starting on port %s...", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}
