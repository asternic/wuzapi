package chatwoot

import (
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"
)

// Client struct holds the configuration for the Chatwoot client.
type Client struct {
	httpClient  *resty.Client
	baseURL     string
	accessToken string
	accountID   string
	inboxID     string // Keep inboxID if it's frequently used in requests, or pass as param
}

// NewClient creates a new Chatwoot client.
// The inboxID is included here for convenience if most operations target a specific inbox.
func NewClient(baseURL, accessToken, accountID, inboxID string) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("Chatwoot baseURL cannot be empty")
	}
	if accessToken == "" {
		return nil, fmt.Errorf("Chatwoot accessToken cannot be empty")
	}
	if accountID == "" {
		return nil, fmt.Errorf("Chatwoot accountID cannot be empty")
	}
	// inboxID might be optional at client level if methods will specify it
	if inboxID == "" {
		return nil, fmt.Errorf("Chatwoot inboxID cannot be empty for this client setup")
	}

	client := resty.New().
		SetBaseURL(baseURL).
		SetHeader("api_access_token", accessToken). // Common header for Chatwoot
		SetTimeout(10 * time.Second)

	log.Info().Str("baseURL", baseURL).Str("accountID", accountID).Str("inboxID", inboxID).Msg("Chatwoot client configured")

	return &Client{
		httpClient:  client,
		baseURL:     baseURL,
		accessToken: accessToken,
		accountID:   accountID,
		inboxID:     inboxID,
	}, nil
}

// CreateContact creates a new contact in Chatwoot.
func (c *Client) CreateContact(payload ChatwootContactPayload) (*ChatwootContact, error) {
	url := fmt.Sprintf("/api/v1/accounts/%s/contacts", c.accountID)

	resp, err := c.httpClient.R().
		SetBody(payload).
		SetResult(&ChatwootContact{}). // Expecting direct contact object, not nested like {"payload": {...}}
		Post(url)

	if err != nil {
		log.Error().Err(err).Str("url", url).Interface("payload", payload).Msg("Chatwoot API: CreateContact request failed")
		return nil, fmt.Errorf("Chatwoot API CreateContact request failed: %w", err)
	}

	if resp.IsError() {
		log.Error().Str("url", url).Interface("payload", payload).Int("statusCode", resp.StatusCode()).Str("responseBody", string(resp.Body())).Msg("Chatwoot API: CreateContact returned an error")
		return nil, fmt.Errorf("Chatwoot API CreateContact error: status %s, body: %s", resp.Status(), resp.String())
	}

	contact := resp.Result().(*ChatwootContact)
	log.Info().Int("contactID", contact.ID).Str("phoneNumber", contact.PhoneNumber).Msg("Successfully created Chatwoot contact")
	return contact, nil
}

// GetContactByPhone searches for a contact by phone number.
// Note: Chatwoot's search is a general query 'q'. If 'phone_number' is not a unique indexed field for search,
// this might return multiple contacts if other fields match the number.
// For exact match on phone number, Chatwoot might require a filter if available, or this function needs to iterate.
func (c *Client) GetContactByPhone(phoneNumber string) (*ChatwootContact, error) {
	url := fmt.Sprintf("/api/v1/accounts/%s/contacts/search", c.accountID)

	var searchResult ChatwootContactSearchPayload // Expects {"payload": [...]}
	resp, err := c.httpClient.R().
		SetQueryParam("q", phoneNumber).
		SetResult(&searchResult).
		Get(url)

	if err != nil {
		log.Error().Err(err).Str("url", url).Str("phoneNumber", phoneNumber).Msg("Chatwoot API: GetContactByPhone request failed")
		return nil, fmt.Errorf("Chatwoot API GetContactByPhone request failed: %w", err)
	}

	if resp.IsError() {
		log.Error().Str("url", url).Str("phoneNumber", phoneNumber).Int("statusCode", resp.StatusCode()).Str("responseBody", string(resp.Body())).Msg("Chatwoot API: GetContactByPhone returned an error")
		return nil, fmt.Errorf("Chatwoot API GetContactByPhone error: status %s, body: %s", resp.Status(), resp.String())
	}

	// Iterate through search results to find an exact match for the phone number.
	// Chatwoot search can be broad.
	for _, contact := range searchResult.Payload {
		if contact.PhoneNumber == phoneNumber {
			log.Info().Int("contactID", contact.ID).Str("phoneNumber", phoneNumber).Msg("Found Chatwoot contact by phone number")
			return &contact, nil
		}
	}

	log.Info().Str("phoneNumber", phoneNumber).Msg("No Chatwoot contact found with this exact phone number")
	return nil, nil // Contact not found
}

// CreateConversation creates a new conversation in Chatwoot.
func (c *Client) CreateConversation(payload ChatwootConversationPayload) (*ChatwootConversation, error) {
	url := fmt.Sprintf("/api/v1/accounts/%s/conversations", c.accountID)

	resp, err := c.httpClient.R().
		SetBody(payload).
		SetResult(&ChatwootConversation{}). // Expecting direct conversation object as response
		Post(url)

	if err != nil {
		log.Error().Err(err).Str("url", url).Interface("payload", payload).Msg("Chatwoot API: CreateConversation request failed")
		return nil, fmt.Errorf("Chatwoot API CreateConversation request failed: %w", err)
	}

	if resp.IsError() {
		log.Error().Str("url", url).Interface("payload", payload).Int("statusCode", resp.StatusCode()).Str("responseBody", string(resp.Body())).Msg("Chatwoot API: CreateConversation returned an error")
		return nil, fmt.Errorf("Chatwoot API CreateConversation error: status %s, body: %s", resp.Status(), resp.String())
	}

	conversation := resp.Result().(*ChatwootConversation)
	log.Info().Int("conversationID", conversation.ID).Int("contactID", payload.ContactID).Msg("Successfully created Chatwoot conversation")
	return conversation, nil
}

