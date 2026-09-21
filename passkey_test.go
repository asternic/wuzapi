package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// ===================== Test helpers =====================

// resetPasskeyMap clears the global pending passkey map and registers a
// cleanup so it is always cleared after each test finishes.
func resetPasskeyMap(t *testing.T) {
	t.Helper()
	pendingPasskeyMu.Lock()
	pendingPasskeyRequests = make(map[string]*PendingPasskeyState)
	pendingPasskeyMu.Unlock()
	t.Cleanup(func() {
		pendingPasskeyMu.Lock()
		pendingPasskeyRequests = make(map[string]*PendingPasskeyState)
		pendingPasskeyMu.Unlock()
	})
}

// addTestUserForPasskey inserts a minimal user row and returns its ID.
func addTestUserForPasskey(t *testing.T, s *server, token string) string {
	t.Helper()
	userID := "test-user-" + token
	_, err := s.db.Exec(
		"INSERT INTO users (id, name, token) VALUES (?, ?, ?)",
		userID, "Test User", token,
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	return userID
}

// injectUserInfo simulates the authalice middleware by placing a Values
// context value on the request.
func injectUserInfo(r *http.Request, userID string) *http.Request {
	v := Values{map[string]string{
		"Id":    userID,
		"Token": "test-token",
	}}
	return r.WithContext(context.WithValue(r.Context(), "userinfo", v))
}

// parseResponse unmarshals the recorder body and returns the map.
func parseResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v\nbody: %s", err, w.Body.String())
	}
	return resp
}

// assertErrorBody checks that the response is a JSON-RPC-style error
// envelope (as returned by s.Respond with an error) and that the error
// message contains the expected substring.
func assertErrorBody(t *testing.T, w *httptest.ResponseRecorder, expectedStatus int, substr string) {
	t.Helper()
	if w.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d", expectedStatus, w.Code)
	}
	resp := parseResponse(t, w)
	if resp["success"] != false {
		t.Fatalf("expected success=false, got %v", resp["success"])
	}
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, substr) {
		t.Fatalf("expected error to contain %q, got %q", substr, errMsg)
	}
}

// ===================== Memory / lifecycle tests =====================

func TestPendingPasskeyState_Lifecycle(t *testing.T) {
	resetPasskeyMap(t)

	t.Run("empty_map_returns_nil", func(t *testing.T) {
		if pk := peekPendingPasskey("u1"); pk != nil {
			t.Fatalf("peek expected nil, got %+v", pk)
		}
		if pk := getAndConsumePendingPasskey("u1"); pk != nil {
			t.Fatalf("consume expected nil, got %+v", pk)
		}
	})

	t.Run("peek_is_non_destructive", func(t *testing.T) {
		state := &PendingPasskeyState{Request: &events.PairPasskeyRequest{}}
		storePendingPasskey("u2", state)

		if pk := peekPendingPasskey("u2"); pk == nil {
			t.Fatal("first peek: expected state")
		}
		if pk := peekPendingPasskey("u2"); pk == nil {
			t.Fatal("second peek: expected state again")
		}
	})

	t.Run("consume_removes_state", func(t *testing.T) {
		state := &PendingPasskeyState{Request: &events.PairPasskeyRequest{}}
		storePendingPasskey("u3", state)

		if pk := getAndConsumePendingPasskey("u3"); pk == nil {
			t.Fatal("consume: expected state")
		}
		if pk := peekPendingPasskey("u3"); pk != nil {
			t.Fatal("after consume: expected nil")
		}
	})

	t.Run("double_consume_returns_nil", func(t *testing.T) {
		state := &PendingPasskeyState{Request: &events.PairPasskeyRequest{}}
		storePendingPasskey("u4", state)

		_ = getAndConsumePendingPasskey("u4")
		if pk := getAndConsumePendingPasskey("u4"); pk != nil {
			t.Fatal("second consume should return nil")
		}
	})
}

func TestPendingPasskeyState_DeleteExplicitly(t *testing.T) {
	resetPasskeyMap(t)

	storePendingPasskey("d1", &PendingPasskeyState{Request: &events.PairPasskeyRequest{}})
	if peekPendingPasskey("d1") == nil {
		t.Fatal("expected state after store")
	}

	deletePendingPasskey("d1")
	if peekPendingPasskey("d1") != nil {
		t.Fatal("expected nil after explicit delete")
	}
}

