package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waWa6"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// WUZAPI_TEST_POSTGRES_DSN optionally runs the same contracts against a real
// PostgreSQL server. Each test owns a new schema and removes only that schema.
func historyTestDB(t *testing.T, driver string) *sqlx.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "users.db")
	if driver == "postgres" {
		dsn = os.Getenv("WUZAPI_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("set WUZAPI_TEST_POSTGRES_DSN to exercise PostgreSQL")
		}
		admin, err := sqlx.Connect("postgres", dsn)
		if err != nil {
			t.Fatal(err)
		}
		id, err := GenerateRandomID()
		if err != nil {
			t.Fatal(err)
		}
		schema := "history_test_" + id
		if _, err := admin.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema)); err != nil {
			admin.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, err := admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema) + " CASCADE")
			if err != nil {
				t.Error(err)
			}
			admin.Close()
		})
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			u, err := url.Parse(dsn)
			if err != nil {
				t.Fatal(err)
			}
			q := u.Query()
			q.Set("search_path", schema)
			u.RawQuery = q.Encode()
			dsn = u.String()
		} else {
			dsn += " search_path=" + schema
		}
	}
	db, err := sqlx.Connect(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if driver == "postgres" {
		// Production creates the whatsmeow store before running application migrations.
		if _, err := db.Exec("CREATE TABLE whatsmeow_message_secrets (message_id TEXT)"); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func historyDrivers(t *testing.T, run func(*testing.T, *sqlx.DB)) {
	t.Helper()
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) { run(t, historyTestDB(t, driver)) })
	}
}

func TestHistorySyncMigration(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if err := createMigrationsTable(db); err != nil {
			t.Fatal(err)
		}
		for _, m := range migrations {
			if m.ID != 13 {
				if err := applyMigration(db, m); err != nil {
					t.Fatal(err)
				}
			}
		}
		if _, err := db.Exec("INSERT INTO users (id,name,token,history) VALUES ('existing','Existing','existing-token',42)"); err != nil {
			t.Fatal(err)
		}
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		var days, history int
		if err := db.QueryRow("SELECT days_to_sync_history,history FROM users WHERE id='existing'").Scan(&days, &history); err != nil {
			t.Fatal(err)
		}
		if days != 0 || history != 42 {
			t.Fatalf("migration changed defaults/retention: %d %d", days, history)
		}
		if _, err := db.Exec("UPDATE users SET days_to_sync_history=30"); err != nil {
			t.Fatal(err)
		}
		// Emulate a fork/manual column that predates the recorded migration.
		if _, err := db.Exec("DELETE FROM migrations WHERE id=13"); err != nil {
			t.Fatal(err)
		}
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		if err := db.Get(&days, "SELECT days_to_sync_history FROM users WHERE id='existing'"); err != nil {
			t.Fatal(err)
		}
		if days != 30 {
			t.Fatalf("existing days overwritten: %d", days)
		}
	})
}

func historyRequest(t *testing.T, handler http.HandlerFunc, method, body, id string) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": id})
	req = req.WithContext(context.WithValue(req.Context(), "userinfo", Values{m: map[string]string{"Id": id, "Token": "history-test-token"}}))
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestHistorySyncConfiguration(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		s := &server{db: db}
		result := historyRequest(t, s.AddUser(), "POST", `{"name":"History","token":"history-test-token","history":42,"days_to_sync_history":30}`, "")
		data := result["data"].(map[string]interface{})
		id := data["id"].(string)
		if data["days_to_sync_history"] != float64(30) {
			t.Fatalf("create omitted days: %v", data)
		}
		check := func(days, history int) {
			t.Helper()
			var gotDays, gotHistory int
			if err := db.QueryRow(db.Rebind("SELECT days_to_sync_history,history FROM users WHERE id=?"), id).Scan(&gotDays, &gotHistory); err != nil {
				t.Fatal(err)
			}
			if gotDays != days || gotHistory != history {
				t.Fatalf("got days=%d history=%d; want %d %d", gotDays, gotHistory, days, history)
			}
		}
		check(30, 42)
		historyRequest(t, s.EditUser(), "PUT", `{"name":"Renamed"}`, id)
		check(30, 42)
		historyRequest(t, s.EditUser(), "PUT", `{"days_to_sync_history":0}`, id)
		check(0, 42)
		result = historyRequest(t, s.SetHistory(), "POST", `{"days_to_sync_history":7}`, id)
		check(7, 42)
		if result["success"] != true || result["data"].(map[string]interface{})["History"] != float64(42) {
			t.Fatalf("session response contract changed: %v", result)
		}
		historyRequest(t, s.SetHistory(), "POST", `{"history":0}`, id)
		check(7, 0)
		historyRequest(t, s.SetHistory(), "POST", `{"days_to_sync_history":0}`, id)
		check(0, 0)
		historyRequest(t, s.SetHistory(), "POST", `{"history":100,"days_to_sync_history":365}`, id)
		check(365, 100)
		result = historyRequest(t, s.ListUsers(), "GET", "", id)
		users := result["data"].([]interface{})
		user := users[0].(map[string]interface{})
		if user["days_to_sync_history"] != float64(365) || user["history"] != float64(100) {
			t.Fatalf("readback: %v", user)
		}
		result = historyRequest(t, s.GetStatus(), "GET", "", id)
		if result["data"].(map[string]interface{})["days_to_sync_history"] != float64(365) {
			t.Fatalf("status: %v", result)
		}
		for _, body := range []string{`{"days_to_sync_history":-1}`, `{"days_to_sync_history":366}`, `{"days_to_sync_history":1.5}`} {
			for name, handler := range map[string]http.HandlerFunc{"session": s.SetHistory(), "create": s.AddUser(), "edit": s.EditUser()} {
				req := httptest.NewRequest("POST", "/", strings.NewReader(body))
				req = mux.SetURLVars(req, map[string]string{"id": id})
				req = req.WithContext(context.WithValue(req.Context(), "userinfo", Values{m: map[string]string{"Id": id}}))
				rec := httptest.NewRecorder()
				handler(rec, req)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("%s accepted %s: %d %s", name, body, rec.Code, rec.Body.String())
				}
			}
			check(365, 100)
		}
		// A new account with no option keeps defaults, and has no dependency on a session.
		result = historyRequest(t, s.AddUser(), "POST", `{"name":"Default","token":"default-history-token"}`, "")
		if result["data"].(map[string]interface{})["days_to_sync_history"] != float64(0) {
			t.Fatal("missing default")
		}
	})
}

