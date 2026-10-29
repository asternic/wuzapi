package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow"
)

func TestPreserveDevicesDeletionIsolation(t *testing.T) {
	historyDrivers(t, func(t *testing.T, db *sqlx.DB) {
		s := deviceTestServer(t, db)
		first := savedTestDevice(t, "15551234567:92@s.whatsapp.net")
		second := savedTestDevice(t, "15551234567:95@s.whatsapp.net")
		insertDeviceTestUser(t, s, "preserve-delete", "15551234567@s.whatsapp.net", "")
		insertDeviceTestUser(t, s, "preserve-keep", "15551234567@s.whatsapp.net", second.ID.String())
		insertClientMessage(t, s, "preserve-delete", "chat", "remove", "remove", time.Now())
		insertClientMessage(t, s, "preserve-keep", "chat", "keep", "keep", time.Now())
		for _, id := range []string{"preserve-delete", "preserve-keep"} {
			dir := filepath.Join(s.exPath, "files", id)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "media"), []byte("test"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		clientManager.SetWhatsmeowClient("preserve-delete", whatsmeow.NewClient(first, nil))
		keepClient := whatsmeow.NewClient(second, nil)
		clientManager.SetWhatsmeowClient("preserve-keep", keepClient)
		t.Cleanup(func() {
			clientManager.DeleteWhatsmeowClient("preserve-delete")
			clientManager.DeleteWhatsmeowClient("preserve-keep")
		})
		req := mux.SetURLVars(httptest.NewRequest(http.MethodDelete, "/", nil), map[string]string{"id": "preserve-delete"})
		response := httptest.NewRecorder()
		s.DeleteUserPreserveDevices()(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("%d %s", response.Code, response.Body.String())
		}
		var body struct {
			Data struct {
				Preserved bool `json:"devices_preserved"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || !body.Data.Preserved {
			t.Fatalf("missing preservation result: %s", response.Body.String())
		}
		for _, table := range []string{"users", "message_history"} {
			column := "user_id"
			if table == "users" {
				column = "id"
			}
			var removed, kept int
			if err := db.Get(&removed, db.Rebind("SELECT COUNT(*) FROM "+table+" WHERE "+column+"=?"), "preserve-delete"); err != nil {
				t.Fatal(err)
			}
			if err := db.Get(&kept, db.Rebind("SELECT COUNT(*) FROM "+table+" WHERE "+column+"=?"), "preserve-keep"); err != nil {
				t.Fatal(err)
			}
			if removed != 0 || kept != 1 {
				t.Fatalf("%s: removed=%d kept=%d", table, removed, kept)
			}
		}
		devices, err := container.GetAllDevices(context.Background())
		if err != nil || len(devices) != 2 {
			t.Fatalf("credentials changed: %v %v", devices, err)
		}
		if _, err := os.Stat(filepath.Join(s.exPath, "files", "preserve-delete")); !os.IsNotExist(err) {
			t.Fatalf("files retained: %v", err)
		}
		if _, err := os.Stat(filepath.Join(s.exPath, "files", "preserve-keep", "media")); err != nil {
			t.Fatal(err)
		}
		if clientManager.GetWhatsmeowClient("preserve-delete") != nil || clientManager.GetWhatsmeowClient("preserve-keep") != keepClient {
			t.Fatal("incorrect client cleanup")
		}
		if _, _, err := s.prepareClientStart("preserve-delete", false); err == nil {
			t.Fatal("deleted user restarted")
		}
	})
}

func TestPreserveDevicesWaitsForSessionStop(t *testing.T) {
	s := makeTestServer(t)
	insertDeviceTestUser(t, s, "preserve-stopping", "", "")
	kill := make(chan bool, 1)
	setKillChannel("preserve-stopping", kill)
	defer deleteKillChannel("preserve-stopping", kill)
	response := httptest.NewRecorder()
	req := mux.SetURLVars(httptest.NewRequest(http.MethodDelete, "/", nil), map[string]string{"id": "preserve-stopping"})
	done := make(chan struct{})
	go func() { s.DeleteUserPreserveDevices()(response, req); close(done) }()
	select {
	case <-kill:
	case <-time.After(2 * time.Second):
		t.Fatal("session not signaled")
	}
	if _, _, err := s.prepareClientStart("preserve-stopping", false); err == nil {
		t.Fatal("connection allowed while deleting")
	}
	if err := reserveSessionDeletion("preserve-stopping"); err == nil {
		releaseSessionDeletion("preserve-stopping")
		t.Fatal("parallel deletion allowed")
	}
	// Keep the owner registered to exercise the real shutdown timeout.
	select {
	case <-done:
	case <-time.After(12 * time.Second):
		t.Fatal("deletion did not time out")
	}
	if response.Code != http.StatusConflict {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
	var count int
	if err := s.db.Get(&count, `SELECT COUNT(*) FROM users WHERE id='preserve-stopping'`); err != nil || count != 1 {
		t.Fatalf("user lost: %d %v", count, err)
	}
	if err := reserveSessionDeletion("preserve-stopping"); err != nil {
		t.Fatal("reservation not released", err)
	}
	releaseSessionDeletion("preserve-stopping")
}

func TestPreserveDevicesHTTPAndStdio(t *testing.T) {
	s := makeTestServer(t)
	s.exPath = t.TempDir()
	insertDeviceTestUser(t, s, "preserve-stdio", "15551234567@s.whatsapp.net", "")
	// An unavailable store proves this route never tries to resolve or delete credentials.
	previous := container
	container = nil
	defer func() { container = previous }()
	for _, token := range []string{"", "wrong-token"} {
		req := httptest.NewRequest(http.MethodDelete, "/admin/users/preserve-stdio/preserve-devices", nil)
		req.Header.Set("Authorization", token)
		response := httptest.NewRecorder()
		s.router.ServeHTTP(response, req)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthorized delete: %d", response.Code)
		}
	}
	denied := executeRequest(t, s, newRequest("denied", "admin.users.delete.preserve_devices", map[string]interface{}{"userId": "preserve-stdio", "adminToken": "wrong-token"}).toJSON(t))
	assertJSONRPC20Error(t, denied, "denied", 401)
	missingID := executeRequest(t, s, newRequest("missing-id", "admin.users.delete.preserve_devices", map[string]interface{}{"adminToken": "test-admin-token"}).toJSON(t))
	if missingID["error"] == nil {
		t.Fatal("missing userId accepted")
	}
	result := executeRequest(t, s, newRequest("delete", "admin.users.delete.preserve_devices", map[string]interface{}{"userId": "preserve-stdio", "adminToken": "test-admin-token"}).toJSON(t))
	data := assertJSONRPC20Success(t, result, "delete").(map[string]interface{})
	if data["devices_preserved"] != true {
		t.Fatalf("unexpected result: %v", data)
	}
	missing := executeRequest(t, s, newRequest("missing", "admin.users.delete.preserve_devices", map[string]interface{}{"userId": "preserve-stdio", "adminToken": "test-admin-token"}).toJSON(t))
	assertJSONRPC20Error(t, missing, "missing", 404)
}

type deletionClientProbe struct {
	connected bool
	logoutErr error
	calls     []string
}

func (c *deletionClientProbe) IsConnected() bool {
	c.calls = append(c.calls, "connected")
	return c.connected
}
func (c *deletionClientProbe) Logout(context.Context) error {
	c.calls = append(c.calls, "logout")
	return c.logoutErr
}
func (c *deletionClientProbe) Disconnect() { c.calls = append(c.calls, "disconnect") }

func TestDeletionClientPreservesRemoteSession(t *testing.T) {
	client := &deletionClientProbe{connected: true}
	if err := disconnectSessionForDeletion(context.Background(), client, true); err != nil {
		t.Fatal(err)
	}
	if len(client.calls) != 1 || client.calls[0] != "disconnect" {
		t.Fatalf("preservation touched remote session: %v", client.calls)
	}
	client = &deletionClientProbe{connected: true, logoutErr: errors.New("logout failed")}
	if err := disconnectSessionForDeletion(context.Background(), client, false); !errors.Is(err, client.logoutErr) {
		t.Fatalf("lost logout error: %v", err)
	}
	if len(client.calls) != 3 || client.calls[1] != "logout" || client.calls[2] != "disconnect" {
		t.Fatalf("full deletion lifecycle changed: %v", client.calls)
	}
}
