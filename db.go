package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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
	SSLMode  string
}

func InitializeDatabase(exPath string) (*sqlx.DB, error) {
	config := getDatabaseConfig(exPath)

	if config.Type == "postgres" {
		return initializePostgres(config)
	}
	return initializeSQLite(config)
}

func getDatabaseConfig(exPath string) DatabaseConfig {
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbSSL := os.Getenv("DB_SSLMODE")

	sslMode := dbSSL
	if dbSSL == "true" {
		sslMode = "require"
	} else if dbSSL == "false" || dbSSL == "" {
		sslMode = "disable"
	}

	if dbUser != "" && dbPassword != "" && dbName != "" && dbHost != "" && dbPort != "" {
		return DatabaseConfig{
			Type:     "postgres",
			Host:     dbHost,
			Port:     dbPort,
			User:     dbUser,
			Password: dbPassword,
			Name:     dbName,
			SSLMode:  sslMode,
		}
	}

	return DatabaseConfig{
		Type: "sqlite",
		Path: filepath.Join(exPath, "dbdata"),
	}
}

func initializePostgres(config DatabaseConfig) (*sqlx.DB, error) {
	dsn := fmt.Sprintf(
		"user=%s password=%s dbname=%s host=%s port=%s sslmode=%s",
		config.User, config.Password, config.Name, config.Host, config.Port, config.SSLMode,
	)

	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		if createErr := createPostgresDatabase(config); createErr != nil {
			return nil, fmt.Errorf("failed to ping postgres database and could not create it: ping error: %w, create error: %v", err, createErr)
		}

		db, err = sqlx.Open("postgres", dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open postgres connection after creating database: %w", err)
		}

		if err := db.Ping(); err != nil {
			return nil, fmt.Errorf("failed to ping postgres database after creating it: %w", err)
		}
	}

	return db, nil
}

func createPostgresDatabase(config DatabaseConfig) error {
	systemDSN := fmt.Sprintf(
		"user=%s password=%s dbname=postgres host=%s port=%s sslmode=%s",
		config.User, config.Password, config.Host, config.Port, config.SSLMode,
	)

	systemDB, err := sqlx.Open("postgres", systemDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to system postgres database: %w", err)
	}
	defer systemDB.Close()

	if err := systemDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping system postgres database: %w", err)
	}

	var exists bool
	checkQuery := "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)"
	if err := systemDB.Get(&exists, checkQuery, config.Name); err != nil {
		return fmt.Errorf("failed to check if database exists: %w", err)
	}

	if exists {
		return nil
	}

	if !isValidDatabaseName(config.Name) {
		return fmt.Errorf("invalid database name: %s", config.Name)
	}

	createQuery := fmt.Sprintf("CREATE DATABASE %s", config.Name)
	if _, err := systemDB.Exec(createQuery); err != nil {
		return fmt.Errorf("failed to create database '%s': %w", config.Name, err)
	}

	fmt.Printf("Database '%s' created successfully\n", config.Name)
	return nil
}

func isValidDatabaseName(name string) bool {
	if len(name) == 0 || len(name) > 63 {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_') {
				return false
			}
		} else {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
				return false
			}
		}
	}
	return true
}

func initializeSQLite(config DatabaseConfig) (*sqlx.DB, error) {
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

type HistoryMessage struct {
	ID              int       `json:"id" db:"id"`
	UserID          string    `json:"user_id" db:"user_id"`
	ChatJID         string    `json:"chat_jid" db:"chat_jid"`
	SenderJID       string    `json:"sender_jid" db:"sender_jid"`
	MessageID       string    `json:"message_id" db:"message_id"`
	Timestamp       time.Time `json:"timestamp" db:"timestamp"`
	MessageType     string    `json:"message_type" db:"message_type"`
	TextContent     string    `json:"text_content" db:"text_content"`
	MediaLink       string    `json:"media_link" db:"media_link"`
	QuotedMessageID string    `json:"quoted_message_id,omitempty" db:"quoted_message_id"`
}

func (s *server) saveMessageToHistory(userID, chatJID, senderJID, messageID, messageType, textContent, mediaLink, quotedMessageID string) error {
	query := `INSERT INTO message_history (user_id, chat_jid, sender_jid, message_id, timestamp, message_type, text_content, media_link, quoted_message_id)
              VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	if s.db.DriverName() == "sqlite" {
		query = `INSERT INTO message_history (user_id, chat_jid, sender_jid, message_id, timestamp, message_type, text_content, media_link, quoted_message_id)
                 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	}
	_, err := s.db.Exec(query, userID, chatJID, senderJID, messageID, time.Now(), messageType, textContent, mediaLink, quotedMessageID)
	if err != nil {
		return fmt.Errorf("failed to save message to history: %w", err)
	}
	return nil
}

func (s *server) trimMessageHistory(userID, chatJID string, limit int) error {
	var query string
	if s.db.DriverName() == "postgres" {
		query = `
            DELETE FROM message_history
            WHERE id IN (
                SELECT id FROM message_history
                WHERE user_id = $1 AND chat_jid = $2
                ORDER BY timestamp DESC
                OFFSET $3
            )`
	} else { // sqlite
		query = `
            DELETE FROM message_history
            WHERE id IN (
                SELECT id FROM message_history
                WHERE user_id = ? AND chat_jid = ?
                ORDER BY timestamp DESC
                LIMIT -1 OFFSET ?
            )`
	}

	_, err := s.db.Exec(query, userID, chatJID, limit)
	if err != nil {
		return fmt.Errorf("failed to trim message history: %w", err)
	}
	return nil
}