func TestPendingPasskeyState_IsolationBetweenUsers(t *testing.T) {
	resetPasskeyMap(t)

	storePendingPasskey("iso-a", &PendingPasskeyState{Request: &events.PairPasskeyRequest{}})
	storePendingPasskey("iso-b", &PendingPasskeyState{Request: &events.PairPasskeyRequest{}})

	getAndConsumePendingPasskey("iso-a")

	if peekPendingPasskey("iso-a") != nil {
		t.Fatal("user-a should be consumed")
	}
	if peekPendingPasskey("iso-b") == nil {
		t.Fatal("user-b should still be present")
	}
}

func TestPendingPasskeyState_Overwrite(t *testing.T) {
	resetPasskeyMap(t)

	state1 := &PendingPasskeyState{
		Request:   &events.PairPasskeyRequest{},
		CreatedAt: time.Now().Add(-10 * time.Minute),
	}
	state2 := &PendingPasskeyState{
		Request:   &events.PairPasskeyRequest{},
		CreatedAt: time.Now(),
	}

	storePendingPasskey("ow", state1)
	storePendingPasskey("ow", state2) // overwrite

	pk := peekPendingPasskey("ow")
	if pk == nil {
		t.Fatal("expected state after overwrite")
	}
	if pk.CreatedAt.Before(state2.CreatedAt.Add(-time.Second)) {
		t.Fatal("second store should have overwritten the first")
	}
}

func TestPendingPasskeyState_TTLExpiry(t *testing.T) {
	resetPasskeyMap(t)

	// Inject an old state directly (bypass store to set CreatedAt in the past)
	pendingPasskeyMu.Lock()
	pendingPasskeyRequests["old-user"] = &PendingPasskeyState{
		Request:   &events.PairPasskeyRequest{},
		CreatedAt: time.Now().Add(-20 * time.Minute),
	}
	pendingPasskeyMu.Unlock()

	// Simulate cleanup (mirrors the goroutine logic)
	now := time.Now()
	pendingPasskeyMu.Lock()
	for uid, st := range pendingPasskeyRequests {
		if now.Sub(st.CreatedAt) > passkeyStateTTL {
			delete(pendingPasskeyRequests, uid)
		}
	}
	pendingPasskeyMu.Unlock()

	if peekPendingPasskey("old-user") != nil {
		t.Fatal("expired state should have been cleaned up")
	}
}

func TestPendingPasskeyState_FreshStateSurvivesCleanup(t *testing.T) {
	resetPasskeyMap(t)

	storePendingPasskey("fresh", &PendingPasskeyState{
		Request:   &events.PairPasskeyRequest{},
		CreatedAt: time.Now(),
	})

	now := time.Now()
	pendingPasskeyMu.Lock()
	for uid, st := range pendingPasskeyRequests {
		if now.Sub(st.CreatedAt) > passkeyStateTTL {
			delete(pendingPasskeyRequests, uid)
		}
	}
	pendingPasskeyMu.Unlock()

	if peekPendingPasskey("fresh") == nil {
		t.Fatal("fresh state should survive cleanup")
	}
}

// getData extracts the "data" field from the standard s.Respond envelope.
func getData(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	resp := parseResponse(t, w)
	data, _ := resp["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("expected data field in response, got: %v", resp)
	}
	return data
}

// ===================== HTTP handler: GetPasskeyStatus =====================

