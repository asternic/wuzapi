package chatwoot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCreateContact(t *testing.T) {
	client, err := NewClient("https://example.test", 1, "token", WithHTTPClient(&http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/public/api/v1/inboxes/inbox-123/contacts" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["identifier"].(string) != "5511999999999" {
				t.Fatalf("unexpected identifier: %v", body["identifier"])
			}
			if body["phone_number"].(string) != "+5511999999999" {
				t.Fatalf("unexpected phone_number: %v", body["phone_number"])
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"source_id":"contact-abc"}`)),
			}, nil
		}),
	}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	id, err := client.CreateContact(context.Background(), "inbox-123", CreateContactRequest{
		Identifier:  "5511999999999",
		PhoneNumber: "+5511999999999",
	})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	if id != "contact-abc" {
		t.Fatalf("expected source_id contact-abc, got %q", id)
	}
}

func TestCreateConversation(t *testing.T) {
	client, err := NewClient("https://example.test", 1, "token", WithHTTPClient(&http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/public/api/v1/inboxes/inbox-123/contacts/contact-abc/conversations" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if r.Body != nil {
				data, _ := io.ReadAll(r.Body)
				if len(strings.TrimSpace(string(data))) != 0 {
					var body map[string]any
					if err := json.Unmarshal(data, &body); err != nil {
						t.Fatalf("decode body: %v", err)
					}
				}
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":42}`)),
			}, nil
		}),
	}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	id, err := client.CreateConversation(context.Background(), "inbox-123", "contact-abc", CreateConversationRequest{})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if id != 42 {
		t.Fatalf("expected conversation id 42, got %d", id)
	}
}

func TestCreateMessage(t *testing.T) {
	client, err := NewClient("https://example.test", 1, "token", WithHTTPClient(&http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/public/api/v1/inboxes/inbox-123/contacts/contact-abc/conversations/99/messages" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["content"].(string) != "hello" {
				t.Fatalf("unexpected content: %v", body["content"])
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"msg-1"}`)),
			}, nil
		}),
	}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	id, err := client.CreateMessage(context.Background(), "inbox-123", "contact-abc", 99, "hello")
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if id != "msg-1" {
		t.Fatalf("expected message id msg-1, got %q", id)
	}
}
