package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/store/sqlstore"
)

func TestProxyPoolReleaseWaitsForSessionShutdown(t *testing.T) {
	s := makeTestServer(t)
	const userID = "pool-lifecycle"
	insertPoolTestUser(t, s, userID, "")
	insertPoolTestEntry(t, s, "p1", "http://proxy.example:8080", 1, true)
	kill, proxyURL, err := s.prepareClientStart(userID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteKillChannel(userID, kill) })
	if proxyURL != "http://proxy.example:8080" {
		t.Fatalf("unexpected proxy: %q", proxyURL)
	}
	// There is no WhatsApp client yet, just the reserved goroutine. The same
	// reservation remains present during retries and temporary disconnections.
	code, _ := adminRequest(t, s, http.MethodPost, "/admin/users/"+userID+"/proxy-pool/release", nil)
	if code != http.StatusConflict {
		t.Fatalf("release during startup: got %d, want 409", code)
	}
	if _, _, err := s.prepareClientStart(userID, true); !errors.Is(err, errSessionActive) {
		t.Fatalf("duplicate startup: got %v", err)
	}
	code, _ = adminRequest(t, s, http.MethodPut, "/admin/users/"+userID, map[string]interface{}{
		"proxyConfig": map[string]interface{}{"enabled": false},
	})
	if code != http.StatusConflict {
		t.Fatalf("admin proxy change during startup: got %d, want 409", code)
	}
	info := Values{map[string]string{"Id": userID, "Token": "token-" + userID}}
	req := httptest.NewRequest(http.MethodPost, "/session/proxy", strings.NewReader(`{"enable":false}`))
	req = req.WithContext(context.WithValue(req.Context(), "userinfo", info))
	rec := httptest.NewRecorder()
	s.SetProxy()(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("session proxy change during startup: got %d, want 400", rec.Code)
	}

	// Disconnect must also work before a client has been created. Acknowledging
	// shutdown does not make the slot available until the goroutine has exited.
	req = httptest.NewRequest(http.MethodPost, "/session/disconnect", nil)
	req = req.WithContext(context.WithValue(req.Context(), "userinfo", info))
	rec = httptest.NewRecorder()
	s.Disconnect()(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disconnect during startup: got %d: %s", rec.Code, rec.Body.String())
	}
	select {
	case <-kill:
	default:
		t.Fatal("disconnect did not signal the starting session")
	}
	code, _ = adminRequest(t, s, http.MethodPost, "/admin/users/"+userID+"/proxy-pool/release", nil)
	if code != http.StatusConflict {
		t.Fatalf("release before shutdown finishes: got %d, want 409", code)
	}
	if got, pool := userPoolState(t, s, userID); got != proxyURL || pool != "p1" {
		t.Fatalf("active assignment changed: proxy=%q pool=%q", got, pool)
	}
	deleteKillChannel(userID, kill)
	code, _ = adminRequest(t, s, http.MethodPost, "/admin/users/"+userID+"/proxy-pool/release", nil)
	if code != http.StatusOK {
		t.Fatalf("release after shutdown: got %d, want 200", code)
	}
}

func TestProxyPoolStartupFailureReleasesSessionReservation(t *testing.T) {
	s := makeTestServer(t)
	s.db.SetMaxOpenConns(1)
	previousContainer := container
	container = sqlstore.NewWithDB(s.db.DB, "sqlite", nil)
	t.Cleanup(func() { container = previousContainer })
	// Refuse CONNECT locally; no WhatsApp connection or credentials are needed.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	const userID = "pool-start-failure"
	insertPoolTestUser(t, s, userID, "")
	insertPoolTestEntry(t, s, "p1", proxy.URL, 1, true)
	kill, _, err := s.prepareClientStart(userID, true)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.startClient(userID, "", "token-"+userID, kill)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		signalKill(userID)
		<-done
		t.Fatal("failed connection setup did not return")
	}
	if sessionActive(userID) || clientManager.GetWhatsmeowClient(userID) != nil {
		t.Fatal("failed startup left an active session or client")
	}
	code, _ := adminRequest(t, s, http.MethodPost, "/admin/users/"+userID+"/proxy-pool/release", nil)
	if code != http.StatusOK {
		t.Fatalf("release after failed startup: got %d, want 200", code)
	}
}

func TestProxyPoolFailedPreparationDoesNotReserveSession(t *testing.T) {
	s := makeTestServer(t)
	insertPoolTestUser(t, s, "owner", "")
	insertPoolTestUser(t, s, "waiting", "")
	insertPoolTestEntry(t, s, "p1", "http://proxy.example:8080", 1, true)
	if _, err := s.assignProxyFromPool("owner"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.prepareClientStart("waiting", true); !errors.Is(err, ErrProxyPoolExhausted) {
		t.Fatalf("expected exhaustion, got %v", err)
	}
	if sessionActive("waiting") {
		t.Fatal("failed preparation leaked a session reservation")
	}
	// Restart behavior stays unchanged: a direct session is not assigned a
	// proxy by startup reconnect, even when the pool is exhausted.
	kill, _, err := s.prepareClientStart("waiting", false)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteKillChannel("waiting", kill)
	if proxy, pool := userPoolState(t, s, "waiting"); proxy != "" || pool != "" {
		t.Fatalf("startup changed direct session: proxy=%q pool=%q", proxy, pool)
	}
}

func TestProxyPoolPreservesAdminEditsToActiveExplicitProxy(t *testing.T) {
	s := makeTestServer(t)
	const userID = "explicit-active"
	insertPoolTestUser(t, s, userID, "http://old.example:8080")
	kill, _, err := s.prepareClientStart(userID, true)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteKillChannel(userID, kill)
	code, _ := adminRequest(t, s, http.MethodPut, "/admin/users/"+userID, map[string]interface{}{
		"proxyConfig": map[string]interface{}{"enabled": true, "proxyURL": "http://new.example:8080"},
	})
	if code != http.StatusOK {
		t.Fatalf("existing admin edit behavior changed: got %d", code)
	}
	if proxy, pool := userPoolState(t, s, userID); proxy != "http://new.example:8080" || pool != "" {
		t.Fatalf("unexpected explicit proxy state: proxy=%q pool=%q", proxy, pool)
	}
}