func decodeHistoryProps(t *testing.T, payload *waWa6.ClientPayload) *waCompanionReg.DeviceProps {
	t.Helper()
	props := &waCompanionReg.DeviceProps{}
	if err := proto.Unmarshal(payload.DevicePairingData.DeviceProps, props); err != nil {
		t.Fatal(err)
	}
	return props
}

func TestHistorySyncPairingPayload(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		s := &server{db: db}
		if _, err := db.Exec("INSERT INTO users (id,name,token,days_to_sync_history) VALUES ('a','A','a',30),('b','B','b',7),('c','C','c',0)"); err != nil {
			t.Fatal(err)
		}
		defaults := proto.Clone(store.DeviceProps)
		container := &sqlstore.Container{}
		clients := make(map[string]*whatsmeow.Client)
		for _, id := range []string{"a", "b", "c"} {
			client := whatsmeow.NewClient(container.NewDevice(), nil)
			s.configureHistorySyncClient(client, id)
			clients[id] = client
		}
		for id, want := range map[string]uint32{"a": 30, "b": 7, "c": 0} {
			props := decodeHistoryProps(t, clients[id].GetClientPayload())
			if want > 0 {
				if !props.GetRequireFullSync() || props.GetHistorySyncConfig().GetFullSyncDaysLimit() != want {
					t.Fatalf("%s: %v", id, props)
				}
			} else if !proto.Equal(props.HistorySyncConfig, store.DeviceProps.HistorySyncConfig) || props.GetRequireFullSync() != store.DeviceProps.GetRequireFullSync() {
				t.Fatal("zero changed upstream defaults")
			}
		}
		// Saving before the next handshake takes effect even if the client already exists.
		if _, err := db.Exec("UPDATE users SET days_to_sync_history=14 WHERE id='a'"); err != nil {
			t.Fatal(err)
		}
		if got := decodeHistoryProps(t, clients["a"].GetClientPayload()).GetHistorySyncConfig().GetFullSyncDaysLimit(); got != 14 {
			t.Fatalf("stale setting: %d", got)
		}
		if !proto.Equal(defaults, store.DeviceProps) {
			t.Fatal("global device properties mutated")
		}
		jid := types.NewJID("123456", types.DefaultUserServer)
		clients["a"].Store.ID = &jid
		before := clients["a"].Store.GetClientPayload()
		after := clients["a"].GetClientPayload()
		if !proto.Equal(before, after) || after.DevicePairingData != nil {
			t.Fatal("ordinary reconnect requested new pairing history")
		}
	})
}

func TestHistorySyncMalformedPayload(t *testing.T) {
	p := &waWa6.ClientPayload{DevicePairingData: &waWa6.ClientPayload_DevicePairingRegistrationData{DeviceProps: []byte{0xff}}}
	if err := configurePairingHistory(p, 30); err == nil {
		t.Fatal("expected malformed protobuf error")
	}
	if !bytes.Equal(p.DevicePairingData.DeviceProps, []byte{0xff}) {
		t.Fatal("failed decode mutated payload")
	}
	for _, days := range []int{-1, 366} {
		t.Run(fmt.Sprint(days), func(t *testing.T) {
			if err := configurePairingHistory(&waWa6.ClientPayload{}, days); err == nil {
				t.Fatal("accepted invalid days")
			}
		})
	}
}
