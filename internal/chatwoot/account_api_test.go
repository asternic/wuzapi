package chatwoot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAccountTestConnection(t *testing.T) {
	client, err := NewClient("https://example.test", 1, "token", WithHTTPClient(&http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET, got %s", r.Method)
			}
			if r.URL.Path != "/api/v1/accounts/1" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if r.Header.Get("api_access_token") != "token" {
				t.Fatalf("missing api_access_token header")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		}),
	}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := client.TestConnection(context.Background()); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
}

func TestCreateAPIInbox(t *testing.T) {
	client, err := NewClient("https://example.test", 1, "token", WithHTTPClient(&http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/api/v1/accounts/1/inboxes" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			channel := body["channel"].(map[string]any)
			if channel["type"].(string) != "api" {
				t.Fatalf("expected channel type api")
			}
			if channel["webhook_url"].(string) != "https://example.com/callback" {
				t.Fatalf("unexpected webhook url: %v", channel["webhook_url"])
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":55,"inbox_identifier":"abc123"}`)),
			}, nil
		}),
	}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	inbox, err := client.CreateAPIInbox(context.Background(), "WuzAPI", "https://example.com/callback")
	if err != nil {
		t.Fatalf("CreateAPIInbox: %v", err)
	}
	if inbox.ID != 55 || inbox.InboxIdentifier != "abc123" {
		t.Fatalf("unexpected inbox response: %+v", inbox)
	}
}

func TestUpdateAPIInbox(t *testing.T) {
	client, err := NewClient("https://example.test", 1, "token", WithHTTPClient(&http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPatch {
				t.Fatalf("expected PATCH, got %s", r.Method)
			}
			if r.URL.Path != "/api/v1/accounts/1/inboxes/99" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			channel := body["channel"].(map[string]any)
			if channel["webhook_url"].(string) != "https://example.com/callback" {
				t.Fatalf("unexpected webhook url: %v", channel["webhook_url"])
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":99,"inbox_identifier":"abc123"}`)),
			}, nil
		}),
	}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	inbox, err := client.UpdateAPIInbox(context.Background(), 99, "WuzAPI", "https://example.com/callback")
	if err != nil {
		t.Fatalf("UpdateAPIInbox: %v", err)
	}
	if inbox.ID != 99 {
		t.Fatalf("unexpected inbox response: %+v", inbox)
	}
}

func TestAccountAPIReturnsError(t *testing.T) {
	client, err := NewClient("https://example.test", 1, "token", WithHTTPClient(&http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("missing")),
				Header:     http.Header{},
			}, nil
		}),
	}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = client.TestConnection(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", apiErr.StatusCode)
	}
}

func TestToggleConversationStatus(t *testing.T) {
	client, err := NewClient("https://example.test", 1, "token", WithHTTPClient(&http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/api/v1/accounts/1/conversations/7/toggle_status" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["status"].(string) != "pending" {
				t.Fatalf("unexpected status: %v", body["status"])
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{}`)),
			}, nil
		}),
	}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := client.ToggleConversationStatus(context.Background(), 7, "pending"); err != nil {
		t.Fatalf("ToggleConversationStatus: %v", err)
	}
}
