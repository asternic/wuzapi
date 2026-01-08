package chatwoot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// Client wraps Chatwoot API calls with shared configuration.
type Client struct {
	baseURL      string
	accountID    int
	accountToken string
	httpClient   *http.Client
}

// Option allows configuring the client.
type Option func(*Client)

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

// NewClient creates a Chatwoot client with basic validation.
func NewClient(baseURL string, accountID int, accountToken string, opts ...Option) (*Client, error) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return nil, errors.New("baseURL is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid baseURL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("invalid baseURL scheme: %s", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, errors.New("baseURL host is required")
	}

	client := &Client{
		baseURL:      strings.TrimRight(parsed.String(), "/"),
		accountID:    accountID,
		accountToken: accountToken,
		httpClient:   &http.Client{Timeout: defaultTimeout},
	}

	for _, opt := range opts {
		opt(client)
	}

	if client.httpClient == nil {
		return nil, errors.New("http client is required")
	}

	return client, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	if method == "" {
		return nil, errors.New("method is required")
	}
	if path == "" {
		return nil, errors.New("path is required")
	}

	fullPath := path
	if !strings.HasPrefix(fullPath, "/") {
		fullPath = "/" + fullPath
	}

	fullURL := c.baseURL + fullPath

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode json: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

func (c *Client) do(req *http.Request, target any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(data))}
	}

	if target == nil {
		return nil
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

// APIError wraps non-2xx responses.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("chatwoot api error: status %d", e.StatusCode)
	}
	return fmt.Sprintf("chatwoot api error: status %d: %s", e.StatusCode, e.Body)
}
