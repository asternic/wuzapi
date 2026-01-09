package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type chatwootSendCall struct {
	userID  string
	waJID   string
	content string
}

type chatwootSendStub struct {
	calls []chatwootSendCall
	err   error
}

func (s *chatwootSendStub) Send(ctx context.Context, userID, waJID, content string) (string, error) {
	s.calls = append(s.calls, chatwootSendCall{userID: userID, waJID: waJID, content: content})
	if s.err != nil {
		return "", s.err
	}
	return "msg-1", nil
}

func TestChatwootCallbackMissingToken(t *testing.T) {
	s := makeTestServer(t)
	handler := s.chatwootCallbackHandler((&chatwootSendStub{}).Send)

	req := newChatwootCallbackRequest(t, map[string]any{"event": "message_created"}, "")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestChatwootCallbackIncomingIgnored(t *testing.T) {
	s := makeTestServer(t)
	insertChatwootConfig(t, s, &ChatwootConfig{
		WuzapiUserID:   "user-1",
		CallbackSecret: "secret-1",
		Enabled:        true,
	})

	stub := &chatwootSendStub{}
	handler := s.chatwootCallbackHandler(stub.Send)

	payload := map[string]any{
		"event":        "message_created",
		"message_type": "incoming",
		"content":      "hello",
		"conversation": map[string]any{
			"contact_inbox": map[string]any{"source_id": "contact-1"},
		},
	}
	req := newChatwootCallbackRequest(t, payload, "secret-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if len(stub.calls) != 0 {
		t.Fatalf("expected no sends, got %d", len(stub.calls))
	}
}

func TestChatwootCallbackOutgoingSends(t *testing.T) {
	s := makeTestServer(t)
	insertChatwootConfig(t, s, &ChatwootConfig{
		WuzapiUserID:   "user-1",
		CallbackSecret: "secret-1",
		InboxID:        10,
		Enabled:        true,
	})
	if err := s.UpsertChatwootMap(&ChatwootMap{
		WuzapiUserID:              "user-1",
		WaJID:                     "5511999999999@s.whatsapp.net",
		WaPhone:                   "5511999999999",
		ChatwootContactIdentifier: "contact-1",
	}); err != nil {
		t.Fatalf("upsert chatwoot map: %v", err)
	}

	stub := &chatwootSendStub{}
	handler := s.chatwootCallbackHandler(stub.Send)

	payload := map[string]any{
		"event": "message_created",
		"conversation": map[string]any{
			"inbox_id":      10,
			"contact_inbox": map[string]any{"source_id": "contact-1"},
		},
		"message": map[string]any{
			"message_type": 1,
			"content":      "hello",
		},
	}
	req := newChatwootCallbackRequest(t, payload, "secret-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("expected 1 send, got %d", len(stub.calls))
	}
	call := stub.calls[0]
	if call.userID != "user-1" {
		t.Fatalf("expected userID user-1, got %q", call.userID)
	}
	if call.waJID != "5511999999999@s.whatsapp.net" {
		t.Fatalf("expected waJID, got %q", call.waJID)
	}
	if call.content != "hello" {
		t.Fatalf("expected content hello, got %q", call.content)
	}
}

func TestChatwootCallbackMissingMapping(t *testing.T) {
	s := makeTestServer(t)
	insertChatwootConfig(t, s, &ChatwootConfig{
		WuzapiUserID:   "user-1",
		CallbackSecret: "secret-1",
		Enabled:        true,
	})

	stub := &chatwootSendStub{}
	handler := s.chatwootCallbackHandler(stub.Send)

	payload := map[string]any{
		"event":        "message_created",
		"message_type": "outgoing",
		"content":      "hello",
		"conversation": map[string]any{
			"contact_inbox": map[string]any{"source_id": "contact-1"},
		},
	}
	req := newChatwootCallbackRequest(t, payload, "secret-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
	if len(stub.calls) != 0 {
		t.Fatalf("expected no sends, got %d", len(stub.calls))
	}
}

func TestChatwootCallbackPrivateIgnored(t *testing.T) {
	s := makeTestServer(t)
	insertChatwootConfig(t, s, &ChatwootConfig{
		WuzapiUserID:   "user-1",
		CallbackSecret: "secret-1",
		Enabled:        true,
	})

	stub := &chatwootSendStub{}
	handler := s.chatwootCallbackHandler(stub.Send)

	payload := map[string]any{
		"event":        "message_created",
		"message_type": "outgoing",
		"private":      true,
		"content":      "hello",
		"conversation": map[string]any{
			"contact_inbox": map[string]any{"source_id": "contact-1"},
		},
	}
	req := newChatwootCallbackRequest(t, payload, "secret-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if len(stub.calls) != 0 {
		t.Fatalf("expected no sends, got %d", len(stub.calls))
	}
}

func TestChatwootCallbackCommandResponds(t *testing.T) {
	s := makeTestServer(t)
	insertChatwootConfig(t, s, &ChatwootConfig{
		WuzapiUserID:            "user-1",
		CallbackSecret:          "secret-1",
		Enabled:                 true,
		ChatwootBaseURL:         "https://chatwoot.local",
		AccountID:               1,
		APIToken:                "account-token",
		InboxIdentifier:         "inbox-1",
		SystemContactIdentifier: "system-contact-1",
	})

	var capture requestCapture
	stubTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capture = captureRequest(req)
		return jsonResponse(http.StatusOK, map[string]any{"id": "msg-1"})
	})
	useChatwootHTTPClient(t, stubTransport)

	stub := &chatwootSendStub{}
	handler := s.chatwootCallbackHandler(stub.Send)

	payload := map[string]any{
		"event":        "message_created",
		"message_type": "outgoing",
		"content":      "#help",
		"conversation": map[string]any{
			"id": 123,
			"contact_inbox": map[string]any{
				"source_id": "system-contact-1",
			},
		},
	}
	req := newChatwootCallbackRequest(t, payload, "secret-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if len(stub.calls) != 0 {
		t.Fatalf("expected no sends, got %d", len(stub.calls))
	}
	if capture.Path != "/public/api/v1/inboxes/inbox-1/contacts/system-contact-1/conversations/123/messages" {
		t.Fatalf("unexpected chatwoot path: %s", capture.Path)
	}

	var body map[string]any
	if err := json.Unmarshal(capture.Body, &body); err != nil {
		t.Fatalf("decode chatwoot request: %v", err)
	}
	content, _ := body["content"].(string)
	if !strings.Contains(content, "Comandos") {
		t.Fatalf("expected help response, got %q", content)
	}
}

func newChatwootCallbackRequest(t *testing.T, payload any, token string) *http.Request {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/integrations/chatwoot/callback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		q := req.URL.Query()
		q.Set("token", token)
		req.URL.RawQuery = q.Encode()
	}
	return req
}

func insertChatwootConfig(t *testing.T, s *server, cfg *ChatwootConfig) {
	t.Helper()
	if err := s.UpsertChatwootConfig(cfg); err != nil {
		t.Fatalf("upsert chatwoot config: %v", err)
	}
}
