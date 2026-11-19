package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow/types"
)

func TestChatwootWorkerDeliversOneQueuedDirectTextMessage(t *testing.T) {
	var paths []string
	chatwoot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer worker-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/accounts/1/contacts/search":
			if query := r.URL.Query().Get("q"); query != "wuzapi:5511999999999@s.whatsapp.net" && query != "+5511999999999" {
				t.Fatalf("search query = %q", r.URL.Query().Get("q"))
			}
			_, _ = w.Write([]byte(`{"payload":[]}`))
		case "/api/v1/accounts/1/contacts":
			_, _ = w.Write([]byte(`{"payload":[{"id":101}]}`))
		case "/api/v1/accounts/1/contacts/101/contact_inboxes":
			_, _ = w.Write([]byte(`{"source_id":"wuzapi:5511999999999@s.whatsapp.net"}`))
		case "/api/v1/accounts/1/contacts/101/conversations":
			_, _ = w.Write([]byte(`{"payload":[]}`))
		case "/api/v1/accounts/1/conversations":
			_, _ = w.Write([]byte(`{"id":201}`))
		case "/api/v1/accounts/1/conversations/201/messages":
			var payload struct {
				Content     string `json:"content"`
				MessageType string `json:"message_type"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Content != "fixture text" || payload.MessageType != "incoming" {
				t.Fatalf("message payload = %+v", payload)
			}
			_, _ = w.Write([]byte(`{"id":301}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer chatwoot.Close()
	allowChatwootTestHost(t, chatwoot.URL)

	s := makeTestServer(t)
	configureChatwootForWorker(t, s, "worker-instance", chatwoot.URL)
	message := chatwootOutboundMessage{
		InstanceID:  "worker-instance",
		MessageID:   "fixture-wa-message",
		ChatJID:     "5511999999999@s.whatsapp.net",
		ContactName: "Fixture",
		Text:        "fixture text",
	}
	if err := s.enqueueChatwootOutbound(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	processed, err := s.processNextChatwootOutbox(context.Background())
	if err != nil || !processed {
		t.Fatalf("processNextChatwootOutbox() = %t, %v", processed, err)
	}

	var state string
	if err := s.db.Get(&state, "SELECT state FROM chatwoot_outbox WHERE source_message_id=?", message.MessageID); err != nil || state != chatwootOutboxDone {
		t.Fatalf("outbox state = %q, %v", state, err)
	}
	var mapCount int
	if err := s.db.Get(&mapCount, "SELECT COUNT(*) FROM chatwoot_message_map WHERE instance_id=? AND wa_message_id=?", message.InstanceID, message.MessageID); err != nil || mapCount != 1 {
		t.Fatalf("message map count = %d, %v", mapCount, err)
	}
	if want := []string{
		"GET /api/v1/accounts/1/contacts/search",
		"GET /api/v1/accounts/1/contacts/search",
		"POST /api/v1/accounts/1/contacts",
		"POST /api/v1/accounts/1/contacts/101/contact_inboxes",
		"GET /api/v1/accounts/1/contacts/101/conversations",
		"POST /api/v1/accounts/1/conversations",
		"POST /api/v1/accounts/1/conversations/201/messages",
	}; strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Fatalf("Chatwoot calls = %v, want %v", paths, want)
	}
}

func TestChatwootWorkerReusesExistingBridgeContact(t *testing.T) {
	var paths []string
	chatwoot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/accounts/1/contacts/search":
			_, _ = w.Write([]byte(`{"payload":[{"id":101,"identifier":"wuzapi:5511999999999@s.whatsapp.net","contact_inboxes":[{"inbox":{"id":44},"source_id":"wuzapi:5511999999999@s.whatsapp.net"}]}]}`))
		case "/api/v1/accounts/1/contacts/101/conversations":
			_, _ = w.Write([]byte(`{"payload":[]}`))
		case "/api/v1/accounts/1/conversations":
			_, _ = w.Write([]byte(`{"id":201}`))
		case "/api/v1/accounts/1/conversations/201/messages":
			_, _ = w.Write([]byte(`{"id":301}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer chatwoot.Close()
	allowChatwootTestHost(t, chatwoot.URL)

	s := makeTestServer(t)
	configureChatwootForWorker(t, s, "bridge-instance", chatwoot.URL)
	message := chatwootOutboundMessage{InstanceID: "bridge-instance", MessageID: "bridge-message", ChatJID: "5511999999999@s.whatsapp.net", Text: "fixture"}
	if err := s.enqueueChatwootOutbound(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if processed, err := s.processNextChatwootOutbox(context.Background()); err != nil || !processed {
		t.Fatalf("processNextChatwootOutbox() = %t, %v", processed, err)
	}
	if want := []string{
		"GET /api/v1/accounts/1/contacts/search",
		"GET /api/v1/accounts/1/contacts/101/conversations",
		"POST /api/v1/accounts/1/conversations",
		"POST /api/v1/accounts/1/conversations/201/messages",
	}; strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Fatalf("Chatwoot calls = %v, want %v", paths, want)
	}
}

func TestChatwootOutboxIsIdempotentAndRejectsNonDirectChats(t *testing.T) {
	s := makeTestServer(t)
	configureChatwootForWorker(t, s, "queue-instance", "https://chatwoot.example.test")
	direct := chatwootOutboundMessage{InstanceID: "queue-instance", MessageID: "same-id", ChatJID: "123@lid", Text: "fixture"}
	if err := s.enqueueChatwootOutbound(context.Background(), direct); err != nil {
		t.Fatal(err)
	}
	if err := s.enqueueChatwootOutbound(context.Background(), direct); err != nil {
		t.Fatal(err)
	}
	if err := s.enqueueChatwootOutbound(context.Background(), chatwootOutboundMessage{InstanceID: "queue-instance", MessageID: "group-id", ChatJID: types.NewJID("120363000000000000", types.GroupServer).String(), Text: "fixture"}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.Get(&count, "SELECT COUNT(*) FROM chatwoot_outbox"); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("outbox count = %d, want 1", count)
	}
}

func TestChatwootDisabledDoesNotQueueMessages(t *testing.T) {
	s := makeTestServer(t)
	if _, err := s.db.Exec("INSERT INTO users (id,name,token) VALUES ('disabled-instance','Disabled','disabled-token')"); err != nil {
		t.Fatal(err)
	}
	message := chatwootOutboundMessage{InstanceID: "disabled-instance", MessageID: "disabled-id", ChatJID: "5511999999999@s.whatsapp.net", Text: "fixture"}
	if err := s.enqueueChatwootOutbound(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.Get(&count, "SELECT COUNT(*) FROM chatwoot_outbox"); err != nil || count != 0 {
		t.Fatalf("queued %d rows, %v", count, err)
	}
}

func TestChatwootDoesNotRequeueItsOwnWhatsAppEcho(t *testing.T) {
	s := makeTestServer(t)
	configureChatwootForWorker(t, s, "echo-instance", "https://chatwoot.example.test")
	if _, err := s.db.Exec(s.db.Rebind(`INSERT INTO chatwoot_message_map (instance_id, wa_message_id, chatwoot_message_id, conversation_id, direction) VALUES (?, ?, ?, ?, ?)`), "echo-instance", "echo-id", 301, 201, chatwootDirectionInbound); err != nil {
		t.Fatal(err)
	}
	if err := s.enqueueChatwootOutbound(context.Background(), chatwootOutboundMessage{InstanceID: "echo-instance", MessageID: "echo-id", ChatJID: "5511999999999@s.whatsapp.net", Text: "fixture", FromMe: true}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.Get(&count, "SELECT COUNT(*) FROM chatwoot_outbox"); err != nil || count != 0 {
		t.Fatalf("queued echo rows = %d, %v", count, err)
	}
}

func TestChatwootRetryDelayIsBounded(t *testing.T) {
	if got := chatwootRetryDelay(1); got < time.Second || got > 2*time.Second {
		t.Fatalf("first retry = %s", got)
	}
	if got := chatwootRetryDelay(100); got > 5*time.Minute {
		t.Fatalf("retry must be capped, got %s", got)
	}
}

func TestChatwootOutboxRetriesThenMarksDeadAndRecoversExpiredLease(t *testing.T) {
	chatwoot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer chatwoot.Close()
	s := makeTestServer(t)
	configureChatwootForWorker(t, s, "retry-instance", chatwoot.URL)
	message := chatwootOutboundMessage{InstanceID: "retry-instance", MessageID: "retry-id", ChatJID: "5511999999999@s.whatsapp.net", Text: "fixture"}
	if err := s.enqueueChatwootOutbound(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if processed, err := s.processNextChatwootOutbox(context.Background()); err != nil || !processed {
		t.Fatalf("first processing = %t, %v", processed, err)
	}
	var row struct {
		State    string `db:"state"`
		Attempts int    `db:"attempts"`
		LastErr  string `db:"last_error"`
	}
	if err := s.db.Get(&row, "SELECT state, attempts, last_error FROM chatwoot_outbox WHERE source_message_id=?", message.MessageID); err != nil {
		t.Fatal(err)
	}
	if row.State != chatwootOutboxPending || row.Attempts != 1 || strings.Contains(row.LastErr, "fixture") {
		t.Fatalf("first failure row = %+v", row)
	}
	if _, err := s.db.Exec(`UPDATE chatwoot_outbox SET attempts=?, next_attempt_at=CURRENT_TIMESTAMP WHERE source_message_id=?`, chatwootMaxAttempts-1, message.MessageID); err != nil {
		t.Fatal(err)
	}
	if processed, err := s.processNextChatwootOutbox(context.Background()); err != nil || !processed {
		t.Fatalf("dead processing = %t, %v", processed, err)
	}
	if err := s.db.Get(&row, "SELECT state, attempts, last_error FROM chatwoot_outbox WHERE source_message_id=?", message.MessageID); err != nil {
		t.Fatal(err)
	}
	if row.State != chatwootOutboxDead || row.Attempts != chatwootMaxAttempts {
		t.Fatalf("dead row = %+v", row)
	}

	if _, err := s.db.Exec(s.db.Rebind(`INSERT INTO chatwoot_outbox (instance_id, direction, state, lock_key, source_message_id, payload, locked_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)`), "retry-instance", chatwootDirectionOutbound, chatwootOutboxProcessing, "retry:expired", "expired", []byte(`{}`), time.Now().Add(-chatwootOutboxLease-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO chatwoot_outbox_locks (lock_key, outbox_id) SELECT lock_key, id FROM chatwoot_outbox WHERE source_message_id='expired'`); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.recoverExpiredChatwootOutbox(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := s.db.Get(&state, "SELECT state FROM chatwoot_outbox WHERE source_message_id='expired'"); err != nil || state != chatwootOutboxPending {
		t.Fatalf("recovered state = %q, %v", state, err)
	}
}

func TestChatwootSchemaUsesItsOwnMigrationRegistry(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		var name string
		if err := db.Get(&name, "SELECT name FROM chatwoot_schema_migrations WHERE id=1"); err != nil || name != "foundation" {
			t.Fatalf("Chatwoot migration = %q, %v", name, err)
		}
		if err := initializeSchema(db); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.Get(&count, "SELECT COUNT(*) FROM chatwoot_schema_migrations"); err != nil || count != 4 {
			t.Fatalf("Chatwoot migration count = %d, %v", count, err)
		}
	})
}

func configureChatwootForWorker(t *testing.T, s *server, instanceID, baseURL string) {
	t.Helper()
	previousKey, previousPersistent := *globalEncryptionKey, globalEncryptionKeyPersistent
	*globalEncryptionKey = "0123456789abcdef0123456789abcdef"
	globalEncryptionKeyPersistent = true
	t.Cleanup(func() {
		*globalEncryptionKey = previousKey
		globalEncryptionKeyPersistent = previousPersistent
	})
	if _, err := s.db.Exec("INSERT INTO users (id,name,token) VALUES (?,?,?)", instanceID, "Worker", instanceID+"-token"); err != nil {
		t.Fatal(err)
	}
	token, err := encryptHMACKey("worker-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(s.db.Rebind(`INSERT INTO chatwoot_config (instance_id, enabled, url, account_id, token, inbox_id, inbox_name) VALUES (?, ?, ?, ?, ?, ?, ?)`), instanceID, true, baseURL, 1, token, 44, "Fixture"); err != nil {
		t.Fatal(err)
	}
}

func allowChatwootTestHost(t *testing.T, baseURL string) {
	t.Helper()
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WUZAPI_CHATWOOT_INTERNAL_HOSTS", parsed.Hostname())
}
