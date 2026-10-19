package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow/store/sqlstore"
)

// These tests use real, independent PostgreSQL transactions instead of the
// process-local mutex. Point WUZAPI_TEST_POSTGRES_DSN at a test database; each
// test creates and drops its own schema and never modifies existing tables.
func makeProxyPoolPostgresServer(t *testing.T) (*server, *sqlx.DB, int) {
	t.Helper()
	dsn := os.Getenv("WUZAPI_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set WUZAPI_TEST_POSTGRES_DSN to run PostgreSQL concurrency regressions")
	}
	admin, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	id, err := GenerateRandomID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "proxy_pool_test_" + id
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
	})
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		query := parsed.Query()
		query.Set("search_path", schema)
		query.Set("statement_timeout", "5000")
		parsed.RawQuery = query.Encode()
		dsn = parsed.String()
	} else {
		dsn += " search_path=" + schema + " statement_timeout=5000"
	}
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	if err := sqlstore.NewWithDB(db.DB, "postgres", nil).Upgrade(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := initializeSchema(db); err != nil {
		t.Fatal(err)
	}
	control, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { control.Close() })
	var pid int
	if err := db.Get(&pid, "SELECT pg_backend_pid()"); err != nil {
		t.Fatal(err)
	}
	return &server{db: db}, control, pid
}

func waitForProxyPoolLock(t *testing.T, control *sqlx.DB, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		if err := control.Get(&waiting, "SELECT COALESCE(wait_event_type = 'Lock', false) FROM pg_stat_activity WHERE pid = $1", pid); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("operation did not reach the expected database lock")
}

func TestProxyPoolPostgresAssignmentPreservesConcurrentExplicitProxy(t *testing.T) {
	s, control, pid := makeProxyPoolPostgresServer(t)
	insertPoolTestUser(t, s, "u1", "")
	insertPoolTestEntry(t, s, "p1", "http://pool.example:8080", 1, true)
	tx := control.MustBegin()
	defer tx.Rollback()
	tx.MustExec("SELECT id FROM proxy_pool FOR UPDATE")
	type result struct {
		proxy string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		proxy, err := s.assignProxyFromPool("u1")
		done <- result{proxy, err}
	}()
	waitForProxyPoolLock(t, control, pid)
	// Another process saves an explicit proxy while assignment waits on the
	// pool. The old implementation had already read the empty user proxy.
	const explicit = "http://explicit.example:8080"
	tx.MustExec("UPDATE users SET proxy_url = $1, proxy_pool_id = NULL WHERE id = 'u1'", explicit)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got := <-done
	if got.err != nil || got.proxy != explicit {
		t.Fatalf("assignment overwrote explicit proxy: %+v", got)
	}
	if proxy, pool := userPoolState(t, s, "u1"); proxy != explicit || pool != "" {
		t.Fatalf("unexpected persisted state: proxy=%q pool=%q", proxy, pool)
	}
}

func TestProxyPoolPostgresEditRechecksCapacityAfterLock(t *testing.T) {
	s, control, pid := makeProxyPoolPostgresServer(t)
	insertPoolTestEntry(t, s, "p1", "http://pool.example:8080", 2, true)
	insertPoolTestUser(t, s, "u1", "")
	insertPoolTestUser(t, s, "u2", "")
	if _, err := s.assignProxyFromPool("u1"); err != nil {
		t.Fatal(err)
	}
	tx := control.MustBegin()
	defer tx.Rollback()
	tx.MustExec("SELECT id FROM proxy_pool FOR UPDATE")
	req := httptest.NewRequest(http.MethodPut, "/admin/proxy-pool/p1", strings.NewReader(`{"max_devices":1}`))
	req = mux.SetURLVars(req, map[string]string{"id": "p1"})
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); s.EditProxyPoolEntry()(rec, req) }()
	waitForProxyPoolLock(t, control, pid)
	tx.MustExec("UPDATE users SET proxy_url = 'http://pool.example:8080', proxy_pool_id = 'p1' WHERE id = 'u2'")
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	<-done
	if rec.Code != http.StatusConflict {
		t.Fatalf("shrink accepted stale count: %d %s", rec.Code, rec.Body.String())
	}
	entry, err := s.getProxyPoolEntry("p1")
	if err != nil || entry.MaxDevices != 2 || entry.AssignedCount != 2 {
		t.Fatalf("unexpected entry: %+v, %v", entry, err)
	}
}

func TestProxyPoolPostgresPartialEditPreservesConcurrentURLChange(t *testing.T) {
	s, control, pid := makeProxyPoolPostgresServer(t)
	insertPoolTestEntry(t, s, "p1", "http://old.example:8080", 2, true)
	tx := control.MustBegin()
	defer tx.Rollback()
	tx.MustExec("SELECT id FROM proxy_pool FOR UPDATE")
	req := httptest.NewRequest(http.MethodPut, "/admin/proxy-pool/p1", strings.NewReader(`{"label":"renamed"}`))
	req = mux.SetURLVars(req, map[string]string{"id": "p1"})
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); s.EditProxyPoolEntry()(rec, req) }()
	waitForProxyPoolLock(t, control, pid)
	tx.MustExec("UPDATE proxy_pool SET proxy_url = 'http://new.example:8080' WHERE id = 'p1'")
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	<-done
	if rec.Code != http.StatusOK {
		t.Fatalf("edit failed: %d %s", rec.Code, rec.Body.String())
	}
	entry, err := s.getProxyPoolEntry("p1")
	if err != nil || entry.ProxyURL != "http://new.example:8080" || entry.Label != "renamed" {
		t.Fatalf("partial edit lost concurrent changes: %+v, %v", entry, err)
	}
}