// GetConversationsForContact retrieves conversations for a given contact ID.
func (c *Client) GetConversationsForContact(contactID int) ([]ChatwootConversation, error) {
	url := fmt.Sprintf("/api/v1/accounts/%s/contacts/%d/conversations", c.accountID, contactID)

	var responsePayload ChatwootContactConversationsResponse // Expects {"payload": [...]}
	resp, err := c.httpClient.R().
		SetResult(&responsePayload).
		Get(url)

	if err != nil {
		log.Error().Err(err).Str("url", url).Int("contactID", contactID).Msg("Chatwoot API: GetConversationsForContact request failed")
		return nil, fmt.Errorf("Chatwoot API GetConversationsForContact request failed: %w", err)
	}

	if resp.IsError() {
		log.Error().Str("url", url).Int("contactID", contactID).Int("statusCode", resp.StatusCode()).Str("responseBody", string(resp.Body())).Msg("Chatwoot API: GetConversationsForContact returned an error")
		return nil, fmt.Errorf("Chatwoot API GetConversationsForContact error: status %s, body: %s", resp.Status(), resp.String())
	}

	log.Info().Int("contactID", contactID).Int("conversationCount", len(responsePayload.Payload)).Msg("Successfully retrieved conversations for contact")
	return responsePayload.Payload, nil
}

// CreateMessage sends a message to a Chatwoot conversation.
func (c *Client) CreateMessage(conversationID int, payload ChatwootMessagePayload) (*ChatwootMessage, error) {
	url := fmt.Sprintf("/api/v1/accounts/%s/conversations/%d/messages", c.accountID, conversationID)

	resp, err := c.httpClient.R().
		SetBody(payload).
		SetResult(&ChatwootMessage{}). // Expecting ChatwootMessage as response
		Post(url)

	if err != nil {
		log.Error().Err(err).Str("url", url).Interface("payload", payload).Msg("Chatwoot API: CreateMessage request failed")
		return nil, fmt.Errorf("Chatwoot API CreateMessage request failed: %w", err)
	}

	if resp.IsError() {
		// Log the full body for more context on API errors
		log.Error().Str("url", url).Interface("payload", payload).Int("statusCode", resp.StatusCode()).Str("responseBody", string(resp.Body())).Msg("Chatwoot API: CreateMessage returned an error")
		return nil, fmt.Errorf("Chatwoot API CreateMessage error: status %s, body: %s", resp.Status(), resp.String())
	}

	message := resp.Result().(*ChatwootMessage)
	log.Info().Int("messageID", message.ID).Int("conversationID", conversationID).Msg("Successfully created Chatwoot message")
	return message, nil
}

// UploadFile uploads a file to Chatwoot's generic upload endpoint.
// Chatwoot typically expects attachments to be uploaded first, and then their IDs are passed when creating a message.
// The exact endpoint for general file uploads might be /api/v1/accounts/{account_id}/upload
// The response should contain an ID for the uploaded attachment.
func (c *Client) UploadFile(fileData []byte, fileName string, contentType string) (*ChatwootAttachment, error) {
	// Note: The 'contentType' parameter might not be explicitly needed by SetFileBytes,
	// as Resty might infer it or Chatwoot might determine it server-side.
	// However, it's good practice to have it if the server requires a specific form field for it.

	// Using a common endpoint pattern, adjust if Chatwoot's specific endpoint is different.
	// The direct upload endpoint might not be tied to a conversation yet.
	url := fmt.Sprintf("/api/v1/accounts/%s/upload", c.accountID)

	// Chatwoot expects the file as 'attachment' or 'attachments[]' in multipart form.
	// Let's assume 'attachment' for a single file upload.
	resp, err := c.httpClient.R().
		SetFileBytes("attachment", fileName, fileData). // "attachment" is the form field name, fileName is the reported filename
		// SetHeader("Content-Type", "multipart/form-data"). // Resty usually sets this automatically for SetFile/SetFileReader/SetFileBytes
		SetResult(&ChatwootAttachment{}). // Expecting ChatwootAttachment as response
		Post(url)

	if err != nil {
		log.Error().Err(err).Str("url", url).Str("fileName", fileName).Msg("Chatwoot API: UploadFile request failed")
		return nil, fmt.Errorf("Chatwoot API UploadFile request failed for %s: %w", fileName, err)
	}

	if resp.IsError() {
		log.Error().Str("url", url).Str("fileName", fileName).Int("statusCode", resp.StatusCode()).Str("responseBody", string(resp.Body())).Msg("Chatwoot API: UploadFile returned an error")
		return nil, fmt.Errorf("Chatwoot API UploadFile error for %s: status %s, body: %s", fileName, resp.Status(), resp.String())
	}

	attachment := resp.Result().(*ChatwootAttachment)
	if attachment.ID == 0 {
		log.Error().Str("fileName", fileName).Interface("response", attachment).Msg("Chatwoot API: UploadFile response did not contain a valid attachment ID")
		return nil, fmt.Errorf("Chatwoot API UploadFile for %s returned no ID", fileName)
	}

	log.Info().Int("attachmentID", attachment.ID).Str("fileName", fileName).Str("dataURL", attachment.DataURL).Msg("Successfully uploaded file to Chatwoot")
	return attachment, nil
}
