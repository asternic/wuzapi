package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"path/filepath"
    "go.mau.fi/whatsmeow/store/sqlstore"
    waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/gorilla/mux"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
    _ "github.com/mattn/go-sqlite3"
)

type server struct {
	db     *sql.DB
	router *mux.Router
	exPath string
}

var (
	address    = flag.String("address", "0.0.0.0", "Bind IP Address")
	port       = flag.String("port", "8080", "Listen Port")
	waDebug    = flag.String("wadebug", "", "Enable whatsmeow debug (INFO or DEBUG)")
	logType    = flag.String("logtype", "console", "Type of log output (console or json)")
	sslcert    = flag.String("sslcertificate", "", "SSL Certificate File")
	sslprivkey = flag.String("sslprivatekey", "", "SSL Certificate Private Key File")
	adminToken = flag.String("admintoken", "", "Security Token to authorize admin actions (list/create/remove users)")
    container *sqlstore.Container

	killchannel   = make(map[int](chan bool))
	userinfocache = cache.New(5*time.Minute, 10*time.Minute)
	log zerolog.Logger
)

func init() {

	flag.Parse()

	if(*logType=="json") {
        log = zerolog.New(os.Stdout).With().Timestamp().Str("role",filepath.Base(os.Args[0])).Logger()
    } else {
        output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
        log = zerolog.New(output).With().Timestamp().Str("role",filepath.Base(os.Args[0])).Logger()
    }

    if(*adminToken == "") {
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

    dbDirectory := exPath + "/dbdata"
	_, err = os.Stat(dbDirectory)
	if os.IsNotExist(err) {
		errDir := os.MkdirAll(dbDirectory, 0751)
		if errDir != nil {
			panic("Could not create dbdata directory")
		}
	}

	db, err := sql.Open("sqlite", exPath + "/dbdata/users.db")
	if err != nil {
		log.Fatal().Err(err).Msg("Could not open/create "+exPath+"/dbdata/users.db")
		os.Exit(1)
	}
	defer db.Close()

	sqlStmt := `CREATE TABLE IF NOT EXISTS users (id INTEGER NOT NULL PRIMARY KEY, name TEXT NOT NULL, token TEXT NOT NULL, webhook TEXT NOT NULL default "", jid TEXT NOT NULL default "", qrcode TEXT NOT NULL default "", connected INTEGER, expiration INTEGER, events TEXT NOT NULL default "All");`
	_, err = db.Exec(sqlStmt)
	if err != nil {
		panic(fmt.Sprintf("%q: %s\n", err, sqlStmt))
	}

	if(*waDebug!="") {
		dbLog := waLog.Stdout("Database", *waDebug, true)
		container, err = sqlstore.New("sqlite3", "file:"+exPath+"/dbdata/main.db?_foreign_keys=on&_busy_timeout=3000", dbLog)
	} else {
		container, err = sqlstore.New("sqlite3", "file:"+exPath+"/dbdata/main.db?_foreign_keys=on&_busy_timeout=3000", nil)
	}
	if err != nil {
		panic(err)
	}

	s := &server{
		router: mux.NewRouter(),
		db:     db,
		exPath: exPath,
	}
	s.routes()

	s.connectOnStartup()

	srv := &http.Server{
		Addr:    *address + ":" + *port,
		Handler: s.router,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if *sslcert != "" {
			if err := srv.ListenAndServeTLS(*sslcert, *sslprivkey); err != nil && err != http.ErrServerClosed {
				//log.Fatalf("listen: %s\n", err)
                log.Fatal().Err(err).Msg("Startup failed")
			}
		} else {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				//log.Fatalf("listen: %s\n", err)
                log.Fatal().Err(err).Msg("Startup failed")
			}
		}
	}()
    //wlog.Infof("Server Started. Listening on %s:%s", *address, *port)
    log.Info().Str("address", *address).Str("port",*port).Msg("Server Started")

	<-done
	log.Info().Msg("Server Stoped")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		// extra handling here
		cancel()
	}()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Str("error",fmt.Sprintf("%+v",err)).Msg("Server Shutdown Failed")
		os.Exit(1)
	}
	log.Info().Msg("Server Exited Properly")
}
