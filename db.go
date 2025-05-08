package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

type DatabaseConfig struct {
	Type     string
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	Path     string
}

func InitializeDatabase(exPath string) (*sqlx.DB, error) {
	config := getDatabaseConfig(exPath)

	if config.Type == "postgres" {
		return initializePostgres(config)
	}
	return initializeSQLite(config)
}

func getDatabaseConfig(exPath string) DatabaseConfig {
	// Check for PostgreSQL configuration
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")

	// If all PostgreSQL configs are present, use PostgreSQL
	if dbUser != "" && dbPassword != "" && dbName != "" && dbHost != "" && dbPort != "" {
		return DatabaseConfig{
			Type:     "postgres",
			Host:     dbHost,
			Port:     dbPort,
			User:     dbUser,
			Password: dbPassword,
			Name:     dbName,
		}
	}

	// Default to SQLite
	return DatabaseConfig{
		Type: "sqlite",
		Path: filepath.Join(exPath, "dbdata"),
	}
}

func initializePostgres(config DatabaseConfig) (*sqlx.DB, error) {
	dsn := fmt.Sprintf(
		"user=%s password=%s dbname=%s host=%s port=%s sslmode=disable",
		config.User, config.Password, config.Name, config.Host, config.Port,
	)

	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping postgres database: %w", err)
	}

	return db, nil
}

func initializeSQLite(config DatabaseConfig) (*sqlx.DB, error) {
	// Ensure dbdata directory exists
	if err := os.MkdirAll(config.Path, 0751); err != nil {
		return nil, fmt.Errorf("could not create dbdata directory: %w", err)
	}

	dbPath := filepath.Join(config.Path, "users.db")
	db, err := sqlx.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_busy_timeout=3000")
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	return db, nil
}
