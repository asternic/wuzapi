package chatwoot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type Inbox struct {
	ID                 int    `json:"id"`
	InboxIdentifier    string `json:"inbox_identifier"`
	ChannelType        string `json:"channel_type"`
	CallbackWebhookURL string `json:"callback_webhook_url"`
	WebhookURL         string `json:"webhook_url"`
}

type createInboxRequest struct {
	Name    string       `json:"name"`
	Channel inboxChannel `json:"channel"`
}

type updateInboxRequest struct {
	Name    string        `json:"name,omitempty"`
	Channel *inboxChannel `json:"channel,omitempty"`
}

type inboxChannel struct {
	Type       string `json:"type"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

// TestConnection validates the account token against the Chatwoot Account API.
func (c *Client) TestConnection(ctx context.Context) error {
	if err := c.validateAccount(); err != nil {
		return err
	}
	path := fmt.Sprintf("/api/v1/accounts/%d", c.accountID)
	req, err := c.newAccountRequest(ctx, "GET", path, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

// CreateAPIInbox creates an API inbox with a webhook URL.
func (c *Client) CreateAPIInbox(ctx context.Context, name, webhookURL string) (*Inbox, error) {
	if err := c.validateAccount(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("name is required")
	}
	if strings.TrimSpace(webhookURL) == "" {
		return nil, errors.New("webhook URL is required")
	}
	if _, err := url.ParseRequestURI(webhookURL); err != nil {
		return nil, fmt.Errorf("invalid webhook URL: %w", err)
	}

	payload := createInboxRequest{
		Name: name,
		Channel: inboxChannel{
			Type:       "api",
			WebhookURL: webhookURL,
		},
	}

	path := fmt.Sprintf("/api/v1/accounts/%d/inboxes", c.accountID)
	req, err := c.newAccountRequest(ctx, "POST", path, payload)
	if err != nil {
		return nil, err
	}

	var resp Inbox
	if err := c.do(req, &resp); err != nil {
		return nil, err
	}
	if resp.ID == 0 || resp.InboxIdentifier == "" {
		return nil, errors.New("chatwoot response missing inbox identifier")
	}
	return &resp, nil
}

// UpdateAPIInbox updates the API inbox name and webhook URL.
func (c *Client) UpdateAPIInbox(ctx context.Context, inboxID int, name, webhookURL string) (*Inbox, error) {
	if err := c.validateAccount(); err != nil {
		return nil, err
	}
	if inboxID <= 0 {
		return nil, errors.New("inbox id is required")
	}
	if strings.TrimSpace(webhookURL) == "" {
		return nil, errors.New("webhook URL is required")
	}
	if _, err := url.ParseRequestURI(webhookURL); err != nil {
		return nil, fmt.Errorf("invalid webhook URL: %w", err)
	}

	payload := updateInboxRequest{
		Name: name,
		Channel: &inboxChannel{
			Type:       "api",
			WebhookURL: webhookURL,
		},
	}

	path := fmt.Sprintf("/api/v1/accounts/%d/inboxes/%d", c.accountID, inboxID)
	req, err := c.newAccountRequest(ctx, "PATCH", path, payload)
	if err != nil {
		return nil, err
	}

	var resp Inbox
	if err := c.do(req, &resp); err != nil {
		return nil, err
	}
	if resp.ID == 0 {
		return nil, errors.New("chatwoot response missing inbox id")
	}
	return &resp, nil
}

func (c *Client) newAccountRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("api_access_token", c.accountToken)
	return req, nil
}

func (c *Client) validateAccount() error {
	if c.accountID <= 0 {
		return errors.New("account id is required")
	}
	if strings.TrimSpace(c.accountToken) == "" {
		return errors.New("account token is required")
	}
	return nil
}
