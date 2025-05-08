package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type server struct {
	db     *sqlx.DB
	router *mux.Router
	exPath string
}

// Replace the global variables
var (
	address     = flag.String("address", "0.0.0.0", "Bind IP Address")
	port        = flag.String("port", "8080", "Listen Port")
	waDebug     = flag.String("wadebug", "", "Enable whatsmeow debug (INFO or DEBUG)")
	logType     = flag.String("logtype", "console", "Type of log output (console or json)")
	skipMedia   = flag.Bool("skipmedia", false, "Do not attempt to download media in messages)")
	osName      = flag.String("osname", "Mac OS 10", "Connection OSName in Whatsapp")
	colorOutput = flag.Bool("color", false, "Enable colored output for console logs")
	sslcert     = flag.String("sslcertificate", "", "SSL Certificate File")
	sslprivkey  = flag.String("sslprivatekey", "", "SSL Certificate Private Key File")
	adminToken  = flag.String("admintoken", "", "Security Token to authorize admin actions (list/create/remove users)")

	container     *sqlstore.Container
	clientManager = NewClientManager()
	killchannel   = make(map[int](chan bool))
	userinfocache = cache.New(5*time.Minute, 10*time.Minute)
)

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Warn().Err(err).Msg("It was not possible to load the .env file (it may not exist).")
	}

	flag.Parse()

	tz := os.Getenv("TZ")
	if tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			log.Warn().Err(err).Msgf("It was not possible to define TZ=%q, using UTC", tz)
		} else {
			time.Local = loc
			log.Info().Str("TZ", tz).Msg("Timezone defined")
		}
	}

	if *logType == "json" {
		log.Logger = zerolog.New(os.Stdout).
			With().
			Timestamp().
			Str("role", filepath.Base(os.Args[0])).
			Logger()
	} else {
		output := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "2006-01-02 15:04:05 -07:00",
			NoColor:    !*colorOutput,
		}

		output.FormatLevel = func(i interface{}) string {
			if i == nil {
				return ""
			}
			lvl := strings.ToUpper(i.(string))
			switch lvl {
			case "DEBUG":
				return "\x1b[34m" + lvl + "\x1b[0m"
			case "INFO":
				return "\x1b[32m" + lvl + "\x1b[0m"
			case "WARN":
				return "\x1b[33m" + lvl + "\x1b[0m"
			case "ERROR", "FATAL", "PANIC":
				return "\x1b[31m" + lvl + "\x1b[0m"
			default:
				return lvl
			}
		}

		log.Logger = zerolog.New(output).
			With().
			Timestamp().
			Str("role", filepath.Base(os.Args[0])).
			Logger()
	}

	if *adminToken == "" {
		if v := os.Getenv("WUZAPI_ADMIN_TOKEN"); v != "" {
			*adminToken = v
		}
	}
}

func main() {
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)

	db, err := InitializeDatabase(exPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
		os.Exit(1)
	}
	defer db.Close()

	// Initialize the schema
	if err = initializeSchema(db); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize schema")
		os.Exit(1)
	}

	var dbLog waLog.Logger
	if *waDebug != "" {
		dbLog = waLog.Stdout("Database", *waDebug, *colorOutput)
	}

	// Get database configuration
	config := getDatabaseConfig(exPath)
	var storeConnStr string
	if config.Type == "postgres" {
		storeConnStr = fmt.Sprintf(
			"user=%s password=%s dbname=%s host=%s port=%s sslmode=disable",
			config.User, config.Password, config.Name, config.Host, config.Port,
		)
		container, err = sqlstore.New("postgres", storeConnStr, dbLog)
	} else {
		storeConnStr = "file:" + filepath.Join(config.Path, "main.db") + "?_pragma=foreign_keys(1)&_busy_timeout=3000"
		container, err = sqlstore.New("sqlite", storeConnStr, dbLog)
	}

	if err != nil {
		log.Fatal().Err(err).Msg("Error creating sqlstore")
		os.Exit(1)
	}

	s := &server{
		router: mux.NewRouter(),
		db:     db,
		exPath: exPath,
	}
	s.routes()

	s.connectOnStartup()

	srv := &http.Server{
		Addr:              *address + ":" + *port,
		Handler:           s.router,
		ReadHeaderTimeout: 20 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       180 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if *sslcert != "" {
			if err := srv.ListenAndServeTLS(*sslcert, *sslprivkey); err != nil && err != http.ErrServerClosed {
				log.Fatal().Err(err).Msg("HTTPS server failed to start")
			}
		} else {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatal().Err(err).Msg("HTTP server failed to start")
			}
		}
	}()
	log.Info().Str("address", *address).Str("port", *port).Msg("Server started. Waiting for connections...")

	<-done
	log.Warn().Msg("Stopping server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to stop server")
		os.Exit(1)
	}
	log.Info().Msg("Server Exited Properly")
}

func initializeSchema(db *sqlx.DB) error {
	// First, check if the table exists
	var exists bool
	var err error

	// Detect the database driver
	driverName := db.DriverName()

	if driverName == "postgres" {
		err = db.Get(&exists, `
            SELECT EXISTS (
                SELECT 1
                FROM information_schema.tables
                WHERE table_name = 'users'
            );`)
	} else if driverName == "sqlite" {
		err = db.Get(&exists, `
            SELECT EXISTS (
                SELECT 1
                FROM sqlite_master
                WHERE type='table' AND name='users'
            );`)
	} else {
		return fmt.Errorf("unsupported database driver: %s", driverName)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to check if users table exists")
		return err
	}

	if exists {
		log.Info().Msg("Users table already exists")
		return nil
	}
	// Create table statement that works with both PostgreSQL and SQLite
	var sqlStmt string
	if driverName == "postgres" {
		sqlStmt = `CREATE TABLE users (
            id SERIAL PRIMARY KEY,
            name TEXT NOT NULL,
            token TEXT NOT NULL,
            webhook TEXT NOT NULL DEFAULT '',
            jid TEXT NOT NULL DEFAULT '',
            qrcode TEXT NOT NULL DEFAULT '',
            connected INTEGER,
            expiration INTEGER,
            events TEXT NOT NULL DEFAULT 'All',
            proxy_url TEXT DEFAULT ''
        );`
	} else {
		// SQLite version
		sqlStmt = `CREATE TABLE users (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL,
            token TEXT NOT NULL,
            webhook TEXT NOT NULL DEFAULT '',
            jid TEXT NOT NULL DEFAULT '',
            qrcode TEXT NOT NULL DEFAULT '',
            connected INTEGER,
            expiration INTEGER,
            events TEXT NOT NULL DEFAULT 'All',
            proxy_url TEXT DEFAULT ''
        );`
	}

	_, err = db.Exec(sqlStmt)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create users table")
		return err
	}

	log.Info().Msg("Successfully created users table")
	return nil
}
