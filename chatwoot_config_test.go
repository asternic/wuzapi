package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestChatwootConfigCreatesInboxAndMasksCredentials(t *testing.T) {
	var createInboxCalls int
	chatwoot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-chatwoot-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/accounts/1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":1}`))
		case "/api/v1/accounts/1/inboxes":
			createInboxCalls++
			var payload struct {
				Name    string `json:"name"`
				Channel struct {
					Type       string `json:"type"`
					WebhookURL string `json:"webhook_url"`
				} `json:"channel"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Name != "WuzAPI Test" || payload.Channel.Type != "api" || !strings.HasPrefix(payload.Channel.WebhookURL, "https://wuzapi.example.test/chatwoot/webhook/") {
				t.Fatalf("unexpected inbox payload: %+v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"name":"WuzAPI Test","channel":{"secret":"fixture-webhook-secret"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer chatwoot.Close()
	chatwootURL, err := url.Parse(chatwoot.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WUZAPI_CHATWOOT_INTERNAL_HOSTS", chatwootURL.Hostname())
	t.Setenv("WUZAPI_PUBLIC_URL", "https://wuzapi.example.test")

	previousKey, previousPersistent := *globalEncryptionKey, globalEncryptionKeyPersistent
	*globalEncryptionKey = "0123456789abcdef0123456789abcdef"
	globalEncryptionKeyPersistent = true
	t.Cleanup(func() {
		*globalEncryptionKey = previousKey
		globalEncryptionKeyPersistent = previousPersistent
	})

	s := makeTestServer(t)
	var migrationName string
	if err := s.db.Get(&migrationName, "SELECT name FROM chatwoot_schema_migrations WHERE id=1"); err != nil || migrationName != "foundation" {
		t.Fatalf("Chatwoot schema migration = %q, %v", migrationName, err)
	}
	if _, err := s.db.Exec("INSERT INTO users (id,name,token) VALUES ('chatwoot-instance','Chatwoot','chatwoot-instance-token')"); err != nil {
		t.Fatal(err)
	}

	body := `{"enabled":true,"url":"` + chatwoot.URL + `","account_id":1,"token":"fixture-chatwoot-token","inbox_name":"WuzAPI Test","attachment_hosts":["storage.example.test"]}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/chatwoot/config", strings.NewReader(body))
	request.Header.Set("Token", "chatwoot-instance-token")
	s.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /chatwoot/config = %d: %s", recorder.Code, recorder.Body.String())
	}
	if createInboxCalls != 1 {
		t.Fatalf("created %d inboxes, want 1", createInboxCalls)
	}

	var stored struct {
		Enabled       bool   `db:"enabled"`
		InboxID       int64  `db:"inbox_id"`
		Token         []byte `db:"token"`
		WebhookSecret []byte `db:"webhook_secret"`
	}
	if err := s.db.Get(&stored, "SELECT enabled, inbox_id, token, webhook_secret FROM chatwoot_config WHERE instance_id=?", "chatwoot-instance"); err != nil {
		t.Fatal(err)
	}
	if !stored.Enabled || stored.InboxID != 42 || string(stored.Token) == "fixture-chatwoot-token" || string(stored.WebhookSecret) == "fixture-webhook-secret" {
		t.Fatalf("stored config = %+v", stored)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/chatwoot/config", nil)
	request.Header.Set("Token", "chatwoot-instance-token")
	s.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /chatwoot/config = %d: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "fixture-chatwoot-token") || strings.Contains(recorder.Body.String(), "fixture-webhook-secret") || !strings.Contains(recorder.Body.String(), `"token_configured":true`) || !strings.Contains(recorder.Body.String(), `"webhook_configured":true`) || !strings.Contains(recorder.Body.String(), `"storage.example.test"`) {
		t.Fatalf("unsafe config response: %s", recorder.Body.String())
	}
}

