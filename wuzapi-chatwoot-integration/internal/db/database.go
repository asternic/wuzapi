package db

import (
	"fmt"
	// "log" // Standard log no longer needed for GORM logger
	stlog "log" // Alias for standard log if still needed for GORM's logger.New

	"github.com/rs/zerolog/log" // Use zerolog's global logger
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// DB is the global database connection instance.
var DB *gorm.DB

// InitDB initializes the database connection using the provided DSN.
func InitDB(dsn string) error {
	if dsn == "" {
		return fmt.Errorf("database DSN cannot be empty")
	}

	// Configure GORM logger to use zerolog
	// GORM's logger.New expects a standard log.Logger instance.
	// We can create one that writes to zerolog, or use a simpler GORM logger config.
	// For simplicity, let's use GORM's default logger but adjust its level based on zerolog's level.
	var gormLogLevel gormlogger.LogLevel
	zerologLevel := log.Logger.GetLevel() // Get current global zerolog level
	switch zerologLevel {
	case gormlogger.Silent:
		gormLogLevel = gormlogger.Silent
	case gormlogger.Error:
		gormLogLevel = gormlogger.Error
	case gormlogger.Warn:
		gormLogLevel = gormlogger.Warn
	default: // Includes Info, Debug, Trace
		gormLogLevel = gormlogger.Info
	}

	newLogger := gormlogger.New(
		stlog.New(log.Logger, "", stlog.LstdFlags), // Use zerolog's global logger as the writer for GORM
		gormlogger.Config{
			SlowThreshold:             gormlogger.DefaultSlowThreshold, // Or configure as needed
			LogLevel:                  gormLogLevel,
			IgnoreRecordNotFoundError: true, // Or false based on preference
			Colorful:                  false, // Zerolog will handle coloring if its output is console
		},
	)

	var err error
	DB, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Info().Msg("Database connection established successfully.")
	return nil
}

// MigrateDB runs GORM's AutoMigrate for the defined models.
// It should be called after InitDB.
// The actual model types will be passed from main.go or another setup function
// to avoid direct dependency from db to models if models also need db.
func MigrateDB(modelsToMigrate ...interface{}) error {
	if DB == nil {
		return fmt.Errorf("database not initialized, call InitDB first")
	}

	if len(modelsToMigrate) == 0 {
		return fmt.Errorf("no models provided for migration")
	}

	err := DB.AutoMigrate(modelsToMigrate...)
	if err != nil {
		return fmt.Errorf("failed to auto-migrate database: %w", err)
	}

	log.Info().Int("models_migrated", len(modelsToMigrate)).Msg("Database migration completed successfully for provided models.")
	return nil
}
