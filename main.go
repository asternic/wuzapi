package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
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

var (
	address     = flag.String("address", "0.0.0.0", "Bind IP Address")
	port        = flag.String("port", "8080", "Listen Port")
	waDebug     = flag.String("wadebug", "", "Enable whatsmeow debug (INFO or DEBUG)")
	logType     = flag.String("logtype", "console", "Type of log output (console or json)")
	colorOutput = flag.Bool("color", false, "Enable colored output for console logs")
	sslcert     = flag.String("sslcertificate", "", "SSL Certificate File")
	sslprivkey  = flag.String("sslprivatekey", "", "SSL Certificate Private Key File")
	adminToken  = flag.String("admintoken", "", "Security Token to authorize admin actions (list/create/remove users)")

	container     *sqlstore.Container
	killchannel   = make(map[int](chan bool))
	userinfocache = cache.New(5*time.Minute, 10*time.Minute)
)

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Warn().Err(err).Msg("Não foi possível carregar o arquivo .env (pode ser que não exista).")
	}

	flag.Parse()

	tz := os.Getenv("TZ")
	if tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			log.Warn().Err(err).Msgf("Não foi possível definir TZ=%q, usando UTC", tz)
		} else {
			time.Local = loc
			log.Info().Str("TZ", tz).Msg("Timezone definido pelo ambiente")
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

	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")

	dsn := fmt.Sprintf(
		"user=%s password=%s dbname=%s host=%s port=%s sslmode=disable",
		dbUser, dbPassword, dbName, dbHost, dbPort,
	)

	var db *sqlx.DB
	const maxAttempts = 10
	for i := 1; i <= maxAttempts; i++ {
		db, err = sqlx.Open("postgres", dsn)
		if err == nil {
			errPing := db.Ping()
			if errPing == nil {
				log.Info().Msgf("[DB] Conexão PostgreSQL estabelecida na tentativa %d", i)
				break
			}
			err = errPing
		}
		log.Warn().Msgf("[DB] Falha ao conectar (%d/%d): %v", i, maxAttempts, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatal().Err(err).Msgf("[DB] Não foi possível conectar ao PostgreSQL após %d tentativas", maxAttempts)
		os.Exit(1)
	}

	if err := runMigrations(db, exPath); err != nil {
		log.Fatal().Err(err).Msg("Falha ao executar migrações")
		os.Exit(1)
	}

	var dbLog waLog.Logger
	if *waDebug != "" {
		dbLog = waLog.Stdout("Database", *waDebug, *colorOutput)
	}
	container, err = sqlstore.New("postgres", dsn, dbLog)
	if err != nil {
		log.Fatal().Err(err).Msg("Falha ao criar container sqlstore")
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
				log.Fatal().Err(err).Msg("Falha ao iniciar o servidor HTTPS")
			}
		} else {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatal().Err(err).Msg("Falha ao iniciar o servidor HTTP")
			}
		}
	}()
	log.Info().Str("address", *address).Str("port", *port).Msg("Servidor iniciado. Aguardando conexões...")

	<-done
	log.Warn().Msg("Servidor parando...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Falha ao parar o servidor")
		os.Exit(1)
	}
	log.Info().Msg("Servidor saiu corretamente")
}

func runMigrations(db *sqlx.DB, exPath string) error {
	log.Info().Msg("Checando se já existe algum usuário...")

	var userCount int
	if err := db.Get(&userCount, "SELECT COUNT(*) FROM users;"); err == nil {
		if userCount > 0 {
			log.Info().Msgf("Usuário(s) encontrado(s): %d. Pulando migrações...", userCount)
			return nil
		}
		log.Warn().Msg("Nenhum usuário encontrado. Rodando migração e inserindo usuário padrão.")
		return applyMigrationsAndCreateUser(db, exPath)
	} else {
		log.Warn().Err(err).Msg("Erro consultando usuários (talvez a tabela nem exista). Rodando migração...")
		return applyMigrationsAndCreateUser(db, exPath)
	}
}

func applyMigrationsAndCreateUser(db *sqlx.DB, exPath string) error {
	log.Info().Msg("Executando migrações...")

	migFile := filepath.Join(exPath, "migrations", "0001_create_users_table.up.sql")
	sqlBytes, err := ioutil.ReadFile(migFile)
	if err != nil {
		return fmt.Errorf("falha ao ler arquivo de migração (%s): %w", migFile, err)
	}
	if _, err = db.Exec(string(sqlBytes)); err != nil {
		return fmt.Errorf("falha ao executar migração: %w", err)
	}
	log.Info().Msg("Migração executada com sucesso.")

	if _, err = db.Exec("INSERT INTO users (name, token) VALUES ($1, $2)", "John", "1234ABCD"); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			log.Warn().Msg("Usuário padrão já existe. Ignorando.")
			return nil
		}
		return fmt.Errorf("erro ao inserir usuário padrão: %w", err)
	}
	log.Info().Msg("Usuário padrão (John/1234ABCD) inserido com sucesso.")
	return nil
}