func TestChatwootConfigRejectsExternalHTTPAndMissingPersistentKey(t *testing.T) {
	previousKey, previousPersistent := *globalEncryptionKey, globalEncryptionKeyPersistent
	*globalEncryptionKey = "0123456789abcdef0123456789abcdef"
	globalEncryptionKeyPersistent = false
	t.Cleanup(func() {
		*globalEncryptionKey = previousKey
		globalEncryptionKeyPersistent = previousPersistent
	})

	s := makeTestServer(t)
	if _, err := s.db.Exec("INSERT INTO users (id,name,token) VALUES ('chatwoot-reject','Chatwoot reject','chatwoot-reject-token')"); err != nil {
		t.Fatal(err)
	}

	for _, body := range []string{
		`{"url":"http://chatwoot.example.test","account_id":1,"token":"fixture"}`,
		`{"url":"https://chatwoot.example.test","account_id":1,"token":"fixture"}`,
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/chatwoot/config", strings.NewReader(body))
		request.Header.Set("Token", "chatwoot-reject-token")
		s.router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("POST /chatwoot/config = %d: %s", recorder.Code, recorder.Body.String())
		}
	}
}

func TestChatwootURLRejectsPrivateHTTPSHostsUnlessAllowlisted(t *testing.T) {
	t.Setenv("WUZAPI_CHATWOOT_INTERNAL_HOSTS", "")
	for _, value := range []string{"https://127.0.0.1", "https://[::1]", "https://10.0.0.7"} {
		if _, err := validateChatwootURL(value); err == nil {
			t.Fatalf("validateChatwootURL(%q) unexpectedly accepted a private host", value)
		}
	}
	t.Setenv("WUZAPI_CHATWOOT_INTERNAL_HOSTS", "127.0.0.1")
	if _, err := validateChatwootURL("https://127.0.0.1"); err != nil {
		t.Fatalf("allowlisted internal HTTPS host rejected: %v", err)
	}
}

func TestChatwootStatusAndDisableKeepRemoteInbox(t *testing.T) {
	previousKey, previousPersistent := *globalEncryptionKey, globalEncryptionKeyPersistent
	*globalEncryptionKey = "0123456789abcdef0123456789abcdef"
	globalEncryptionKeyPersistent = true
	t.Cleanup(func() {
		*globalEncryptionKey = previousKey
		globalEncryptionKeyPersistent = previousPersistent
	})
	s := makeTestServer(t)
	if _, err := s.db.Exec("INSERT INTO users (id,name,token) VALUES ('chatwoot-status','Chatwoot status','chatwoot-status-token')"); err != nil {
		t.Fatal(err)
	}
	token, err := encryptHMACKey("fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(s.db.Rebind(`INSERT INTO chatwoot_config (instance_id, enabled, url, account_id, token, inbox_id, inbox_name) VALUES (?, ?, ?, ?, ?, ?, ?)`), "chatwoot-status", true, "https://chatwoot.example.test", 1, token, 44, "Fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(s.db.Rebind(`INSERT INTO chatwoot_outbox (instance_id, direction, state, lock_key, source_message_id, payload, last_error) VALUES (?, ?, ?, ?, ?, ?, ?)`), "chatwoot-status", chatwootDirectionOutbound, chatwootOutboxPending, "fixture", "pending", []byte(`{}`), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(s.db.Rebind(`INSERT INTO chatwoot_outbox (instance_id, direction, state, lock_key, source_message_id, payload, last_error) VALUES (?, ?, ?, ?, ?, ?, ?)`), "chatwoot-status", chatwootDirectionOutbound, chatwootOutboxDead, "fixture-dead", "dead", []byte(`{}`), "sanitized error"); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/chatwoot/status", nil)
	request.Header.Set("Token", "chatwoot-status-token")
	s.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"pending":1`) || !strings.Contains(recorder.Body.String(), `"dead":1`) {
		t.Fatalf("GET /chatwoot/status = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/chatwoot/config", nil)
	request.Header.Set("Token", "chatwoot-status-token")
	s.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("DELETE /chatwoot/config = %d: %s", recorder.Code, recorder.Body.String())
	}
	var enabled bool
	if err := s.db.Get(&enabled, "SELECT enabled FROM chatwoot_config WHERE instance_id=?", "chatwoot-status"); err != nil || enabled {
		t.Fatalf("enabled = %t, %v", enabled, err)
	}
}