func TestGetPasskeyStatus_NoPending(t *testing.T) {
	resetPasskeyMap(t)
	s := makeTestServer(t)
	userID := addTestUserForPasskey(t, s, "ps-np")

	req := httptest.NewRequest("GET", "/session/passkey-status", nil)
	req = mux.SetURLVars(req, map[string]string{"id": userID})
	req = injectUserInfo(req, userID)
	w := httptest.NewRecorder()
	s.GetPasskeyStatus()(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	data := getData(t, w)
	if data["passkeyPending"] != false {
		t.Fatalf("expected passkeyPending=false, got %v", data["passkeyPending"])
	}
	if data["publicKey"] != nil {
		t.Fatalf("expected publicKey=nil, got %v", data["publicKey"])
	}
}

func TestGetPasskeyStatus_WithPending(t *testing.T) {
	resetPasskeyMap(t)
	s := makeTestServer(t)
	userID := addTestUserForPasskey(t, s, "ps-wp")

	fakePK := &types.WebAuthnPublicKey{RelyingPartID: "whatsapp.com", Timeout: 600000}
	storePendingPasskey(userID, &PendingPasskeyState{
		Request:   &events.PairPasskeyRequest{PublicKey: fakePK},
		CreatedAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/session/passkey-status", nil)
	req = mux.SetURLVars(req, map[string]string{"id": userID})
	req = injectUserInfo(req, userID)
	w := httptest.NewRecorder()
	s.GetPasskeyStatus()(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	data := getData(t, w)
	if data["passkeyPending"] != true {
		t.Fatalf("expected passkeyPending=true, got %v", data["passkeyPending"])
	}
	if data["publicKey"] == nil {
		t.Fatal("expected publicKey to be non-nil")
	}
}

func TestGetPasskeyStatus_StateWithNilRequest(t *testing.T) {
	resetPasskeyMap(t)
	s := makeTestServer(t)
	userID := addTestUserForPasskey(t, s, "ps-nr")

	// Store a state where Request is nil
	pendingPasskeyMu.Lock()
	pendingPasskeyRequests[userID] = &PendingPasskeyState{
		Request:   nil,
		CreatedAt: time.Now(),
	}
	pendingPasskeyMu.Unlock()

	req := httptest.NewRequest("GET", "/session/passkey-status", nil)
	req = mux.SetURLVars(req, map[string]string{"id": userID})
	req = injectUserInfo(req, userID)
	w := httptest.NewRecorder()
	s.GetPasskeyStatus()(w, req)

	data := getData(t, w)
	// Request is nil → passkeyPending should be false
	if data["passkeyPending"] != false {
		t.Fatalf("expected passkeyPending=false when Request=nil, got %v", data["passkeyPending"])
	}
}

// ===================== HTTP handler: GetQR =====================

// TestGetQR_NoSession_ReturnsNoSession verifies that GetQR returns a 500
// with "no session" when the clientManager has no client for this user.
// This is the realistic path since we cannot spin up a real whatsmeow
// client in a unit test.
func TestGetQR_NoSession_ReturnsNoSession(t *testing.T) {
	resetPasskeyMap(t)
	s := makeTestServer(t)
	userID := addTestUserForPasskey(t, s, "qr-ns")

	req := httptest.NewRequest("GET", "/session/qr", nil)
	req = mux.SetURLVars(req, map[string]string{"id": userID})
	req = injectUserInfo(req, userID)
	w := httptest.NewRecorder()
	s.GetQR()(w, req)

	// GetQR checks clientManager first → no client = "no session" error
	assertErrorBody(t, w, http.StatusInternalServerError, "no session")
}

// TestGetQR_NoSession_DoesNotLeakPasskey verifies that even when a
// passkey is pending, the "no session" check happens first. The passkey
// state should remain unconsumed.
func TestGetQR_NoSession_DoesNotLeakPasskey(t *testing.T) {
	resetPasskeyMap(t)
	s := makeTestServer(t)
	userID := addTestUserForPasskey(t, s, "qr-leak")

	fakePK := &types.WebAuthnPublicKey{RelyingPartID: "whatsapp.com"}
	storePendingPasskey(userID, &PendingPasskeyState{
		Request:   &events.PairPasskeyRequest{PublicKey: fakePK},
		CreatedAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/session/qr", nil)
	req = mux.SetURLVars(req, map[string]string{"id": userID})
	req = injectUserInfo(req, userID)
	w := httptest.NewRecorder()
	s.GetQR()(w, req)

	// Should fail with "no session"
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}

	// Passkey state should NOT have been consumed
	if pk := peekPendingPasskey(userID); pk == nil {
		t.Fatal("passkey state should still exist after GetQR 'no session' error")
	}
}

// ===================== HTTP handler: PasskeyResponse =====================

func TestPasskeyResponse_NoPendingReturns400(t *testing.T) {
	resetPasskeyMap(t)
	s := makeTestServer(t)
	userID := addTestUserForPasskey(t, s, "pr-400")

	body := `{"response":{"id":"x","rawId":"x","type":"public-key","response":{"clientDataJSON":"x","authenticatorData":"x","signature":"x","userHandle":null}}}`
	req := httptest.NewRequest("POST", "/session/passkey-response", strings.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": userID})
	req = injectUserInfo(req, userID)
	w := httptest.NewRecorder()
	s.PasskeyResponse()(w, req)

	assertErrorBody(t, w, http.StatusBadRequest, "no pending passkey")
}

func TestPasskeyResponse_InvalidJSON_Returns400AndRestoresState(t *testing.T) {
	resetPasskeyMap(t)
	s := makeTestServer(t)
	userID := addTestUserForPasskey(t, s, "pr-badjson")

	fakePK := &types.WebAuthnPublicKey{RelyingPartID: "whatsapp.com"}
	storePendingPasskey(userID, &PendingPasskeyState{
		Request:   &events.PairPasskeyRequest{PublicKey: fakePK},
		CreatedAt: time.Now(),
	})

	req := httptest.NewRequest("POST", "/session/passkey-response", strings.NewReader("NOT_JSON"))
	req = mux.SetURLVars(req, map[string]string{"id": userID})
	req = injectUserInfo(req, userID)
	w := httptest.NewRecorder()
	s.PasskeyResponse()(w, req)

	assertErrorBody(t, w, http.StatusBadRequest, "could not decode")

	// State should be restored so the user can retry
	if pk := peekPendingPasskey(userID); pk == nil {
		t.Fatal("state should be restored after decode error")
	}
}

func TestPasskeyResponse_MissingResponseField_Returns400AndRestoresState(t *testing.T) {
	resetPasskeyMap(t)
	s := makeTestServer(t)
	userID := addTestUserForPasskey(t, s, "pr-noresp")

	fakePK := &types.WebAuthnPublicKey{RelyingPartID: "whatsapp.com"}
	storePendingPasskey(userID, &PendingPasskeyState{
		Request:   &events.PairPasskeyRequest{PublicKey: fakePK},
		CreatedAt: time.Now(),
	})

	// Valid JSON but missing the "response" field
	req := httptest.NewRequest("POST", "/session/passkey-response", strings.NewReader("{}"))
	req = mux.SetURLVars(req, map[string]string{"id": userID})
	req = injectUserInfo(req, userID)
	w := httptest.NewRecorder()
	s.PasskeyResponse()(w, req)

	assertErrorBody(t, w, http.StatusBadRequest, "missing response")

	// State should be restored
	if pk := peekPendingPasskey(userID); pk == nil {
		t.Fatal("state should be restored after missing response field")
	}
}

// ===================== HTTP handler: PasskeyConfirm =====================

func TestPasskeyConfirm_NoPendingReturns400(t *testing.T) {
	resetPasskeyMap(t)
	s := makeTestServer(t)
	userID := addTestUserForPasskey(t, s, "pc-400")

	req := httptest.NewRequest("POST", "/session/passkey-confirm", nil)
	req = mux.SetURLVars(req, map[string]string{"id": userID})
	req = injectUserInfo(req, userID)
	w := httptest.NewRecorder()
	s.PasskeyConfirm()(w, req)

	assertErrorBody(t, w, http.StatusBadRequest, "no pending passkey")
}

func TestPasskeyConfirm_StateWithNilClientReturns400(t *testing.T) {
	resetPasskeyMap(t)
	s := makeTestServer(t)
	userID := addTestUserForPasskey(t, s, "pc-nil")

	// State exists but Client is nil
	pendingPasskeyMu.Lock()
	pendingPasskeyRequests[userID] = &PendingPasskeyState{
		Request:   &events.PairPasskeyRequest{},
		Client:    nil,
		CreatedAt: time.Now(),
	}
	pendingPasskeyMu.Unlock()

	req := httptest.NewRequest("POST", "/session/passkey-confirm", nil)
	req = mux.SetURLVars(req, map[string]string{"id": userID})
	req = injectUserInfo(req, userID)
	w := httptest.NewRecorder()
	s.PasskeyConfirm()(w, req)

	assertErrorBody(t, w, http.StatusBadRequest, "no pending passkey")

	// State should still exist (peek was used, not consume)
	if pk := peekPendingPasskey(userID); pk == nil {
		t.Fatal("state should still exist after nil-client check")
	}
}
