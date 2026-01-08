package chatwoot

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNewClientValidatesBaseURL(t *testing.T) {
	if _, err := NewClient("", 0, ""); err == nil {
		t.Fatal("expected error for empty baseURL")
	}

	if _, err := NewClient("ftp://example.com", 0, ""); err == nil {
		t.Fatal("expected error for invalid scheme")
	}

	if _, err := NewClient("http:///", 0, ""); err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestClientDoSuccess(t *testing.T) {
	client, err := NewClient("https://example.test", 1, "token", WithHTTPClient(&http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/ping" {
				t.Fatalf("expected path /ping, got %s", r.URL.Path)
			}
			if r.Header.Get("Accept") != "application/json" {
				t.Fatalf("expected Accept header, got %q", r.Header.Get("Accept"))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			}, nil
		}),
	}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	req, err := client.newRequest(context.Background(), http.MethodGet, "ping", nil)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}

	var resp struct {
		OK bool `json:"ok"`
	}
	if err := client.do(req, &resp); err != nil {
		t.Fatalf("do: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
}

func TestClientDoError(t *testing.T) {
	client, err := NewClient("https://example.test", 1, "token", WithHTTPClient(&http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("nope")),
				Header:     http.Header{},
			}, nil
		}),
	}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	req, err := client.newRequest(context.Background(), http.MethodGet, "/secure", nil)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}

	err = client.do(req, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Error(), "nope") {
		t.Fatalf("expected error body in message, got %q", apiErr.Error())
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
