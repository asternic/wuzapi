package chatwoot

import (
	"context"
	"errors"
	"fmt"
	"net/url"
)

type CreateContactRequest struct {
	Identifier       string         `json:"identifier,omitempty"`
	IdentifierHash   string         `json:"identifier_hash,omitempty"`
	Email            string         `json:"email,omitempty"`
	Name             string         `json:"name,omitempty"`
	PhoneNumber      string         `json:"phone_number,omitempty"`
	AvatarURL        string         `json:"avatar_url,omitempty"`
	CustomAttributes map[string]any `json:"custom_attributes,omitempty"`
}

type createContactResponse struct {
	SourceID string `json:"source_id"`
}

type CreateConversationRequest struct {
	CustomAttributes map[string]any `json:"custom_attributes,omitempty"`
}

type createConversationResponse struct {
	ID int `json:"id"`
}

type CreateMessageRequest struct {
	Content string `json:"content"`
	EchoID  string `json:"echo_id,omitempty"`
}

type createMessageResponse struct {
	ID string `json:"id"`
}

func (c *Client) CreateContact(ctx context.Context, inboxIdentifier string, payload CreateContactRequest) (string, error) {
	if inboxIdentifier == "" {
		return "", errors.New("inbox identifier is required")
	}

	path := fmt.Sprintf("/public/api/v1/inboxes/%s/contacts", url.PathEscape(inboxIdentifier))
	req, err := c.newRequest(ctx, "POST", path, payload)
	if err != nil {
		return "", err
	}

	var resp createContactResponse
	if err := c.do(req, &resp); err != nil {
		return "", err
	}
	if resp.SourceID == "" {
		return "", errors.New("chatwoot response missing source_id")
	}
	return resp.SourceID, nil
}

func (c *Client) CreateConversation(ctx context.Context, inboxIdentifier, contactIdentifier string, payload CreateConversationRequest) (int, error) {
	if inboxIdentifier == "" {
		return 0, errors.New("inbox identifier is required")
	}
	if contactIdentifier == "" {
		return 0, errors.New("contact identifier is required")
	}

	path := fmt.Sprintf(
		"/public/api/v1/inboxes/%s/contacts/%s/conversations",
		url.PathEscape(inboxIdentifier),
		url.PathEscape(contactIdentifier),
	)
	var body any
	if payload.CustomAttributes != nil {
		body = payload
	}

	req, err := c.newRequest(ctx, "POST", path, body)
	if err != nil {
		return 0, err
	}

	var resp createConversationResponse
	if err := c.do(req, &resp); err != nil {
		return 0, err
	}
	if resp.ID == 0 {
		return 0, errors.New("chatwoot response missing conversation id")
	}
	return resp.ID, nil
}

func (c *Client) CreateMessage(ctx context.Context, inboxIdentifier, contactIdentifier string, conversationID int, content string) (string, error) {
	if inboxIdentifier == "" {
		return "", errors.New("inbox identifier is required")
	}
	if contactIdentifier == "" {
		return "", errors.New("contact identifier is required")
	}
	if conversationID == 0 {
		return "", errors.New("conversation id is required")
	}
	if content == "" {
		return "", errors.New("content is required")
	}

	path := fmt.Sprintf(
		"/public/api/v1/inboxes/%s/contacts/%s/conversations/%d/messages",
		url.PathEscape(inboxIdentifier),
		url.PathEscape(contactIdentifier),
		conversationID,
	)
	payload := CreateMessageRequest{Content: content}

	req, err := c.newRequest(ctx, "POST", path, payload)
	if err != nil {
		return "", err
	}

	var resp createMessageResponse
	if err := c.do(req, &resp); err != nil {
		return "", err
	}
	if resp.ID == "" {
		return "", errors.New("chatwoot response missing message id")
	}
	return resp.ID, nil
}
