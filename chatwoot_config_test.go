package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"wuzapi/internal/chatwoot"
)

func TestChatwootConfigSaveAndGet(t *testing.T) {
	s := makeTestServer(t)
	useChatwootHTTPClient(t, chatwootOnboardingTransport())
	saveHandler := s.SaveChatwootConfig()
	getHandler := s.GetChatwootConfig()

	payload := chatwootConfigPayload{
		ChatwootBaseURL: "https://chat.example",
		AccountID:       1,
		APIToken:        "api-token",
		InboxIdentifier: "inbox-1",
		InboxName:       "WuzAPI",
		InboxID:         10,
		CallbackSecret:  "secret-1234567890",
		Enabled:         true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	saveReq := withUserContext(httptest.NewRequest(http.MethodPost, "/integrations/chatwoot/config", bytes.NewReader(body)))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	saveHandler.ServeHTTP(saveRec, saveReq)

	if saveRec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, saveRec.Code)
	}

	getReq := withUserContext(httptest.NewRequest(http.MethodGet, "/integrations/chatwoot/config", nil))
	getRec := httptest.NewRecorder()
	getHandler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, getRec.Code)
	}

	resp := decodeJSONResponse(t, getRec)
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data in response, got %#v", resp["data"])
	}
	if data["chatwoot_base_url"] != "https://chat.example" {
		t.Fatalf("expected base url, got %#v", data["chatwoot_base_url"])
	}
	if int(data["inbox_id"].(float64)) != 10 {
		t.Fatalf("expected inbox id 10, got %#v", data["inbox_id"])
	}
}

func TestChatwootConfigGetNotFound(t *testing.T) {
	s := makeTestServer(t)
	handler := s.GetChatwootConfig()

	req := withUserContext(httptest.NewRequest(http.MethodGet, "/integrations/chatwoot/config", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestChatwootConnectionTest(t *testing.T) {
	var capture requestCapture
	stubTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capture = captureRequest(req)
		return jsonResponse(http.StatusOK, map[string]any{"id": 1})
	})
	useChatwootHTTPClient(t, stubTransport)

	s := makeTestServer(t)
	handler := s.TestChatwootConnection()

	payload := chatwootTestPayload{
		ChatwootBaseURL: "https://chatwoot.local",
		AccountID:       42,
		APIToken:        "account-token",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := withUserContext(httptest.NewRequest(http.MethodPost, "/integrations/chatwoot/test", bytes.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if capture.Method != http.MethodGet {
		t.Fatalf("expected GET request, got %s", capture.Method)
	}
	if capture.Path != "/api/v1/accounts/42" {
		t.Fatalf("expected path /api/v1/accounts/42, got %s", capture.Path)
	}
	if capture.Headers.Get("api_access_token") != "account-token" {
		t.Fatalf("expected api_access_token header")
	}
}

func TestChatwootInboxProvisionCreate(t *testing.T) {
	var capture requestCapture
	stubTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capture = captureRequest(req)
		return jsonResponse(http.StatusOK, map[string]any{
			"id":               77,
			"inbox_identifier": "inbox-77",
		})
	})
	useChatwootHTTPClient(t, stubTransport)

	s := makeTestServer(t)
	handler := s.ProvisionChatwootInbox()

	payload := chatwootInboxPayload{
		ChatwootBaseURL: "https://chatwoot.local",
		AccountID:       10,
		APIToken:        "account-token",
		InboxName:       "WuzAPI",
		CallbackSecret:  "secret-1234567890",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := withUserContext(httptest.NewRequest(http.MethodPost, "/integrations/chatwoot/inbox", bytes.NewReader(body)))
	req.Host = "wuzapi.local"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if capture.Method != http.MethodPost {
		t.Fatalf("expected POST request, got %s", capture.Method)
	}
	if capture.Path != "/api/v1/accounts/10/inboxes" {
		t.Fatalf("expected path /api/v1/accounts/10/inboxes, got %s", capture.Path)
	}

	var requestBody map[string]any
	if err := json.Unmarshal(capture.Body, &requestBody); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	channel, ok := requestBody["channel"].(map[string]any)
	if !ok {
		t.Fatalf("expected channel payload, got %#v", requestBody["channel"])
	}
	expectedCallback := "http://wuzapi.local/integrations/chatwoot/callback?token=secret-1234567890"
	if channel["webhook_url"] != expectedCallback {
		t.Fatalf("expected callback %q, got %#v", expectedCallback, channel["webhook_url"])
	}
}

func TestChatwootConfigSaveCreatesOnboarding(t *testing.T) {
	s := makeTestServer(t)
	useChatwootHTTPClient(t, chatwootOnboardingTransport())

	payload := chatwootConfigPayload{
		ChatwootBaseURL: "https://chat.example",
		AccountID:       1,
		APIToken:        "api-token",
		InboxIdentifier: "inbox-1",
		InboxName:       "WuzAPI",
		InboxID:         10,
		CallbackSecret:  "secret-1234567890",
		Enabled:         true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := withUserContext(httptest.NewRequest(http.MethodPost, "/integrations/chatwoot/config", bytes.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.SaveChatwootConfig().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	cfg, err := s.GetChatwootConfigByUserID("user-1")
	if err != nil {
		t.Fatalf("get chatwoot config: %v", err)
	}
	if cfg.SystemContactIdentifier != "system-contact-1" {
		t.Fatalf("expected system contact identifier, got %q", cfg.SystemContactIdentifier)
	}
	if !cfg.SystemConversationID.Valid || cfg.SystemConversationID.Int64 != 777 {
		t.Fatalf("expected system conversation id 777, got %v", cfg.SystemConversationID)
	}
}

func withUserContext(req *http.Request) *http.Request {
	ctx := context.WithValue(req.Context(), "userinfo", Values{m: map[string]string{
		"Id":    "user-1",
		"Token": "user-token",
	}})
	return req.WithContext(ctx)
}

func decodeJSONResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return payload
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type requestCapture struct {
	Method  string
	Path    string
	Headers http.Header
	Body    []byte
}

func captureRequest(req *http.Request) requestCapture {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		_ = req.Body.Close()
	}
	return requestCapture{
		Method:  req.Method,
		Path:    req.URL.Path,
		Headers: req.Header.Clone(),
		Body:    body,
	}
}

func jsonResponse(status int, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

func chatwootOnboardingTransport() http.RoundTripper {
	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/public/api/v1/inboxes/inbox-1/contacts":
			return jsonResponse(http.StatusOK, map[string]any{"source_id": "system-contact-1"})
		case "/public/api/v1/inboxes/inbox-1/contacts/system-contact-1/conversations":
			return jsonResponse(http.StatusOK, map[string]any{"id": 777})
		case "/public/api/v1/inboxes/inbox-1/contacts/system-contact-1/conversations/777/messages":
			return jsonResponse(http.StatusOK, map[string]any{"id": "msg-1"})
		default:
			return jsonResponse(http.StatusOK, map[string]any{})
		}
	})
}

func useChatwootHTTPClient(t *testing.T, transport http.RoundTripper) {
	t.Helper()

	previous := newChatwootClient
	newChatwootClient = func(baseURL string, accountID int, apiToken string, opts ...chatwoot.Option) (*chatwoot.Client, error) {
		opts = append(opts, chatwoot.WithHTTPClient(&http.Client{
			Transport: transport,
		}))
		return chatwoot.NewClient(baseURL, accountID, apiToken, opts...)
	}
	t.Cleanup(func() {
		newChatwootClient = previous
	})
}
